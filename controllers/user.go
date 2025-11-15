package controllers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func RegisterUser(c *gin.Context) {
	var body struct {
		FirstName   string `json:"first_name" binding:"required"`
		LastName    string `json:"last_name"`
		Email       string `json:"email"`
		Password    string `json:"password" binding:"required,min=6"`
		PhoneNumber string `json:"phone_number"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "Please provide a valid json object",
		})
		return
	}

	if body.Email == "" && body.PhoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a valid json object",
		})
		return
	}

	var existingUser models.User
	if body.Email != "" {
		if err := db.DB.Where("email = ?", body.Email).First(&existingUser).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "User with this email already exists",
			})
			return
		}

	} else {
		if err := db.DB.Where("phone_number = ?", body.PhoneNumber).First(&existingUser).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": "User with this phone number already exists",
			})
			return
		}
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
	tokens, err := lib.GenerateAuthTokens(user.ID)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"user":    user,
		"tokens":  tokens,
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
		NewPassword     string `json:"new_password" binding:"required,min=6"`
		CurrentPassword string `json:"current_password" binding:"required"`
	}

	user, _ := lib.GetUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized",
		})
		return
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Please provide a valid body",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.CurrentPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Current password is incorrect",
		})
		return
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to hash password",
			"error":   err.Error(),
		})
		return
	}

	// Update password
	if err := db.DB.Model(user).Update("password", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to update password",
			"error":   err.Error(),
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
		FirstName   string `form:"first_name"`
		LastName    string `form:"last_name"`
		PhoneNumber string `form:"phone_number"`
		Email       string `form:"email"`
	}

	user, exists := lib.GetUser(c)

	fmt.Println("User")
	fmt.Println(user)

	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "User not authenticated",
		})
		return
	}

	if err := c.ShouldBind(&body); err != nil {
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
	if body.Email != "" {
		updates["email"] = body.Email
	}

	file, err := c.FormFile("profile_photo")
	if err == nil && file != nil {
		allowedTypes := map[string]bool{
			"image/jpeg": true,
			"image/jpg":  true,
			"image/png":  true,
			"image/gif":  true,
			"image/webp": true,
		}

		contentType := file.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Invalid file type. Only images are allowed",
			})
			return
		}

		maxFileSize := int64(5 * 1024 * 1024) // 5MB
		if file.Size > maxFileSize {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "File size too large. Maximum 5MB allowed",
			})
			return
		}

		profilePhotoPath := filepath.Join(MEDIA_FILE_PATH, "profiles")
		if err := os.MkdirAll(profilePhotoPath, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Failed to create profile photo directory",
				"error":   err.Error(),
			})
			return
		}

		if user.ProfilePhoto != "" {
			oldPhotoPath := user.ProfilePhoto
			if _, err := os.Stat(oldPhotoPath); err == nil {
				os.Remove(oldPhotoPath)
			}
		}

		ext := filepath.Ext(file.Filename)
		// Convert UUID to string explicitly
		userIDStr := fmt.Sprintf("%v", user.ID)
		fileName := fmt.Sprintf("profile_%s_%d%s", userIDStr, time.Now().UnixNano(), ext)
		filePath := filepath.Join(profilePhotoPath, fileName)

		if err := c.SaveUploadedFile(file, filePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "Failed to upload profile photo",
				"error":   err.Error(),
			})
			return
		}

		updates["profile_photo"] = filePath
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "No fields to update",
		})
		return
	}

	if err := db.DB.Model(&user).Updates(updates).Error; err != nil {
		if profilePhoto, ok := updates["profile_photo"].(string); ok {
			os.Remove(profilePhoto)
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "Failed to update profile",
		})
		return
	}

	// Use Where clause with proper UUID parameter binding instead of First with direct ID
	var updatedUser models.User
	if err := db.DB.Where("id = ?", user.ID).First(&updatedUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Profile updated but failed to fetch updated data",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user": gin.H{
			"id":            updatedUser.ID,
			"first_name":    updatedUser.FirstName,
			"last_name":     updatedUser.LastName,
			"email":         updatedUser.Email,
			"phone_number":  updatedUser.PhoneNumber,
			"profile_photo": updatedUser.ProfilePhoto,
		},
	})
}

func VerifyUser(c *gin.Context) {
	var body struct {
		PhoneNumber string `json:"phone_number"`
		Email       string `json:"email"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Provide a valid JSON object",
			"error":   err.Error(),
		})
		return
	}

	if body.Email == "" && body.PhoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide either email or phone_number",
		})
		return
	}

	var user models.User
	query := db.DB

	if body.Email != "" {
		query = query.Where("email = ?", body.Email)
	}

	if body.PhoneNumber != "" {
		query = query.Where("phone_number = ?", body.PhoneNumber)
	}

	if err := query.First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "User not found",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User verified",
		"user": gin.H{
			"id":           user.ID,
			"email":        user.Email,
			"phone_number": user.PhoneNumber,
		},
	})
}
