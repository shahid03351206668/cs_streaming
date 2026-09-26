package middleware

import (
	"net/http"

	"tasksy/db"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

// RequireRole must run after AuthMiddleware. It checks the user_roles join
// table (User.Roles many2many) for the given role name.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get("user")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		u := user.(models.User)

		var count int64
		if err := db.DB.Table("user_roles").
			Where("user_id = ? AND role_name = ?", u.ID, role).
			Count(&count).Error; err != nil || count == 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to perform this action"})
			return
		}

		c.Next()
	}
}
