package notifications

import (
	"net/http"
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
