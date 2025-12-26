package job

import (
	"net/http"
	"strconv"
	"tasksy/models"

	// "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/gin-gonic/gin"
)

const MAX_JOBS_PER_PAGE = 20

type Handler struct {
	service *Service
}

func (h *Handler) JobFeedHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	category := c.Query("category")
	searchQuery := c.Query("query")

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > MAX_JOBS_PER_PAGE {
		limit = MAX_JOBS_PER_PAGE
	}

	jobs, count, err := h.service.GetJobFeed(category, searchQuery, page, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    jobs,
		"message": "success",
		"meta": map[string]any{
			"page":  page,
			"limit": MAX_JOBS_PER_PAGE,
			"total": count,
		},
	})
}

func (h *Handler) CreateJobPost(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var body struct {
		CategoryID  string  `form:"category_id" binding:"required"`
		Title       string  `form:"title" binding:"required,min=3,max=255"`
		Description string  `form:"description" binding:"required,min=10"`
		Budget      float64 `form:"budget" binding:"omitempty,min=0"`
		OpenBudget  bool    `form:"open_budget"`
		Address     string  `form:"address" binding:"omitempty,max=500"`
	}

	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})

		return
	}

	var category models.Category
	if err := h.service.db.First(&category, "id = ?", body.CategoryID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}
	tx := h.service.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	jobPost := models.JobPost{
		Status:      models.JobStatusOpen,
		CreatedByID: user.ID,
		CategoryID:  category.ID,
		Title:       body.Title,
		Description: body.Description,
		Address:     body.Address,
	}

	if err := tx.Create(&jobPost).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	form, err := c.MultipartForm()
	if err == nil && form != nil && form.File["media"] != nil {

		// files := form.File["media"]
		// for _, i in range files {
		// }
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}
