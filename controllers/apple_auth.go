package controllers

import (
	"errors"
	"net/http"
	"strings"

	"tasksy/db"
	"tasksy/internal/modules/applesignin"
	"tasksy/lib"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// appleVerifier is replaced in tests.
var appleVerifier = applesignin.NewFromEnv()

// AppleSignInController signs a customer in with a Sign in with Apple identity
// token, creating the account on first use.
//
//	POST /api/auth/apple-auth
//	{ "identity_token": "...", "nonce": "...", "first_name": "...", "last_name": "..." }
//
// Apple sends the user's name only on the very first authorisation, and only to
// the app, so the app passes first_name/last_name along; they are used when the
// account is created and ignored afterwards.
func AppleSignInController(c *gin.Context) {
	var body struct {
		IdentityToken string `json:"identity_token"`
		IDToken       string `json:"id_token"` // alias
		Nonce         string `json:"nonce"`
		FirstName     string `json:"first_name"`
		LastName      string `json:"last_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "identity_token is required"})
		return
	}
	token := body.IdentityToken
	if token == "" {
		token = body.IDToken
	}
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "identity_token is required"})
		return
	}

	id, err := appleVerifier.Verify(c.Request.Context(), token, body.Nonce)
	switch {
	case errors.Is(err, applesignin.ErrNotConfigured):
		logger.Log.Error("apple sign-in attempted but APPLE_CLIENT_IDS is not set")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Sign in with Apple is not available"})
		return
	case errors.Is(err, applesignin.ErrInvalidToken):
		logger.Log.Warn("apple sign-in refused", zap.String("ip", c.ClientIP()), zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Apple identity token"})
		return
	case err != nil:
		logger.Log.Error("apple sign-in could not verify token", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "Could not verify the token with Apple, try again"})
		return
	}

	user, status, msg := findOrCreateAppleUser(id, body.FirstName, body.LastName)
	if status != 0 {
		c.JSON(status, gin.H{"error": msg})
		return
	}

	if user.Disabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "This account has been disabled"})
		return
	}
	if refuseAdminAccount(c, user.ID) {
		return
	}

	provisionStripeAccount(*user, c.ClientIP())

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	user.Password = ""
	c.JSON(http.StatusOK, gin.H{"message": "success", "user": user, "tokens": tokens})
}

// findOrCreateAppleUser returns the account for an Apple identity. The Apple
// subject is the account key; an existing account is only matched by email when
// Apple says that email is verified, so nobody can take over an account by
// claiming its address. A non-zero status means the request must be refused.
func findOrCreateAppleUser(id *applesignin.Identity, first, last string) (*models.User, int, string) {
	var user models.User

	err := db.DB.Where("apple_id = ?", id.Subject).First(&user).Error
	if err == nil {
		return &user, 0, ""
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, http.StatusInternalServerError, "Database error"
	}

	if id.Email == "" {
		return nil, http.StatusBadRequest, "Apple did not share an email address, so an account cannot be created"
	}

	err = db.DB.Where("LOWER(email) = ?", strings.ToLower(id.Email)).First(&user).Error
	switch {
	case err == nil:
		if !id.EmailVerified {
			return nil, http.StatusForbidden, "The email address on this Apple ID is not verified"
		}
		if user.AppleID != "" && user.AppleID != id.Subject {
			return nil, http.StatusConflict, "This account is linked to a different Apple ID"
		}
		user.AppleID = id.Subject
		user.EmailVerified = true
		if err := db.DB.Model(&user).Updates(map[string]any{"apple_id": user.AppleID, "email_verified": true}).Error; err != nil {
			return nil, http.StatusInternalServerError, "Failed to link account"
		}
		return &user, 0, ""
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, http.StatusInternalServerError, "Database error"
	}

	user = models.User{
		FirstName:     strings.TrimSpace(first),
		LastName:      strings.TrimSpace(last),
		Email:         id.Email,
		AppleID:       id.Subject,
		EmailVerified: id.EmailVerified,
	}
	if err := db.DB.Create(&user).Error; err != nil {
		// A concurrent first sign-in with the same Apple ID loses on the unique index.
		var winner models.User // not `user`: its fresh ID would be added to the query
		if e2 := db.DB.Where("apple_id = ?", id.Subject).First(&winner).Error; e2 == nil {
			return &winner, 0, ""
		}
		logger.Log.Error("failed to create apple user", zap.Error(err))
		return nil, http.StatusInternalServerError, "Failed to create user"
	}
	return &user, 0, ""
}
