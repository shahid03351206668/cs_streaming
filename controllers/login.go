package controllers

import (
	"net/http"
	"os"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/account"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// provisionStripeAccount creates a Stripe Express Connect account for a user
// if they don't already have one. Runs fire-and-forget; never blocks login.
func provisionStripeAccount(user models.User) {
	if user.StripeConnectAccountID != "" {
		return
	}
	go func() {
		stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
		acc, err := account.New(&stripe.AccountParams{
			Type:    stripe.String(string(stripe.AccountTypeExpress)),
			Email:   stripe.String(user.Email),
			Country: stripe.String("GB"),
			Capabilities: &stripe.AccountCapabilitiesParams{
				Transfers: &stripe.AccountCapabilitiesTransfersParams{
					Requested: stripe.Bool(true),
				},
			},
		})
		if err != nil {
			logger.Log.Error("stripe connect account creation failed at login", zap.String("user_id", user.ID), zap.Error(err))
			return
		}
		if err := db.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("stripe_connect_account_id", acc.ID).Error; err != nil {
			logger.Log.Error("failed to save stripe connect account id at login", zap.String("user_id", user.ID), zap.Error(err))
		}
	}()
}

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

	provisionStripeAccount(user)

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

	provisionStripeAccount(user)

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
