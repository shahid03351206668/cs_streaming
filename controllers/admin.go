package controllers

import (
	"net/http"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"

	"strconv"

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


func ListRecords(c *gin.Context) {
	model := c.Param("model")
	if model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error": "model is missing in the query params",
		})
		return
	}


}

func AdminUserListController(c *gin.Context) {
	type UserResponse struct {
		ID          string `json:"id"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
		Disabled    bool   `json:"disabled"`
	}

	users := []UserResponse{}

	limit, err := strconv.ParseInt(c.Query("limit"), 10, 64)
	if err != nil {
		limit = 20
	}

	offset, err := strconv.ParseInt(c.Query("page"), 10, 64)
	if err != nil {
		offset = 0
	}

	if err := db.DB.Model(models.User{}).Find(&users).Limit(int(limit)).Offset(int(offset)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": users,
	})
}

func AdminGetUserController(c *gin.Context) {
	userID := c.Param("id")
	var user models.User

	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "User not found",
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": user,
	})
}
func AdminUserCreateController(c *gin.Context) {
	var body struct {
		FirstName   string `json:"first_name" binding:"required,min=2"`
		LastName    string `json:"last_name" binding:"required,min=2"`
		Email       string `json:"email" binding:"required,email"`
		PhoneNumber string `json:"phone_number" binding:"required,min=6"`
		Password    string `json:"password" binding:"required,min=6"`

		PhoneVerified bool `json:"phone_verified"`
		EmailVerified bool `json:"email_verified"`
		Disabled      bool `json:"disabled"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	DB := db.DB

	hashedPassword := ""
	if body.Password != "" {
		hashedPassword = lib.MakePassword(body.Password)
	}

	err := DB.Create(models.User{
		FirstName:     body.FirstName,
		LastName:      body.LastName,
		Email:         body.Email,
		Password:      hashedPassword,
		PhoneNumber:   body.PhoneNumber,
		PhoneVerified: body.PhoneVerified,
		EmailVerified: body.EmailVerified,
	}).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
	})

}

func AdminUserUpdateController() {

}
