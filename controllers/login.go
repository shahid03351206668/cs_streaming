package controllers

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

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

func stripeIndividualParams(user models.User, addr *models.UserAddress) *stripe.PersonParams {
	params := &stripe.PersonParams{
		FirstName: stripe.String(user.FirstName),
		LastName:  stripe.String(user.LastName),
		Email:     stripe.String(user.Email),
	}
	if user.PhoneNumber != "" {
		params.Phone = stripe.String(user.PhoneNumber)
	}
	if user.DateOfBirth != nil {
		params.DOB = &stripe.PersonDOBParams{
			Day:   stripe.Int64(int64(user.DateOfBirth.Day())),
			Month: stripe.Int64(int64(user.DateOfBirth.Month())),
			Year:  stripe.Int64(int64(user.DateOfBirth.Year())),
		}
	}
	if addr != nil {
		params.Address = &stripe.AddressParams{
			Line1:      stripe.String(addr.Line1),
			Line2:      stripe.String(addr.Line2),
			City:       stripe.String(addr.City),
			State:      stripe.String(addr.State),
			PostalCode: stripe.String(addr.PostalCode),
			Country:    stripe.String(addr.Country),
		}
	}
	return params
}

func stripeExternalAccountParams(bank *models.UserBankAccount) *stripe.AccountExternalAccountParams {
	if bank == nil || bank.AccountNumber == "" || bank.SortCode == "" {
		return nil
	}
	currency := bank.Currency
	if currency == "" {
		currency = "gbp"
	}
	return &stripe.AccountExternalAccountParams{
		AccountNumber:     stripe.String(bank.AccountNumber),
		AccountHolderName: stripe.String(bank.AccountHolderName),
		AccountHolderType: stripe.String("individual"),
		Country:           stripe.String("GB"),
		Currency:          stripe.String(currency),
		RoutingNumber:     stripe.String(strings.NewReplacer("-", "", " ", "").Replace(bank.SortCode)),
	}
}

func shouldAttachExternalAccount(user models.User, bank *models.UserBankAccount) bool {
	if stripeExternalAccountParams(bank) == nil {
		return false
	}
	return user.StripeConnectAccountID == "" || bank.StripeConnectAccountID == "" || bank.StripeConnectAccountID != user.StripeConnectAccountID
}

func loadStripeProvisioningDetails(userID string) (*models.UserAddress, *models.UserBankAccount) {
	var addr models.UserAddress
	var addrPtr *models.UserAddress
	if err := db.DB.Where("user_id = ?", userID).Order("is_default DESC, created_at DESC").First(&addr).Error; err == nil {
		addrPtr = &addr
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Log.Error("failed to load user address for stripe provisioning", zap.String("user_id", userID), zap.Error(err))
	}

	var bank models.UserBankAccount
	var bankPtr *models.UserBankAccount
	if err := db.DB.Where("user_id = ?", userID).Order("is_default DESC, created_at DESC").First(&bank).Error; err == nil {
		bankPtr = &bank
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Log.Error("failed to load user bank account for stripe provisioning", zap.String("user_id", userID), zap.Error(err))
	}

	return addrPtr, bankPtr
}

func provisionStripeAccount(user models.User, clientIP string) {
	go func() {
		stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
		now := time.Now().Unix()
		ip := clientIP
		if ip == "" {
			ip = "127.0.0.1"
		}

		addr, bank := loadStripeProvisioningDetails(user.ID)
		params := &stripe.AccountParams{
			Type:         stripe.String(string(stripe.AccountTypeCustom)),
			Email:        stripe.String(user.Email),
			Country:      stripe.String("GB"),
			BusinessType: stripe.String("individual"),
			BusinessProfile: &stripe.AccountBusinessProfileParams{
				URL: stripe.String("https://tasksy.co.uk"),
				MCC: stripe.String("7372"),
			},
			Individual: stripeIndividualParams(user, addr),
			TOSAcceptance: &stripe.AccountTOSAcceptanceParams{
				Date: stripe.Int64(now),
				IP:   stripe.String(ip),
			},
			Capabilities: &stripe.AccountCapabilitiesParams{
				CardPayments: &stripe.AccountCapabilitiesCardPaymentsParams{
					Requested: stripe.Bool(true),
				},
				Transfers: &stripe.AccountCapabilitiesTransfersParams{
					Requested: stripe.Bool(true),
				},
			},
		}
		if shouldAttachExternalAccount(user, bank) {
			params.ExternalAccount = stripeExternalAccountParams(bank)
		}

		if user.StripeConnectAccountID != "" {
			updateParams := *params
			updateParams.Type = nil
			updateParams.Country = nil
			if _, err := account.Update(user.StripeConnectAccountID, &updateParams); err != nil {
				logger.Log.Error("stripe connect account sync failed at login", zap.String("user_id", user.ID), zap.Error(err))
			}
			return
		}

		acc, err := account.New(params)
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

	if user.Disabled {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This account has been disabled",
		})
		return
	}

	provisionStripeAccount(user, c.ClientIP())

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

	if user.Disabled {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This account has been disabled",
		})
		return
	}

	provisionStripeAccount(user, c.ClientIP())

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

	if user.Disabled {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This account has been disabled",
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

	if user.Disabled {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "This account has been disabled",
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
