package middleware

import (
	"net/http"
	"strings"
	"tasksy/lib"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"`
	jwt.RegisteredClaims
}


func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := c.Request.Header.Get("Authorization")

		if headers == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "authorization header missing",
				"message": "error",
			})
			return
		}

		parts := strings.Split(headers, " ")

		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Invalid authorization format. Expected 'Bearer <token>'",
				"message": "error",
			})
			return
		}

		token := parts[1]
		jwtSecret := lib.GetJWTSecret()

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Bearer token missing",
				"message": "error",
			})
			return
		}

		accessToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Invalid or expired token",
				"message": "error",
			})
			return
		}

		if claims, ok := accessToken.Claims.(*Claims); ok && accessToken.Valid {

			if claims.Type != "access" {
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "Invalid token type. Use access token",
					"message": "error",
				})
				return
			}

			c.Set("user_id", claims.UserID)
			c.Next()
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Internal Server Error"})
	}
}
