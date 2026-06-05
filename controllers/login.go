package controllers

import (
	"net/http"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func LoginControllerV1(c *gin.Context) {
	var body struct {
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
		Password    string `json:"password"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a valid request body",
		})
		return
	}

	if body.Email == "" && body.PhoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Please provide either email or phone number",
		})
		return
	}
	if body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Password is required",
		})
		return
	}

	var user models.User
	var result *gorm.DB

	if body.Email != "" {
		result = db.DB.Where("email = ?", body.Email).First(&user)
	} else {
		result = db.DB.Where("phone_number = ?", body.PhoneNumber).First(&user)
	}

	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	if user.Password == "" && user.GoogleID != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "This account uses Google Sign-In. Please login with Google.",
		})
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, 5)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"tokens":  tokens,
	})
}

func LoginController(c *gin.Context) {
	var body struct {
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
		Password    string `json:"password"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a valid request body",
		})
		return
	}

	if body.Email == "" && body.PhoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Please provide either email or phone number",
		})
		return
	}
	if body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Password is required",
		})
		return
	}

	var user models.User
	var result *gorm.DB

	if body.Email != "" {
		result = db.DB.Where("email = ?", body.Email).First(&user)
	} else {
		result = db.DB.Where("phone_number = ?", body.PhoneNumber).First(&user)
	}

	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	if user.Password == "" && user.GoogleID != "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "This account uses Google Sign-In. Please login with Google.",
		})
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"tokens":  tokens,
		"user":    user,
	})
}

func GetUserAuthToken(c *gin.Context) {
	var body struct {
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a valid request body",
		})
		return
	}

	if body.Email == "" && body.PhoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Please provide either email or phone number",
		})
		return
	}

	var user models.User
	var result *gorm.DB

	if body.Email != "" {
		result = db.DB.Where("email = ?", body.Email).First(&user)
	} else {
		result = db.DB.Where("phone_number = ?", body.PhoneNumber).First(&user)
	}

	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"tokens":  tokens,
		"user":    user,
	})
}

func RefreshTokenController(c *gin.Context) {
	var body struct {
		RefreshToken string
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Please provide valid body",
		})
		return
	}

	token, err := jwt.ParseWithClaims(body.RefreshToken, &lib.Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return lib.GetJWTSecret(), nil
	})

	claims, ok := token.Claims.(*lib.Claims)
	if !ok || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "Invalid token claims",
			"message": "Please login again",
		})
		return
	}
	if claims.Type != "refresh" {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "Invalid token type",
			"message": "Please provide a refresh token",
		})
		return
	}

	var user models.User
	if err := db.DB.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "User not found",
			"message": "Please login again",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "Invalid or expired refresh token",
			"message": "Please login again",
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(claims.UserID, 5)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to generate tokens",
			"message": "Please try again",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Tokens refreshed successfully",
		"tokens":  tokens,
	})
}
