package paymentsv2

import (
	"fmt"
	"io"
	"net/http"
	"tasksy/config"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84/webhook"
	// "golang.org/x/text/cases"
	// "gorm.io/gorm"
)

type Handler struct {
	config  *config.Config
	service *Service
}

func NewHandler(config *config.Config, service *Service) *Handler {
	return &Handler{
		config:  config,
		service: service,
	}
}

func (h *Handler) HandleStripeWebhook(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(65536))
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error"})
	}

	stripeConfig := h.config.Stripe
	webhookSecret := stripeConfig.WebhookSecret
	signature := c.GetHeader("Stripe-Signature")

	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	switch event.Type {
	case "charge.succeeded":
		err := h.service.handleChargeSucceeded(c, &event)
		if err != nil {
			fmt.Println(err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "succeed"})

	default:
		c.JSON(http.StatusContinue, gin.H{"message": "received", "info": fmt.Sprintf("unhandled event type %s", event.Type)})

	}
}
