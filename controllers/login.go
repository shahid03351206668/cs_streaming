package controllers

import (
	"net/http"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func LoginController(c *gin.Context) {
	var body struct {
		Email    string
		Password string
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Please provide a request body",
		})
	}

	var user models.User
	result := db.DB.Where("email = ?", body.Email).First(&user)

	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid email or password",
		})
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid email or password",
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message": "logged in",
		"tokens":  tokens,
	})
}

func RefreshTokenController(c *gin.Context) {
	var body struct {
		RefreshToken string
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "Please provide valid body",
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
	tokens, err := lib.GenerateAuthTokens(claims.UserID)
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
		// "accessToken":  tokens.AccessToken,
		// "refreshToken": tokens.RefreshToken,
	})
}
