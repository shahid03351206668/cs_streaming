package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"tasksy/db"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var CACHED_SYSTEM_SETTINGS *models.SystemSettings

// InvalidateSettingsCache clears the in-process system-settings cache.
// Must be called whenever an admin updates system settings so that subsequent
// payment calculations pick up the new rates without requiring a server restart.
func InvalidateSettingsCache() {
	CACHED_SYSTEM_SETTINGS = nil
}

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
		c.JSON(http.StatusOK, gin.H{"message": "error"})
		return
	}

	endpointSecret := s.service.config.WebhookSecret
	signature := c.GetHeader("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, signature, endpointSecret)

	if err != nil {
		logger.Log.Error("webhook signature verification failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	if s.isEventProcessed(event.ID) {
		logger.Log.Info("duplicate stripe event, skipping", zap.String("event_id", event.ID))
		c.JSON(http.StatusOK, gin.H{"message": "received", "info": "duplicate event"})
		return
	}

	switch event.Type {
	case "charge.succeeded", "charge.updated":
		s.handleChargeSucceeded(c, &event)

	// case "payment_intent.payment_failed":
	// 	s.handlePaymentIntentFailed(c, &event)

	case "charge.refunded":
		s.handleChargeRefunded(c, &event)

	default:
		logger.Log.Info("unhandled stripe event type", zap.String("type", string(event.Type)))
		c.JSON(http.StatusOK, gin.H{"message": "received", "info": fmt.Sprintf("unhandled event type %s", event.Type)})
	}
}

func (s *PaymentHandler) isEventProcessed(eventID string) bool {
	var count int64
	s.service.db.Model(&models.PaymentTransaction{}).
		Where("stripe_event_id = ?", eventID).
		Count(&count)
	if count > 0 {
		return true
	}
	// Also check audit log for non-transaction events (failures, refunds)
	s.service.db.Model(&models.PaymentAuditLog{}).
		Where("stripe_event_id = ?", eventID).
		Count(&count)
	return count > 0
}

func (s *PaymentHandler) handleChargeSucceeded(c *gin.Context, event *stripe.Event) {
	var StripeCharge stripe.Charge
	if err := json.Unmarshal(event.Data.Raw, &StripeCharge); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": fmt.Sprintf("Error parsing Charge JSON: %v", err)})
		return
	}

	if StripeCharge.Status != "succeeded" {
		c.JSON(http.StatusOK, gin.H{"message": "received", "info": "charge status not succeeded"})
		return
	}

	var existing int64
	s.service.db.Model(&models.PaymentTransaction{}).
		Where("charge_id = ?", StripeCharge.ID).
		Count(&existing)
	if existing > 0 {
		logger.Log.Info("transaction already exists for charge, skipping", zap.String("charge_id", StripeCharge.ID))
		c.JSON(http.StatusOK, gin.H{"message": "received", "info": "duplicate charge"})
		return
	}

	proposalID := StripeCharge.Metadata["proposal_id"]
	if proposalID == "" {
		logger.Log.Warn("charge has no proposal_id in metadata, skipping", zap.String("charge_id", StripeCharge.ID))
		c.JSON(http.StatusBadRequest, gin.H{"message": "received", "info": "no proposal_id in metadata"})
		return
	}

	var proposal models.Proposal
	if err := s.service.db.Where("id = ?", proposalID).Preload("JobPost").First(&proposal).Error; err != nil {
		logger.Log.Error("proposal not found for charge", zap.String("proposal_id", proposalID), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "proposal not found"})
		return
	}

	payment, err := MakeContractPaymentFromCharge(&proposal, event, &StripeCharge)
	if err != nil {
		logger.Log.Error("error building payment transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

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
		logger.Log.Error("error creating payment transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to record transaction"})
		return
	}

	var contract models.Contract
	if err := tx.Where("proposal_id = ?", proposalID).First(&contract).Error; err == nil {
		if err := tx.Model(&models.Contract{}).Where("id = ?", contract.ID).
			Update("status", models.ContractStatusActive).Error; err != nil {
			logger.Log.Warn("failed to activate contract after payment",
				zap.String("contract_id", contract.ID), zap.Error(err))
		}

		tx.Model(payment).Updates(map[string]interface{}{
			"reference_type": "contract",
			"reference_id":   contract.ID,
		})
	}

	// Create escrow_fund ledger transaction within the same DB transaction.
	// Debit: External (what Stripe actually charged).
	// If a referral discount was applied, Debit: Marketing (to cover the gap).
	// Credit: System Escrow (full project value = charge + discount).
	{
		ledgerInTx := NewLedgerService(tx)

		escrowAcct, err := ledgerInTx.GetSystemAccount(models.AccountTypeEscrow)
		if err != nil {
			tx.Rollback()
			logger.Log.Error("escrow system account not found", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "escrow account not configured"})
			return
		}
		externalAcct, err := ledgerInTx.GetSystemAccount(models.AccountTypeExternal)
		if err != nil {
			tx.Rollback()
			logger.Log.Error("external system account not found", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "external account not configured"})
			return
		}

		fullEscrowAmount := payment.Amount + payment.DiscountAmount

		entries := []EntryInput{
			{AccountID: externalAcct.ID, Amount: -payment.Amount, Category: "stripe_payment"},
			{AccountID: escrowAcct.ID, Amount: fullEscrowAmount, Category: "escrow_deposit"},
		}

		if payment.DiscountAmount > 0 {
			marketingAcct, err := ledgerInTx.GetSystemAccount(models.AccountTypeMarketing)
			if err != nil {
				tx.Rollback()
				logger.Log.Error("marketing system account not found", zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "marketing account not configured"})
				return
			}
			entries = []EntryInput{
				{AccountID: externalAcct.ID, Amount: -payment.Amount, Category: "stripe_payment"},
				{AccountID: marketingAcct.ID, Amount: -payment.DiscountAmount, Category: "referral_discount_subsidy"},
				{AccountID: escrowAcct.ID, Amount: fullEscrowAmount, Category: "escrow_deposit"},
			}
		}

		contractID := payment.ReferenceID
		if contractID == "" {
			contractID = proposalID
		}

		if _, err := ledgerInTx.CreateLedgerTransaction(
			models.LedgerTxEscrowFund,
			payment.ID,
			fmt.Sprintf("Escrow funded for contract %s (charge %s)", contractID, payment.ChargeID),
			entries,
		); err != nil {
			tx.Rollback()
			logger.Log.Error("failed to create escrow fund ledger entries", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to record ledger entries"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		logger.Log.Error("failed to commit payment transaction", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	// Post-commit: process referral and write audit log
	if payment.DiscountAmount > 0 {
		if err := s.service.ProcessReferralAfterPayment(payment.ID, payerID, payment.DiscountAmount); err != nil {
			logger.Log.Warn("failed to process referral after payment", zap.Error(err))
		}
	}

	s.service.WriteAuditLog(AuditLogEntry{
		EventType:     string(event.Type),
		StripeEventID: event.ID,
		Action:        "payment_created",
		EntityType:    "payment_transaction",
		EntityID:      payment.ID,
		UserID:        payerID,
		Amount:        payment.Amount,
		Status:        payment.Status,
		Details: map[string]interface{}{
			"charge_id":             StripeCharge.ID,
			"proposal_id":           proposalID,
			"gross_amount":          payment.Amount,
			"client_commission":     payment.ClientCommissionAmount,
			"freelancer_commission": payment.FreelancerCommissionAmount,
			"app_fee":               payment.AppFeeAmount,
			"referral_discount":     payment.DiscountAmount,
			"net_amount":            payment.NetAmount,
		},
	})

	logger.Log.Info("payment transaction created from charge",
		zap.String("payment_id", payment.ID),
		zap.Int64("amount", payment.Amount),
		zap.Int64("net_amount", payment.NetAmount),
	)

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (s *PaymentHandler) handleChargeRefunded(c *gin.Context, event *stripe.Event) {
	var charge stripe.Charge
	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "received"})
		return
	}

	// Update payment transaction status to refunded
	result := s.service.db.Model(&models.PaymentTransaction{}).
		Where("charge_id = ?", charge.ID).
		Update("status", models.PaymentStatusRefunded)

	if result.RowsAffected == 0 {
		logger.Log.Warn("no payment transaction found for refunded charge", zap.String("charge_id", charge.ID))
	}

	s.service.WriteAuditLog(AuditLogEntry{
		EventType:     string(event.Type),
		StripeEventID: event.ID,
		Action:        "payment_refunded",
		EntityType:    "payment_transaction",
		EntityID:      charge.ID,
		Amount:        charge.AmountRefunded,
		Status:        "refunded",
		Details: map[string]interface{}{
			"charge_id":       charge.ID,
			"amount_refunded": charge.AmountRefunded,
			"total_amount":    charge.Amount,
		},
	})

	logger.Log.Info("charge refunded",
		zap.String("charge_id", charge.ID),
		zap.Int64("amount_refunded", charge.AmountRefunded),
	)

	c.JSON(http.StatusOK, gin.H{"message": "received"})
}

// // handlePaymentIntentFailed handles failed payment intents.
// // Logs the failure for auditing.
// func (s *PaymentHandler) handlePaymentIntentFailed(c *gin.Context, event *stripe.Event) {
// 	var pi stripe.PaymentIntent
// 	if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
// 		c.JSON(http.StatusOK, gin.H{"message": "received"})
// 		return
// 	}

// 	var failureMessage string
// 	if pi.LastPaymentError != nil {
// 		failureMessage = pi.LastPaymentError.Msg
// 	}

// 	s.service.WriteAuditLog(AuditLogEntry{
// 		EventType:     string(event.Type),
// 		StripeEventID: event.ID,
// 		Action:        "payment_failed",
// 		EntityType:    "payment_intent",
// 		EntityID:      pi.ID,
// 		Amount:        pi.Amount,
// 		Status:        "failed",
// 		Details: map[string]interface{}{
// 			"payment_intent_id": pi.ID,
// 			"failure_message":   failureMessage,
// 		},
// 	})

// 	logger.Log.Warn("payment intent failed",
// 		zap.String("payment_intent_id", pi.ID),
// 		zap.String("failure", failureMessage),
// 	)

// 	c.JSON(http.StatusOK, gin.H{"message": "received"})
// }

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

type AuditLogEntry struct {
	EventType     string
	StripeEventID string
	Action        string
	EntityType    string
	EntityID      string
	UserID        string
	Amount        int64
	Status        string
	Details       map[string]interface{}
}

func (s *PaymentService) WriteAuditLog(entry AuditLogEntry) {
	detailsJSON, _ := json.Marshal(entry.Details)

	log := models.PaymentAuditLog{
		EventType:     entry.EventType,
		StripeEventID: entry.StripeEventID,
		Action:        entry.Action,
		EntityType:    entry.EntityType,
		EntityID:      entry.EntityID,
		UserID:        entry.UserID,
		Amount:        entry.Amount,
		Currency:      "gbp",
		Details:       detailsJSON,
		Status:        entry.Status,
	}

	if err := s.db.Create(&log).Error; err != nil {
		logger.Log.Error("failed to write audit log",
			zap.String("action", entry.Action),
			zap.Error(err),
		)
	}
}

func MakeContractPaymentFromCharge(proposal *models.Proposal, event *stripe.Event, charge *stripe.Charge) (*models.PaymentTransaction, error) {
	settings, _ := GetSystemSettings()
	totalAmount := charge.Amount
	freelancerCommissionPct := settings.FreelancerCommissionPercentage
	clientCommissionPct := settings.ClientCommissionPercentage
	appFee := settings.ApplicationFeeAmount
	freelancerCommission := int64(float64(totalAmount) * freelancerCommissionPct / 100)
	clientCommission := int64(float64(totalAmount) * clientCommissionPct / 100)
	netAmount := totalAmount - freelancerCommission - clientCommission - appFee

	// Extract the referral discount that was already applied before the Stripe charge.
	// This is stored in the PaymentIntent metadata by InitiateEscrow so that the webhook
	// can record the true discount without re-running CalculateReferralDiscount.
	var storedRefDiscount int64
	if refDiscountStr, ok := charge.Metadata["ref_discount"]; ok {
		if v, err := strconv.ParseInt(refDiscountStr, 10, 64); err == nil {
			storedRefDiscount = v
		}
	}

	metadata, _ := json.Marshal(charge.Metadata)
	var paymentIntentID string
	if charge.PaymentIntent != nil {
		paymentIntentID = charge.PaymentIntent.ID
	}

	payment := models.PaymentTransaction{
		FromUserID:                     proposal.JobPost.CreatedByID,
		ToUserID:                       proposal.FreelancerID,
		MetaData:                       metadata,
		ReferenceType:                  "proposal",
		ReferenceID:                    proposal.ID,
		TransactionDate:                time.Unix(charge.Created, 0),
		Status:                         models.PaymentStatusSuccess,
		StripeEventID:                  event.ID,
		Amount:                         totalAmount,
		Currency:                       string(charge.Currency),
		PaymentMethod:                  "card",
		GatewayRefID:                   charge.ID,
		FreelancerCommissionPercentage: freelancerCommissionPct,
		FreelancerCommissionAmount:     freelancerCommission,
		ClientCommissionPercentage:     clientCommissionPct,
		ClientCommissionAmount:         clientCommission,
		AppFeePercentage:               settings.AppFeePercentage,
		AppFeeAmount:                   appFee,
		DiscountAmount:                 storedRefDiscount, // from InitiateEscrow metadata
		NetAmount:                      netAmount,
		PaymentIntentID:                paymentIntentID,
		ChargeID:                       charge.ID,
	}
	return &payment, nil
}

// ApplyReferralDiscountToPayment links the referral code to the payment.
// The actual DiscountAmount is already extracted from the charge metadata in
// MakeContractPaymentFromCharge (from the "ref_discount" field set by InitiateEscrow),
// so we only need to find the matching ReferralUsage and attach its IDs.
func (s *PaymentService) ApplyReferralDiscountToPayment(payment *models.PaymentTransaction, userID string) error {
	// Only proceed if a discount was recorded from the charge metadata.
	if payment.DiscountAmount == 0 {
		return nil
	}

	var usage models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Where("referee_id = ? AND is_qualified = ?", userID, false).First(&usage).Error; err != nil {
		return nil
	}

	// Attach referral identifiers; discount amount and netAmount are already correct.
	payment.ReferralCodeID = &usage.ReferralCode.ID
	payment.ReferrerID = &usage.ReferrerID

	return nil
}

// ProcessReferralAfterPayment marks the referral as qualified after a payment.
func (s *PaymentService) ProcessReferralAfterPayment(paymentID string, userID string, discountApplied int64) error {
	var usage models.ReferralUsage
	if err := s.db.Where("referee_id = ? AND is_qualified = ?", userID, false).First(&usage).Error; err != nil {
		return nil
	}
	return s.MarkReferralAsQualified(&usage, paymentID, discountApplied)
}

// ReleaseContractFunds credits the freelancer's wallet and creates a payout record
// when both parties confirm contract completion. This is called from the CompleteContract
// controller after both client and freelancer have confirmed.
func (s *PaymentService) ReleaseContractFunds(contractID string) error {
	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return fmt.Errorf("contract not found: %w", err)
	}

	// Find the payment transaction for this contract
	var payment models.PaymentTransaction
	if err := s.db.Where("reference_id = ? AND reference_type = ? AND status = ?",
		contractID, "contract", models.PaymentStatusSuccess).
		First(&payment).Error; err != nil {
		return fmt.Errorf("no successful payment found for contract: %w", err)
	}

	// Prevent duplicate payouts
	var existingPayout int64
	s.db.Model(&models.PayoutTransaction{}).
		Where("stripe_id = ?", "contract_"+contractID).
		Count(&existingPayout)
	if existingPayout > 0 {
		return nil // Already processed
	}

	// Calculate freelancer payout: gross amount minus BOTH commissions and the fixed app fee.
	// fullEscrowAmount is what was credited to escrow (including any referral discount subsidy from marketing).
	// Platform revenue = client_commission + freelancer_commission + app_fee + discount_subsidy.
	freelancerPayout := payment.Amount - payment.FreelancerCommissionAmount - payment.ClientCommissionAmount - payment.AppFeeAmount
	if freelancerPayout < 0 {
		freelancerPayout = 0
	}
	fullEscrowAmount := payment.Amount + payment.DiscountAmount
	platformFee := fullEscrowAmount - freelancerPayout

	if txErr := s.db.Transaction(func(tx *gorm.DB) error {
		ledgerInTx := NewLedgerService(tx)

		escrowAcct, err := ledgerInTx.GetSystemAccount(models.AccountTypeEscrow)
		if err != nil {
			return fmt.Errorf("escrow account not found: %w", err)
		}
		revenueAcct, err := ledgerInTx.GetSystemAccount(models.AccountTypeRevenue)
		if err != nil {
			return fmt.Errorf("revenue account not found: %w", err)
		}
		freelancerWallet, err := ledgerInTx.GetOrCreateUserAccount(contract.FreelancerID)
		if err != nil {
			return fmt.Errorf("failed to get freelancer wallet account: %w", err)
		}

		// escrow_release: Debit Escrow, Credit FreelancerWallet + Revenue.
		// Sum: -fullEscrowAmount + freelancerPayout + platformFee = 0 ✓
		if _, err := ledgerInTx.CreateLedgerTransaction(
			models.LedgerTxEscrowRelease,
			contractID,
			fmt.Sprintf("Escrow released for contract %s (freelancer %s)", contractID, contract.FreelancerID),
			[]EntryInput{
				{AccountID: escrowAcct.ID, Amount: -fullEscrowAmount, Category: "escrow_release"},
				{AccountID: freelancerWallet.ID, Amount: freelancerPayout, Category: "freelancer_payout"},
				{AccountID: revenueAcct.ID, Amount: platformFee, Category: "platform_fee"},
			},
		); err != nil {
			return fmt.Errorf("failed to create escrow release ledger entries: %w", err)
		}

		// Create payout transaction record for audit trail.
		payout := models.PayoutTransaction{
			TransactionDate: time.Now(),
			UserID:          contract.FreelancerID,
			Amount:          fullEscrowAmount,
			NetAmount:       freelancerPayout,
			AppFeeAmount:    platformFee,
			Currency:        payment.Currency,
			Status:          models.PaymentStatusSuccess,
			StripeID:        "contract_" + contractID,
		}
		if err := tx.Create(&payout).Error; err != nil {
			return fmt.Errorf("failed to create payout transaction: %w", err)
		}

		// Link payment to payout.
		return tx.Model(&payment).Update("payout_id", payout.ID).Error
	}); txErr != nil {
		return fmt.Errorf("failed to release contract funds: %w", txErr)
	}

	// Re-fetch payout record for the audit log.
	var payout models.PayoutTransaction
	s.db.Where("stripe_id = ?", "contract_"+contractID).First(&payout)

	// Audit log
	s.WriteAuditLog(AuditLogEntry{
		Action:     "freelancer_wallet_credited",
		EntityType: "payout_transaction",
		EntityID:   payout.ID,
		UserID:     contract.FreelancerID,
		Amount:     freelancerPayout,
		Status:     "credited",
		Details: map[string]interface{}{
			"contract_id":           contractID,
			"payment_id":            payment.ID,
			"gross_amount":          payment.Amount,
			"freelancer_commission": payment.FreelancerCommissionAmount,
			"app_fee":               payment.AppFeeAmount,
			"net_credited":          freelancerPayout,
		},
	})

	logger.Log.Info("contract funds released to freelancer",
		zap.String("contract_id", contractID),
		zap.String("freelancer_id", contract.FreelancerID),
		zap.Int64("payout_amount", freelancerPayout),
	)

	return nil
}

// func MakeContractPayment(contract *models.Contract, event *stripe.Event, intent *stripe.PaymentIntent) (*models.PaymentTransaction, error) {
// 	settings, _ := GetSystemSettings()
// 	totalAmount := contract.TotalAmount
// 	freelancerCommissionPct := settings.FreelancerCommissionPercentage
// 	clientCommissionPct := settings.ClientCommissionPercentage
// 	appFeePct := settings.AppFeePercentage
// 	referralDiscountPct := settings.ReferralDiscountPercentage

// 	freelancerCommission := int64(float64(totalAmount) * freelancerCommissionPct / 100)
// 	clientCommission := int64(float64(totalAmount) * clientCommissionPct / 100)
// 	appFee := int64(float64(totalAmount) * appFeePct / 100)
// 	referralDiscount := int64(float64(freelancerCommission+clientCommission) * referralDiscountPct / 100)

// 	netAmount := int64(totalAmount) - freelancerCommission - clientCommission - appFee + referralDiscount

// 	metadata, _ := json.Marshal(intent.Metadata)
// 	var chargeID string
// 	if intent.LatestCharge != nil {
// 		chargeID = intent.LatestCharge.ID
// 	}

// 	payment := models.PaymentTransaction{
// 		FromUserID:                     contract.ClientID,
// 		ToUserID:                       contract.FreelancerID,
// 		MetaData:                       metadata,
// 		ReferenceType:                  "contract",
// 		ReferenceID:                    contract.ID,
// 		Status:                         models.PaymentStatusSuccess,
// 		StripeEventID:                  event.ID,
// 		Amount:                         int64(contract.TotalAmount),
// 		Currency:                       "gbp",
// 		PaymentMethod:                  "gateway",
// 		GatewayRefID:                   chargeID,
// 		FreelancerCommissionPercentage: freelancerCommissionPct,
// 		FreelancerCommissionAmount:     freelancerCommission,
// 		ClientCommissionPercentage:     clientCommissionPct,
// 		ClientCommissionAmount:         clientCommission,
// 		AppFeePercentage:               appFeePct,
// 		AppFeeAmount:                   appFee,
// 		ReferralDiscountPercentage:     referralDiscountPct,
// 		ReferralDiscountAmount:         referralDiscount,
// 		NetAmount:                      netAmount,
// 		PaymentIntentID:                intent.ID,
// 		ChargeID:                       chargeID,
// 	}
// 	return &payment, nil
// }
