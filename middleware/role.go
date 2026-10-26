package middleware

import (
	"net/http"

	"tasksy/db"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func HasRole(userID, role string) bool {
	var count int64
	err := db.DB.Table("user_roles").
		Where("user_id = ? AND role_name = ?", userID, role).
		Count(&count).Error
	return err == nil && count > 0
}

// RequireRole must run after AuthMiddleware. The role is read from the
// database on every request, so revoking it takes effect immediately.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("user")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if !HasRole(user.(models.User).ID, role) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to perform this action"})
			return
		}

		c.Next()
	}
}

func AdminOnly() []gin.HandlerFunc {
	return []gin.HandlerFunc{AuthMiddleware(), RequireRole("admin")}
}
