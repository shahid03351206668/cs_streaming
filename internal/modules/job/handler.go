package job

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"tasksy/models"
)

// search radius in Kilometers
const JOB_SEARCH_RADIUS = 50
const MAX_JOBS_PER_PAGE = 20

type Handler struct {
	service Service
}

func NewHandler(s Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) JobFeedHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	category := c.Query("category")
	searchQuery := c.Query("query")

	// Parse optional location parameters for radius-based filtering
	var lat, lng *float64
	if latStr := c.Query("lat"); latStr != "" {
		if v, err := strconv.ParseFloat(latStr, 64); err == nil {
			lat = &v
		}
	}
	if lngStr := c.Query("long"); lngStr != "" {
		if v, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lng = &v
		}
	}

	// Both lat and long must be provided together
	if (lat != nil && lng == nil) || (lat == nil && lng != nil) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "both 'lat' and 'long' query parameters are required for location filtering",
			"message": "error",
		})
		return
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	if limit > MAX_JOBS_PER_PAGE {
		limit = MAX_JOBS_PER_PAGE
	}

	params := JobFeedParams{
		Category:    category,
		SearchQuery: searchQuery,
		Page:        page,
		Limit:       limit,
		Latitude:    lat,
		Longitude:   lng,
		RadiusKM:    JOB_SEARCH_RADIUS,
	}

	jobs, count, err := h.service.GetJobFeed(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	meta := map[string]any{
		"page":  page,
		"limit": MAX_JOBS_PER_PAGE,
		"total": count,
	}
	if lat != nil && lng != nil {
		meta["radius_km"] = JOB_SEARCH_RADIUS
		meta["location"] = map[string]float64{"lat": *lat, "long": *lng}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    jobs,
		"message": "success",
		"meta":    meta,
	})
}

func (h *Handler) CreateJobPost(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	var data JobPostData
	if err := c.ShouldBind(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
			"detail":  "Invalid form data",
		})
		return
	}

	form, _ := c.MultipartForm()
	fmt.Println(form)
	files := form.File["media"]

	var category models.Category
	if err := h.service.db.First(&category, "id = ?", data.CategoryID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
			"detail":  "category not found",
		})
		return
	}

	tx := h.service.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	jobPost, err := h.service.CreateJobPost(user, data, files)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error while creating job post",
			"error":   err.Error(),
		})
		return

	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    jobPost,
	})
}
