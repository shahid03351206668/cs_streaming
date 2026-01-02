package user

import (
	"net/http"
	"tasksy/lib"
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

func (h *Handler) RegisterUser(c *gin.Context) {
	var data UserData

	if err := c.ShouldBind(&data); err != nil {
		return
	}

	file, _ := c.FormFile("image")
	user, err := h.service.CreateUser(data, file)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)
	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"user": map[string]any{
			"id":            user.ID,
			"created_at":    user.CreatedAt,
			"updated_at":    user.UpdatedAt,
			"first_name":    user.FirstName,
			"last_name":     user.LastName,
			"email":         user.Email,
			"phone_number":  user.PhoneNumber,
			"profile_photo": user.ProfilePhoto,
		},
		"tokens": tokens,
	})
}
