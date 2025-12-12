package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"tasksy/db"
	"tasksy/models"
)

type QueryParams struct {
	Fields       []string        `json:"fields"`
	Filters      [][]interface{} `json:"filters"`
	OrFilters    [][]interface{} `json:"or_filters"`
	OrderBy      string          `json:"order_by"`
	GroupBy      string          `json:"group_by"`
	Start        int             `json:"start"`
	PageLength   int             `json:"page_length"`
	Search       string          `json:"search"`
	SearchFields []string        `json:"search_fields"`
}

type ListResponse struct {
	Message string                   `json:"message"`
	Data    []map[string]interface{} `json:"data"`
	Count   int64                    `json:"count"`
}

var ModelRegistry = map[string]interface{}{
	"user":     &models.User{},
	"job_post": &models.JobPost{},
	"category": &models.Category{},
	"contract": &models.Contract{},
}

func GetResourceList(c *gin.Context) {
	db := db.DB

	model := c.Param("model")

	if model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "model is missing in the query params",
		})
		return
	}

	modelType, exists := ModelRegistry[model]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   fmt.Sprintf("model '%s' not found in registry", model),
		})
		return
	}

	var params QueryParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	if params.PageLength == 0 {
		params.PageLength = 20
	}
	if params.PageLength > 500 {
		params.PageLength = 500
	}

	query := db.Model(modelType)
	query = applyFilters(query, params.Filters, "AND")
	query = applyFilters(query, params.OrFilters, "OR")

	if params.Search != "" && len(params.SearchFields) > 0 {
		query = applySearch(query, params.Search, params.SearchFields)
	}

	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"debug":   true,
			"error":   err.Error(),
		})
		return
	}

	idExists := false
	for _, v := range params.Fields {
		if v == "id " {
			idExists = true
		}
	}

	if !idExists {
		params.Fields = append(params.Fields, "id")
	}

	fmt.Println("params.Fields")
	fmt.Println(params.Fields)

	query = query.Select(params.Fields)

	if params.GroupBy != "" {
		query = query.Group(params.GroupBy)
	}

	if params.OrderBy != "" {
		query = query.Order(params.OrderBy)
	} else {
		query = query.Order("created_at DESC")
	}

	query = query.Offset(params.Start).Limit(params.PageLength)

	var results []map[string]interface{}
	if err := query.Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ListResponse{
		Message: "success",
		Data:    results,
		Count:   totalCount,
	})
}

// applyFilters applies filters to the query
// Filters format: [["field", "operator", "value"], ...]
func applyFilters(query *gorm.DB, filters [][]interface{}, logicOperator string) *gorm.DB {
	if len(filters) == 0 {
		return query
	}

	var conditions []string
	var values []interface{}

	for _, filter := range filters {
		if len(filter) < 3 {
			continue
		}

		field := fmt.Sprintf("%v", filter[0])
		operator := strings.ToLower(fmt.Sprintf("%v", filter[1]))
		value := filter[2]

		switch operator {
		case "=", "equals":
			conditions = append(conditions, fmt.Sprintf("%s = ?", field))
			values = append(values, value)
		case "!=", "not equals":
			conditions = append(conditions, fmt.Sprintf("%s != ?", field))
			values = append(values, value)
		case ">", "greater than":
			conditions = append(conditions, fmt.Sprintf("%s > ?", field))
			values = append(values, value)
		case ">=", "greater than or equals":
			conditions = append(conditions, fmt.Sprintf("%s >= ?", field))
			values = append(values, value)
		case "<", "less than":
			conditions = append(conditions, fmt.Sprintf("%s < ?", field))
			values = append(values, value)
		case "<=", "less than or equals":
			conditions = append(conditions, fmt.Sprintf("%s <= ?", field))
			values = append(values, value)
		case "like":
			conditions = append(conditions, fmt.Sprintf("%s LIKE ?", field))
			values = append(values, fmt.Sprintf("%%%v%%", value))
		case "not like":
			conditions = append(conditions, fmt.Sprintf("%s NOT LIKE ?", field))
			values = append(values, fmt.Sprintf("%%%v%%", value))
		case "in":
			conditions = append(conditions, fmt.Sprintf("%s IN ?", field))
			values = append(values, value)
		case "not in":
			conditions = append(conditions, fmt.Sprintf("%s NOT IN ?", field))
			values = append(values, value)
		case "is":
			if value == nil || fmt.Sprintf("%v", value) == "null" {
				conditions = append(conditions, fmt.Sprintf("%s IS NULL", field))
			}
		case "is not":
			if value == nil || fmt.Sprintf("%v", value) == "null" {
				conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", field))
			}
		case "between":
			if betweenVals, ok := value.([]interface{}); ok && len(betweenVals) == 2 {
				conditions = append(conditions, fmt.Sprintf("%s BETWEEN ? AND ?", field))
				values = append(values, betweenVals[0], betweenVals[1])
			}
		}
	}

	if len(conditions) > 0 {
		whereClause := strings.Join(conditions, fmt.Sprintf(" %s ", logicOperator))
		query = query.Where(whereClause, values...)
	}

	return query
}

// applySearch applies search across multiple fields
func applySearch(query *gorm.DB, searchTerm string, searchFields []string) *gorm.DB {
	if searchTerm == "" || len(searchFields) == 0 {
		return query
	}

	var conditions []string
	var values []interface{}

	searchPattern := fmt.Sprintf("%%%s%%", searchTerm)
	for _, field := range searchFields {
		conditions = append(conditions, fmt.Sprintf("%s LIKE ?", field))
		values = append(values, searchPattern)
	}

	whereClause := strings.Join(conditions, " OR ")
	return query.Where(whereClause, values...)
}

func DBMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	}
}
