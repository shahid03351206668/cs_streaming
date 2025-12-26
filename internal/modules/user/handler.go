package user

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

func (h *Handler) GetUserProfile(c *gin.Context) {
	paramID := c.Param("id")
	var id string

	if paramID != "" {
		id = paramID
	} else {
		user := c.MustGet("user").(models.User)
		id = user.ID
	}

	res, err := h.service.GetUserProfile(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":    res.User,
		"reviews": res.Reviews,
	})
}
