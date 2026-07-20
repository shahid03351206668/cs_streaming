package paymentsv3

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84/webhook"
)

func (h *Handler) HandleStripeWebhookV3(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(65536))
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error"})
		return
	}

	webhookSecret := h.config.Stripe.WebhookSecret
	signature := c.GetHeader("Stripe-Signature")

	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	switch event.Type {
	case "charge.succeeded":
		if err := h.service.handleChargeSucceededV3(&event); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "succeed"})

	case "transfer.created":
		if err := h.service.handleTransferPaidV3(&event); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})

	case "charge.refunded":
		if err := h.service.handleChargeRefundedV3(&event); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})

	case "account.updated":
		if err := h.service.handleAccountUpdatedV3(&event); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})

	default:
		c.JSON(http.StatusOK, gin.H{"message": "received", "info": fmt.Sprintf("unhandled event type %s", event.Type)})
	}
}
