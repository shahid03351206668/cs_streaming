package controllers

import (
	"net/http"
	"tasksy/db"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func GetUsers(c *gin.Context) {
	type UserResponse struct {
		ID          string `json:"id"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}

	var users []UserResponse

	if err := db.DB.Model(&models.User{}).
		Select("id", "first_name", "last_name", "email", "phone_number", "created_at", "updated_at").
		Find(&users).
		Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	if users == nil {
		users = make([]UserResponse, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"users":   users,
	})
}
