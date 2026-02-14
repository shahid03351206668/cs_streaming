package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"tasksy/db"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
	"go.uber.org/zap"
)

var CACHED_SYSTEM_SETTINGS *models.SystemSettings

type PaymentHandler struct {
	service *PaymentService
}

func NewHandler(service *PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

func (s *PaymentHandler) HandlePaymentIntents(c *gin.Context) {
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
		logger.Log.Error("webhook signature verification failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	switch event.Type {
	case "charge.succeeded":
		var charge stripe.Charge

		err := json.Unmarshal(event.Data.Raw, &charge)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   fmt.Sprintf("Error parsing Charge JSON: %v\n", err),
				"message": "error",
			})
			return
		}

		proposal_id := charge.Metadata["proposal_id"]
		fmt.Println(charge.Metadata)

		var proposal *models.Proposal
		if proposal_id != "" {
			s.service.db.Where("id = ?", proposal_id).Preload("JobPost").First(&proposal)
		}

		tx := s.service.db.Begin()

		defer func() {
			if recover() != nil {
				tx.Rollback()
			}
		}()

		if proposal != nil {
			payment, err := MakeContractPaymentFromCharge(proposal, &event, &charge)
			if err != nil {
				logger.Log.Error("error while create payment transaction on stripe webhook", zap.Error(err))
			}

			payerID := proposal.FreelancerID
			if err := s.service.ApplyReferralDiscountToPayment(payment, payerID); err != nil {
				logger.Log.Warn("failed to apply referral discount", zap.Error(err))
			}

			if err := tx.Create(&payment).Error; err != nil {
				logger.Log.Error("error while creating payment transaction", zap.Error(err))
			} else {
				// Mark referral as qualified after successful payment
				if payment.DiscountAmount > 0 {
					if err := s.service.ProcessReferralAfterPayment(payment.ID, payerID, payment.DiscountAmount); err != nil {
						logger.Log.Warn("failed to process referral after payment", zap.Error(err))
					}
				}
			}

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

	case "payment_intent.created", "payment_intent.succeeded", "charge.updated":
		logger.Log.Info("received stripe event", zap.String("type", string(event.Type)))
		c.JSON(http.StatusOK, gin.H{"message": "received"})
		return

	default:
		// Handle unknown event types gracefully
		logger.Log.Info("unhandled stripe event type", zap.String("type", string(event.Type)))
		c.JSON(http.StatusOK, gin.H{"message": "received"})
		return
	}
}

func GetSystemSettings() (*models.SystemSettings, error) {
	if CACHED_SYSTEM_SETTINGS == nil {
		var settings models.SystemSettings
		err := db.DB.First(&settings).Error
		if err == nil {
			CACHED_SYSTEM_SETTINGS = &settings
		}
		return &settings, err
	}
	return CACHED_SYSTEM_SETTINGS, nil
}

func MakeContractPayment(contract *models.Contract, event *stripe.Event, intent *stripe.PaymentIntent) (*models.PaymentTransaction, error) {
	var appFee int64

	settings, _ := GetSystemSettings()

	totalAmount := contract.TotalAmount
	commissionPercentage := settings.ClientCommissionPercentage

	if totalAmount != 0 {
		appFee = int64(totalAmount / 100 * commissionPercentage)
	} else {
		appFee = 0
	}

	metadata, _ := json.Marshal(intent.Metadata)

	// Get ChargeID from the latest charge if available
	var chargeID string
	if intent.LatestCharge != nil {
		chargeID = intent.LatestCharge.ID
	}

	payment := models.PaymentTransaction{
		FromUserID:    contract.ClientID,
		ToUserID:      contract.FreelancerID,
		MetaData:      metadata,
		ReferenceType: "contract",
		ReferenceID:   contract.ID,

		Status:        models.PaymentStatusSuccess,
		AppFeeAmount:  appFee,
		StripeEventID: event.ID,

		Amount:    int64(contract.TotalAmount),
		NetAmount: int64(contract.TotalAmount) - appFee,

		PaymentIntentID: intent.ID,
		ChargeID:        chargeID,
	}

	return &payment, nil
}

func MakeContractPaymentFromCharge(proposal *models.Proposal, event *stripe.Event, charge *stripe.Charge) (*models.PaymentTransaction, error) {
	var appFee int64

	settings, _ := GetSystemSettings()

	totalAmount := proposal.BidAmount
	commissionPercentage := settings.ClientCommissionPercentage

	if totalAmount != 0 {
		appFee = int64(totalAmount / 100 * commissionPercentage)
	} else {
		appFee = 0
	}

	metadata, _ := json.Marshal(charge.Metadata)

	var paymentIntentID string
	if charge.PaymentIntent != nil {
		paymentIntentID = charge.PaymentIntent.ID
	}

	amount := int64(proposal.BidAmount)

	payment := models.PaymentTransaction{
		FromUserID:     proposal.FreelancerID,
		ToUserID:       proposal.JobPost.CreatedByID,
		MetaData:       metadata,
		ReferenceType:  "contract",
		ReferenceID:    proposal.ID,
		DiscountAmount: 0,

		Status:        models.PaymentStatusSuccess,
		AppFeeAmount:  appFee,
		StripeEventID: event.ID,

		Amount:    amount,
		NetAmount: amount - appFee,

		PaymentIntentID: paymentIntentID,
		ChargeID:        charge.ID,
	}

	return &payment, nil
}

func (s *PaymentService) ApplyReferralDiscountToPayment(payment *models.PaymentTransaction, userID string) error {
	var usage models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Where("referee_id = ? AND is_qualified = ?", userID, false).First(&usage).Error; err != nil {
		return nil
	}

	referralCode := usage.ReferralCode
	var discount int64

	if referralCode.DiscountAmount > 0 {
		discount = referralCode.DiscountAmount
	} else if referralCode.DiscountPercentage > 0 {
		discount = (payment.Amount * referralCode.DiscountPercentage) / 100
	}

	if discount > payment.AppFeeAmount {
		discount = payment.AppFeeAmount
	}

	payment.DiscountAmount = discount
	payment.AppFeeAmount = payment.AppFeeAmount - discount
	payment.NetAmount = payment.Amount - payment.AppFeeAmount
	payment.ReferralCodeID = &referralCode.ID

	return nil
}

func (s *PaymentService) ProcessReferralAfterPayment(paymentID string, userID string, discountApplied int64) error {
	var usage models.ReferralUsage
	if err := s.db.Where("referee_id = ? AND is_qualified = ?", userID, false).First(&usage).Error; err != nil {
		// No pending referral
		return nil
	}

	return s.MarkReferralAsQualified(&usage, paymentID, discountApplied)
}
