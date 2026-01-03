package user

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"tasksy/lib"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetUserProfile(c *gin.Context) {
	paramID := c.Param("id")
	var id string

	if paramID != "" {
		id = paramID
	} else {
		user := c.MustGet("user").(models.User)
		id = user.ID
	}

	res, err := h.service.GetUserProfile(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":    res.User,
		"reviews": res.Reviews,
	})
}

func (h *Handler) RegisterUser(c *gin.Context) {
	var data UserData

	if err := c.ShouldBind(&data); err != nil {
		return
	}

	file, _ := c.FormFile("image")
	user, err := h.service.CreateUser(data, file)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)
	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"user": map[string]any{
			"id":            user.ID,
			"created_at":    user.CreatedAt,
			"updated_at":    user.UpdatedAt,
			"first_name":    user.FirstName,
			"last_name":     user.LastName,
			"email":         user.Email,
			"phone_number":  user.PhoneNumber,
			"profile_photo": user.ProfilePhoto,
		},
		"tokens": tokens,
	})
}

func (h *Handler) StripeIdentityWebhookHandler(c *gin.Context) {
	const MaxRequestSize = int64(65536)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxRequestSize)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	endpointSecret := h.service.appConfig.Stripe.IdentityWebhookSecret

	if endpointSecret == "" {
		logger.Log.Error("Stripe Identity Webhook Secret is missing")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Configuration error"})
		return
	}

	event, err := webhook.ConstructEvent(body, c.GetHeader("Stripe-Signature"), endpointSecret)

	switch event.Type {
	case "identity.verification_session.verified":
		var session stripe.IdentityVerificationSession

		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			logger.Log.Error("Error parsing webhook JSON", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "JSON parsing error"})
			return
		}

		UserID := session.Metadata["user_id"]
		if UserID != "" {
			var user models.User
			if err := h.service.db.Where("id = ?", UserID).First(&user).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   err.Error(),
				})
				return
			}
			if user.ID == "" {
				logger.Log.Error("Webhook received but no user_id found in metadata")
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   fmt.Sprintf("invalid user ID %s", user.ID),
				})
				return
			}
			result := h.service.db.Select("id = ?", user.ID).Update("identity_verified", true)

			if result.Error != nil {
				logger.Log.Error("Database update failed", zap.Error(result.Error))
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   fmt.Sprintf("error while updating user %s ", err.Error()),
				})
				return
			}
		}
	default:
		fmt.Fprintf(os.Stdout, "Unhandled event type: %v", event.Type)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}
