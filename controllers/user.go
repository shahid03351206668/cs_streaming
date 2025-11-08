package controllers

import (
	"fmt"
	"net/http"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func RegisterUser(c *gin.Context) {
	var body struct {
		FirstName   string `json:"firstName" binding:"required"`
		LastName    string `json:"lastName" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
		PhoneNumber string `json:"phoneNumber"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "Please provide a valid json object",
		})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to hash password",
		})
		return
	}

	user := models.User{
		FirstName:   body.FirstName,
		LastName:    body.LastName,
		Email:       body.Email,
		Password:    string(hashedPassword),
		PhoneNumber: body.PhoneNumber,
	}

	if err := db.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "Failed to create user",
		})
		return
	}

	user.Password = ""
	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"user":    user,
	})
}

func GetUsers(c *gin.Context) {
	var users []models.User

	if err := db.DB.Model(&users).Select(
		"ID",
		"FirstName",
		"LastName",
		"Email",
		"PhoneNumber",
		"CreatedAt",
		"UpdatedAt",
	).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "Failed to fetch users",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Users fetched successfully",
		"count":   len(users),
		"users":   users,
	})
}

func ChangePassword(c *gin.Context) {
	var body struct {
		NewPassword     string
		CurrentPassword string
	}

	user, _ := lib.GetUser(c)
	if user == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a valid body",
		})
		return
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a valid body",
		})
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.CurrentPassword))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "internal server error",
			"error":   err.Error(),
		})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "Current password is incorrect",
		})
		return
	}

	if err := db.DB.Model(&user).Update("password", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "Failed to update password",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password updated successfully",
	})
}

func GetProfile(c *gin.Context) {
	user, exists := lib.GetUser(c)

	fmt.Println(user)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "User not authenticated",
		})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"user":    user,
	})
}

func UpdateProfile(c *gin.Context) {
	var body struct {
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		PhoneNumber string `json:"phoneNumber"`
		// ProfilePhoto string `json:"profilePhoto"`
	}

	user, exists := lib.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "User not authenticated",
		})
		return
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "Please provide valid profile data",
		})
		return
	}

	updates := make(map[string]interface{})

	if body.FirstName != "" {
		updates["first_name"] = body.FirstName
	}
	if body.LastName != "" {
		updates["last_name"] = body.LastName
	}
	if body.PhoneNumber != "" {
		updates["phone_number"] = body.PhoneNumber
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "No fields to update",
		})
		return
	}

	if err := db.DB.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "Failed to update profile",
		})
		return
	}

	var updatedUser models.User
	if err := db.DB.Select(
		"id",
		"first_name",
		"last_name",
		"email",
		"phone_number",
		"verified",
		"profile_photo",
		"created_at",
		"updated_at",
	).Where("id = ?", user.ID).First(&updatedUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch updated user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user":    updatedUser,
	})
}
