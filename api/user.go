package api

import (
	"errors"
	"net/http"

	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserSerailizer struct {
	FirstName   string   `form:"first_name" binding:"required"`
	LastName    string   `form:"last_name" binding:"required"`
	Username    string   `form:"username"`
	Email       string   `form:"email" binding:"required,email"`
	PhoneNumber string   `form:"phone_number" binding:"required"`
	Password    string   `form:"password"`
	Roles       []string `form:"roles[]"`
}

func CreateUser(c *gin.Context) {
	var form UserSerailizer
	if err := c.ShouldBind(&form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	DB := db.DB
	hashedPwd := lib.MakePassword(form.Password)

	var roles []models.Role
	if len(form.Roles) > 0 {
		DB.Where("name IN ?", form.Roles).Find(&roles)
	}

	user := models.User{
		FirstName:   form.FirstName,
		LastName:    form.LastName,
		Email:       form.Email,
		PhoneNumber: form.PhoneNumber,
		Password:    hashedPwd,
		Roles:       roles,
	}

	if err := DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}

func UpdateUser(c *gin.Context) {
	id := c.Param("id")

	DB := db.DB
	var user models.User

	if err := db.DB.Preload("Roles").First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var form UserSerailizer
	if err := c.ShouldBind(&form); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// newPhotoPath, err := saveProfileImage(c)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload image"})
	// 	return
	// }
	// if newPhotoPath != "" {
	// 	user.ProfilePhoto = newPhotoPath
	// }

	user.FirstName = form.FirstName
	user.LastName = form.LastName
	user.Email = form.Email
	user.PhoneNumber = form.PhoneNumber

	if form.Password != "" {
		user.Password = lib.MakePassword(form.Password)
	}

	if len(form.Roles) > 0 {
		var roles []models.Role
		DB.Where("name IN ?", form.Roles).Find(&roles)
		DB.Model(&user).Association("Roles").Replace(roles)
	}

	if err := db.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully", "user": user})
}

func GetUser(c *gin.Context) {
	id := c.Param("id")
	DB := db.DB
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User ID is required"})
		return
	}

	var user models.User

	userID := uuid.MustParse(id)

	if err := DB.Preload("Roles").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "User not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"message": "Database error", "error": err.Error()})
		return
	}

	type Response struct {
		ID           string        `json:"id"`
		FirstName    string        `json:"first_name"`
		Username     string        `json:"username"`
		LastName     string        `json:"last_name"`
		Email        string        `json:"email"`
		PhoneNumber  string        `json:"phone_number"`
		Roles        []models.Role `json:"roles"`
		ProfilePhoto string        `json:"profile_photo"`
	}

	response := Response{
		ID:           user.ID,
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		Email:        user.Email,
		PhoneNumber:  user.PhoneNumber,
		Roles:        user.Roles,
		ProfilePhoto: user.ProfilePhoto,
	}

	c.JSON(http.StatusOK, gin.H{
		"data": response,
	})
}
