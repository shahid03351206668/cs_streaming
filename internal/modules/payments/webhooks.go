package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
)

type StripePaymentHandler struct {
	service *PaymentService
}

func NewHandler(service *PaymentService) *StripePaymentHandler {
	return &StripePaymentHandler{service: service}
}

func (s *StripePaymentHandler) HandlePaymentIntents(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(65536))
	payload, err := io.ReadAll(c.Request.Body)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "error",
		})
		return
	}

	endpointSecret := s.service.config.WebhookSecret
	signature := c.GetHeader("Stripe-Signature")

	event, err := webhook.ConstructEvent(payload, signature, endpointSecret)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var intent stripe.PaymentIntent

		err := json.Unmarshal(event.Data.Raw, &intent)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   fmt.Sprintf("Error parsing PaymentIntent JSON: %v\n", err),
				"message": "error",
			})
			return
		}

		receiptURL, cardBrand, last4 := "", "", ""

		fmt.Println("intent.Amount")
		fmt.Println(intent.Amount)

		appFee := int64((intent.Amount / 100) * 10)
		userID := intent.Metadata["user_id"]
		jobID := intent.Metadata["job_id"]
		netAmount := intent.Amount - appFee

		if intent.LatestCharge != nil &&

			intent.LatestCharge.PaymentMethodDetails != nil &&
			intent.LatestCharge.PaymentMethodDetails.Card != nil {
			cardBrand = string(intent.LatestCharge.PaymentMethodDetails.Card.Brand)
			last4 = intent.LatestCharge.PaymentMethodDetails.Card.Last4
			receiptURL = intent.LatestCharge.ReceiptURL

		} else {
			fmt.Println("Card details missing in webhook, using defaults.")
		}

		payment := models.Payment{
			UserID:               userID,
			JobID:                jobID,
			PaymentIntentID:      intent.ID,
			StripeEventID:        event.ID,
			ChargeID:             intent.LatestCharge.ID,
			Amount:               intent.Amount,
			ApplicationFeeAmount: appFee,
			NetAmount:            netAmount,
			Currency:             string(intent.Currency),
			Status:               "succeeded",
			ReceiptURL:           receiptURL,
			CardBrand:            cardBrand,
			Last4:                last4,
		}

		tx := s.service.db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		if err := tx.Create(&payment).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusForbidden, gin.H{
				"message": "error",
				"error":   err.Error(),
			})
			return
		}

		if err := tx.Commit().Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusForbidden, gin.H{
				"message": "error",
				"error":   err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "success"})
		return
		// fmt.Println("payment created successfully was attached to a Customer!")

	}
}
