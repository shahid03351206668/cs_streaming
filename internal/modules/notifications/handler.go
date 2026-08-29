package notifications

import (
	"net/http"
	"strconv"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetPreferences(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	pref, err := h.service.GetNotificationPreferences(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": pref})
}

func (h *Handler) ListNotifications(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	notifications, total, err := h.service.GetUserNotifications(user.ID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    notifications,
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

func (h *Handler) UpsertPreferences(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var req struct {
		EnableProposalSent     *bool     `json:"enable_proposal_sent"`
		EnableProposalReceived *bool     `json:"enable_proposal_received"`
		EnableProposalDecision *bool     `json:"enable_proposal_decision"`
		EnableNewJobs          *bool     `json:"enable_new_jobs"`
		JobRadiusKM            *float64  `json:"job_radius_km"`
		City                   *string   `json:"city"`
		Latitude               *float64  `json:"latitude"`
		Longitude              *float64  `json:"longitude"`
		CategoryIDs            *[]string `json:"category_ids"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	updated, err := h.service.UpsertNotificationPreferences(user.ID, UpsertPreferencesInput{
		EnableProposalSent:     req.EnableProposalSent,
		EnableProposalReceived: req.EnableProposalReceived,
		EnableProposalDecision: req.EnableProposalDecision,
		EnableNewJobs:          req.EnableNewJobs,
		JobRadiusKM:            req.JobRadiusKM,
		City:                   req.City,
		Latitude:               req.Latitude,
		Longitude:              req.Longitude,
		CategoryIDs:            req.CategoryIDs,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": updated})
}
