package controllers

import (
	"net/http"
	"tasksy/db"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func GetCategories(c *gin.Context) {
	type Response struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
	}

	var results []Response
	err := db.DB.
		Model(&models.Category{}).
		Select("id", "name", "sort_order").
		Where("disable = ?", false).
		Order("sort_order ASC, created_at ASC").
		Scan(&results).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error fetching categories",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    results,
	})
}

func CreateCategory(c *gin.Context) {
	DB := db.DB

	var body struct {
		Name string `binding:"required" json:"name"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	exists := DB.Where("name = ?", body.Name).First(&models.Category{})
	if exists.RowsAffected > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "category with this name already exists",
		})
		return
	}

	// Place new category at the end of the sort order.
	var maxOrder int
	DB.Model(&models.Category{}).Select("COALESCE(MAX(sort_order), -1)").Scan(&maxOrder)

	category := models.Category{Name: body.Name, SortOrder: maxOrder + 1}
	if err := DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"data":    category,
	})
}

func GetCategoryByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "id is required"})
		return
	}

	var category models.Category
	if err := db.DB.Where("id = ?", id).First(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error fetching category",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": category, "message": "success"})
}

func UpdateCategory(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "id is required"})
		return
	}

	var body struct {
		Name    string `binding:"required" json:"name"`
		Disable bool   `json:"disable"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	var category models.Category
	if err := db.DB.Model(&category).Where("id = ?", id).Updates(&body).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": category})
}

func DeleteCategory(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "id is required"})
		return
	}

	db.DB.Where("id = ?", id).Delete(&models.Category{})
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// ReorderCategories accepts an ordered list of category IDs and writes their
// sort_order positions in a single transaction.
// PUT /api/v1/category/reorder
// Body: {"ids": ["uuid1", "uuid2", "uuid3"]}
func ReorderCategories(c *gin.Context) {
	var body struct {
		IDs []string `json:"ids" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	tx := db.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for i, id := range body.IDs {
		if err := tx.Model(&models.Category{}).
			Where("id = ?", id).
			Update("sort_order", i).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "error",
				"error":   "failed to update sort order: " + err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	// Return the freshly sorted list so the client stays in sync.
	type Response struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
	}
	var results []Response
	db.DB.Model(&models.Category{}).
		Select("id", "name", "sort_order").
		Where("disable = ?", false).
		Order("sort_order ASC, created_at ASC").
		Scan(&results)

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": results})
}
