package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"tasksy/db"
	"tasksy/lib"
	"tasksy/middleware"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const adminTokenMinutes = 60

var (
	adminEmailLimiter = middleware.NewLoginLimiter(5, 15*time.Minute)
	adminIPLimiter    = middleware.NewLoginLimiter(50, 15*time.Minute)

	// Compared against when the email is unknown, so response time doesn't reveal which emails exist.
	dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)
)

// AdminLoginController signs in administrators only. The password is always
// required, and an account without the admin role never receives a token.
func AdminLoginController(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	ipKey, emailKey := c.ClientIP(), email

	for _, l := range []struct {
		limiter *middleware.LoginLimiter
		key     string
	}{{adminIPLimiter, ipKey}, {adminEmailLimiter, emailKey}} {
		if wait, blocked := l.limiter.Blocked(l.key); blocked {
			c.Header("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many failed attempts, try again later"})
			return
		}
	}

	fail := func() {
		adminIPLimiter.Fail(ipKey)
		adminEmailLimiter.Fail(emailKey)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
	}

	var user models.User
	found := db.DB.Where("LOWER(email) = ?", email).First(&user).Error == nil

	hash := []byte(user.Password)
	if !found || user.Password == "" {
		hash = dummyPasswordHash
	}
	passwordOK := bcrypt.CompareHashAndPassword(hash, []byte(body.Password)) == nil

	if !found || !passwordOK || user.Disabled {
		fail()
		return
	}

	if !middleware.HasRole(user.ID, "admin") {
		logger.Log.Warn("non-admin tried the admin login", zap.String("user_id", user.ID), zap.String("ip", c.ClientIP()))
		c.JSON(http.StatusForbidden, gin.H{"error": "this account is not an administrator"})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, adminTokenMinutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	adminEmailLimiter.Reset(emailKey)
	logger.Log.Info("admin login", zap.String("user_id", user.ID), zap.String("ip", c.ClientIP()))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"tokens":  tokens,
		"user": gin.H{
			"id":         user.ID,
			"first_name": user.FirstName,
			"last_name":  user.LastName,
			"email":      user.Email,
			"roles":      []string{"admin"},
		},
	})
}

// refuseAdminAccount stops the password-less sign-in paths from being used
// against an administrator; admins must use AdminLoginController.
func refuseAdminAccount(c *gin.Context, userID string) bool {
	if !middleware.HasRole(userID, "admin") {
		return false
	}
	logger.Log.Warn("password-less sign-in refused for an admin account", zap.String("user_id", userID), zap.String("ip", c.ClientIP()))
	c.JSON(http.StatusForbidden, gin.H{"error": "administrators must sign in through the admin login"})
	return true
}
