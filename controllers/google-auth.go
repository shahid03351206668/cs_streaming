package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GoogleSignInFirebaseController(c *gin.Context) {
	var body struct {
		IDToken string `json:"id_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ID token is required",
		})
		return
	}

	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + body.IDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Failed to verify token",
		})
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token",
		})
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read response",
		})
		return
	}

	var tokenInfo struct {
		Aud           string `json:"aud"`
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Iss           string `json:"iss"`
	}

	if err := json.Unmarshal(data, &tokenInfo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse token info",
		})
		return
	}

	if tokenInfo.Iss != "https://securetoken.google.com/"+os.Getenv("FIREBASE_PROJECT_ID") &&
		tokenInfo.Iss != "https://accounts.google.com" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token issuer",
		})
		return
	}

	var user models.User
	result := db.DB.Where("email = ?", tokenInfo.Email).First(&user)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			user = models.User{
				FirstName:     tokenInfo.GivenName,
				LastName:      tokenInfo.FamilyName,
				Email:         tokenInfo.Email,
				GoogleID:      tokenInfo.Sub,
				ProfilePhoto:  tokenInfo.Picture,
				EmailVerified: tokenInfo.EmailVerified == "true",
			}

			if err := db.DB.Create(&user).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to create user",
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
			return
		}
	} else {
		if user.GoogleID == "" {
			user.GoogleID = tokenInfo.Sub
			user.ProfilePhoto = tokenInfo.Picture
			user.EmailVerified = tokenInfo.EmailVerified == "true"
			db.DB.Save(&user)
		}
	}

	provisionStripeAccount(user)

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate tokens",
		})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"user":    user,
		"tokens":  tokens,
	})
}
