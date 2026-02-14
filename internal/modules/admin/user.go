package admin

import (
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserPayload struct {
	FirstName        string `form:"first_name" json:"first_name" binding:"required"`
	LastName         string `form:"last_name" json:"last_name" binding:"required"`
	Email            string `form:"email" json:"email" binding:"required,email"`
	Password         string `form:"password" json:"password" binding:"required,min=6"`
	Disabled         bool   `form:"disabled" json:"disabled"`
	PhoneNumber      string `form:"phone_number" json:"phone_number"`
	PhoneVerified    bool   `form:"phone_verified" json:"phone_verified"`
	EmailVerified    bool   `form:"email_verified" json:"email_verified"`
	IdentityVerified bool   `form:"identity_verified" json:"identity_verified"`
	GoogleID         string `form:"google_id" json:"google_id"`
	// ProfilePhoto     string `form:"profile_photo" json:"profile_photo"`
}

type AdminHandler struct {
	db      *gorm.DB
	service *Service
}

func (h *AdminHandler) UserSaveHandler(c *gin.Context) {
	var data UserPayload

	if err := c.ShouldBind(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	user, err := h.service.CreateUser(&data, nil)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"data": map[string]any{
			"user": user,
		},
	})
}

func (h *AdminHandler) GetUserDetail(c *gin.Context) {
	id := c.Param("id")
	var user models.User

	if err := h.service.db.Where("id = ? ", id).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	loginHistory := map[string]any{}
	
	// var loginRes  models.

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"user":    user,
		"history": loginHistory,
	})
}
