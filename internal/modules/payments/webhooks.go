package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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
	case "charge.succeeded", "charge.updated":
		var charge stripe.Charge
		if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   fmt.Sprintf("Error parsing Charge JSON: %v\n", err),
				"message": "error",
			})
			return
		}

		// charge.updated fires for many reasons (e.g. receipt URL update).
		// Only process it when the charge has actually succeeded.
		if charge.Status != "succeeded" {
			c.JSON(http.StatusOK, gin.H{"message": "received", "error": "charge status not equals to succeeded"})
			return
		}

		proposal_id := charge.Metadata["proposal_id"]
		logger.Log.Info("stripe charge event received",
			zap.String("event_type", string(event.Type)),
			zap.String("charge_id", charge.ID),
			zap.String("proposal_id", proposal_id),
		)

		// Idempotency: skip if we already recorded a transaction for this charge
		var existing int64
		s.service.db.Model(&models.PaymentTransaction{}).
			Where("charge_id = ?", charge.ID).
			Count(&existing)
		if existing > 0 {
			logger.Log.Info("transaction already exists for charge, skipping", zap.String("charge_id", charge.ID))
			c.JSON(http.StatusOK, gin.H{"message": "received"})
			return
		}

		if proposal_id == "" {
			logger.Log.Warn("charge has no proposal_id in metadata, skipping", zap.String("charge_id", charge.ID))
			c.JSON(http.StatusOK, gin.H{"message": "received"})
			return
		}

		var proposal models.Proposal
		if err := s.service.db.Where("id = ?", proposal_id).Preload("JobPost").First(&proposal).Error; err != nil {
			// Return 400 so Stripe retries — the proposal may not exist yet due to race
			logger.Log.Error("proposal not found for charge", zap.String("proposal_id", proposal_id), zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "proposal not found"})
			return
		}

		logger.Log.Info("processing payment for proposal",
			zap.String("proposal_id", proposal_id),
			zap.String("job_title", proposal.JobPost.Title),
		)

		payment, err := MakeContractPaymentFromCharge(&proposal, &event, &charge)
		if err != nil {
			logger.Log.Error("error building payment transaction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
			return
		}

		// Client is the payer (job creator); freelancer is the recipient
		payerID := proposal.JobPost.CreatedByID
		if err := s.service.ApplyReferralDiscountToPayment(payment, payerID); err != nil {
			logger.Log.Warn("failed to apply referral discount", zap.Error(err))
		}

		tx := s.service.db.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		if err := tx.Create(payment).Error; err != nil {
			tx.Rollback()
			logger.Log.Error("error while creating payment transaction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to record transaction"})
			return
		}

		if err := tx.Commit().Error; err != nil {
			tx.Rollback()
			logger.Log.Error("failed to commit payment transaction", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
			return
		}

		// Process referral reward after successful commit
		if payment.DiscountAmount > 0 {
			if err := s.service.ProcessReferralAfterPayment(payment.ID, payerID, payment.DiscountAmount); err != nil {
				logger.Log.Warn("failed to process referral after payment", zap.Error(err))
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "success"})
		return

	case "payment_intent.amount_capturable_updated":
		// Escrow: funds are authorized (held) — mark contract escrow as "funded"
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			logger.Log.Error("failed to parse payment_intent.amount_capturable_updated", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "JSON parse error"})
			return
		}

		if err := s.service.MarkEscrowFunded(pi.ID); err != nil {
			logger.Log.Warn("failed to mark escrow funded", zap.String("payment_intent_id", pi.ID), zap.Error(err))
		} else {
			logger.Log.Info("escrow marked as funded", zap.String("payment_intent_id", pi.ID))
		}

		// If the contract is in release_pending (both parties completed), capture immediately
		var contract models.Contract
		if err := s.service.db.Where("escrow_payment_intent_id = ? AND escrow_status = ?", pi.ID, "release_pending").
			First(&contract).Error; err == nil {
			if captureErr := s.service.CaptureEscrow(contract.ID); captureErr != nil {
				logger.Log.Error("failed to auto-capture escrow after fund confirmation", zap.Error(captureErr))
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "received"})
		return

	case "payment_intent.succeeded":
		// Escrow capture completed — record the payment transaction
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "received"})
			return
		}

		contractID := pi.Metadata["contract_id"]
		if contractID == "" {
			c.JSON(http.StatusOK, gin.H{"message": "received"})
			return
		}

		var contract models.Contract
		if err := s.service.db.First(&contract, "id = ?", contractID).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "received"})
			return
		}

		settings, _ := GetSystemSettings()
		amount := pi.AmountReceived
		appFee := int64(settings.ApplicationFeeAmount)
		freelancerCommPct := settings.FreelancerCommissionPercentage
		freelancerComm := int64(float64(amount) * freelancerCommPct / 100)
		netAmount := amount - freelancerComm - appFee

		payment := models.PaymentTransaction{
			FromUserID:                     contract.ClientID,
			ToUserID:                       contract.FreelancerID,
			ReferenceType:                  "contract",
			ReferenceID:                    contractID,
			Status:                         models.PaymentStatusSuccess,
			StripeEventID:                  event.ID,
			PaymentIntentID:                pi.ID,
			Amount:                         amount,
			Currency:                       string(pi.Currency),
			PaymentMethod:                  "escrow",
			AppFeeAmount:                   appFee,
			AppFeePercentage:               settings.AppFeePercentage,
			FreelancerCommissionPercentage: freelancerCommPct,
			FreelancerCommissionAmount:     freelancerComm,
			NetAmount:                      netAmount,
			TransactionDate:                time.Unix(event.Created, 0),
		}

		if err := s.service.db.Create(&payment).Error; err != nil {
			logger.Log.Error("failed to create payment transaction for escrow capture", zap.Error(err))
		}

		// Process referral reward
		if err := s.service.ProcessReferralAfterPayment(payment.ID, contract.ClientID, payment.DiscountAmount); err != nil {
			logger.Log.Warn("failed to process referral after escrow capture", zap.Error(err))
		}

		c.JSON(http.StatusOK, gin.H{"message": "received"})
		return

	case "payment_intent.created":
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
	settings, _ := GetSystemSettings()
	totalAmount := contract.TotalAmount
	freelancerCommissionPct := settings.FreelancerCommissionPercentage
	clientCommissionPct := settings.ClientCommissionPercentage
	appFeePct := settings.AppFeePercentage
	referralDiscountPct := settings.ReferralDiscountPercentage

	// Calculate commissions
	freelancerCommission := int64(float64(totalAmount) * freelancerCommissionPct / 100)
	clientCommission := int64(float64(totalAmount) * clientCommissionPct / 100)
	appFee := int64(float64(totalAmount) * appFeePct / 100)

	// Referral discount
	referralDiscount := int64(float64(freelancerCommission+clientCommission) * referralDiscountPct / 100)

	netAmount := int64(totalAmount) - freelancerCommission - clientCommission - appFee + referralDiscount

	metadata, _ := json.Marshal(intent.Metadata)
	var chargeID string
	if intent.LatestCharge != nil {
		chargeID = intent.LatestCharge.ID
	}

	payment := models.PaymentTransaction{
		FromUserID:                     contract.ClientID,
		ToUserID:                       contract.FreelancerID,
		MetaData:                       metadata,
		ReferenceType:                  "contract",
		ReferenceID:                    contract.ID,
		Status:                         models.PaymentStatusSuccess,
		StripeEventID:                  event.ID,
		Amount:                         int64(contract.TotalAmount),
		Currency:                       "gbp",
		PaymentMethod:                  "gateway", // update as needed
		GatewayRefID:                   chargeID,
		FreelancerCommissionPercentage: freelancerCommissionPct,
		FreelancerCommissionAmount:     freelancerCommission,
		ClientCommissionPercentage:     clientCommissionPct,
		ClientCommissionAmount:         clientCommission,
		AppFeePercentage:               appFeePct,
		AppFeeAmount:                   appFee,
		ReferralDiscountPercentage:     referralDiscountPct,
		ReferralDiscountAmount:         referralDiscount,
		NetAmount:                      netAmount,
		PaymentIntentID:                intent.ID,
		ChargeID:                       chargeID,
	}
	return &payment, nil
}

func MakeContractPaymentFromCharge(proposal *models.Proposal, event *stripe.Event, charge *stripe.Charge) (*models.PaymentTransaction, error) {
	settings, _ := GetSystemSettings()
	totalAmount := proposal.BidAmount
	freelancerCommissionPct := settings.FreelancerCommissionPercentage
	clientCommissionPct := settings.ClientCommissionPercentage
	appFeePct := settings.AppFeePercentage
	referralDiscountPct := settings.ReferralDiscountPercentage

	freelancerCommission := int64(float64(totalAmount) * freelancerCommissionPct / 100)
	clientCommission := int64(float64(totalAmount) * clientCommissionPct / 100)
	appFee := int64(float64(totalAmount) * appFeePct / 100)
	referralDiscount := int64(float64(freelancerCommission+clientCommission) * referralDiscountPct / 100)
	netAmount := int64(totalAmount) - freelancerCommission - clientCommission - appFee + referralDiscount

	metadata, _ := json.Marshal(charge.Metadata)
	var paymentIntentID string
	if charge.PaymentIntent != nil {
		paymentIntentID = charge.PaymentIntent.ID
	}
	amount := int64(proposal.BidAmount)

	payment := models.PaymentTransaction{
		// Client (job creator) pays; freelancer receives
		FromUserID:                     proposal.JobPost.CreatedByID,
		ToUserID:                       proposal.FreelancerID,
		MetaData:                       metadata,
		ReferenceType:                  "proposal",
		ReferenceID:                    proposal.ID,
		TransactionDate:                time.Unix(charge.Created, 0),
		Status:                         models.PaymentStatusSuccess,
		StripeEventID:                  event.ID,
		Amount:                         amount,
		Currency:                       "gbp",
		PaymentMethod:                  "card",
		GatewayRefID:                   charge.ID,
		FreelancerCommissionPercentage: freelancerCommissionPct,
		FreelancerCommissionAmount:     freelancerCommission,
		ClientCommissionPercentage:     clientCommissionPct,
		ClientCommissionAmount:         clientCommission,
		AppFeePercentage:               appFeePct,
		AppFeeAmount:                   appFee,
		ReferralDiscountPercentage:     referralDiscountPct,
		ReferralDiscountAmount:         referralDiscount,
		NetAmount:                      netAmount,
		PaymentIntentID:                paymentIntentID,
		ChargeID:                       charge.ID,
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
