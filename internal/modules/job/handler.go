package job

import (
	// "encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	// "tasksy/db"
	"tasksy/lib"
	"tasksy/models"
	"tasksy/pkg/logger"

	"go.uber.org/zap"
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

	var preferredCategoryIDs []string
	// if category == "" {
	// 	if userID := lib.TryGetUserID(c); userID != "" {
	// 		var prefs models.UserFeedPreferences
	// 		if err := db.DB.Where("user_id = ?", userID).First(&prefs).Error; err == nil {
	// 			var cats []models.CategoryItem
	// 			if json.Unmarshal(prefs.Categories, &cats) == nil {
	// 				for _, cat := range cats {
	// 					if cat.ID != "" {
	// 						preferredCategoryIDs = append(preferredCategoryIDs, cat.ID)
	// 					}
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	var CategoryIds []string

	splittedCats := strings.Split(category, ",")
	if len(splittedCats) > 0 {
		for _, id := range splittedCats {
			if id != "" {
				val := strings.TrimSpace(id)
				CategoryIds = append(CategoryIds, val)
			}
		}

	}

	viewerID := lib.TryGetUserID(c)
	params := JobFeedParams{
		Category:                CategoryIds,
		SearchQuery:             searchQuery,
		Page:                    page,
		Limit:                   limit,
		Latitude:                lat,
		Longitude:               lng,
		RadiusKM:                JOB_SEARCH_RADIUS,
		PreferredCategoryIDs:    preferredCategoryIDs,
		ExcludeReportedByUserID: viewerID,
		ExcludeBlockedUsersFor:  viewerID,
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

func (h *Handler) DeleteJobPost(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	jobID := c.Param("id")

	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "job id is required"})
		return
	}

	err := h.service.DeleteJobPost(jobID, user.ID)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
		return
	}

	switch {
	case errors.Is(err, ErrJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
	case errors.Is(err, ErrJobNotOwned):
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": err.Error()})
	case errors.Is(err, ErrJobCannotBeDeleted):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"message": "error", "error": err.Error()})
	default:
		logger.Log.Error("delete job post failed", zap.String("job_id", jobID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to delete job"})
	}
}

func (h *Handler) ReportJob(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	jobID := c.Param("id")

	var body struct {
		Reason  string `json:"reason" binding:"required"`
		Details string `json:"details"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	report, err := h.service.ReportJob(user.ID, jobID, body.Reason, body.Details)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"message": "success", "data": report})
		return
	}

	switch {
	case errors.Is(err, ErrJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
	case errors.Is(err, ErrAlreadyReported):
		c.JSON(http.StatusConflict, gin.H{"message": "error", "error": err.Error()})
	default:
		logger.Log.Error("report job failed", zap.String("job_id", jobID), zap.String("user_id", user.ID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to report job"})
	}
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
	logger.Log.Info("job creation multipart form received", zap.Int("file_count", len(form.File["media"])))
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
