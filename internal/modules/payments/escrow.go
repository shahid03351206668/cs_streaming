package payments

import (
	"errors"
	"fmt"
	"tasksy/models"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/paymentintent"
)

const (
	EscrowStatusPending  = "pending"
	EscrowStatusFunded   = "funded"
	EscrowStatusReleased = "released"
	EscrowStatusRefunded = "refunded"
	EscrowStatusFailed   = "failed"
)

// InitiateEscrow creates a Stripe PaymentIntent with manual capture for the given contract.
// Returns the client_secret for the frontend to confirm the payment.
func (s *PaymentService) InitiateEscrow(contractID string, clientID string) (string, int64, error) {
	stripe.Key = s.config.SecretKey

	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return "", 0, errors.New("contract not found")
	}

	if contract.ClientID != clientID {
		return "", 0, errors.New("only the client can deposit escrow funds")
	}

	if contract.Status == models.ContractStatusCompleted || contract.Status == models.ContractStatusCancelled {
		return "", 0, fmt.Errorf("cannot initiate escrow for contract in status: %s", contract.Status)
	}

	if contract.EscrowStatus == EscrowStatusFunded || contract.EscrowStatus == EscrowStatusReleased {
		return "", 0, errors.New("escrow is already funded or released for this contract")
	}

	settings, _ := GetSystemSettings()
	amountInCents := int64(contract.TotalAmount * 100)

	// Apply promotional discount if eligible
	promoDiscount := s.GetPromotionalDiscount(clientID, amountInCents)
	finalAmount := amountInCents - promoDiscount
	if finalAmount < 50 { // Stripe minimum is 50 cents
		finalAmount = 50
	}

	// Apply referral discount
	refDiscount, _, _ := s.CalculateReferralDiscount(clientID, finalAmount)
	finalAmount -= refDiscount
	if finalAmount < 50 {
		finalAmount = 50
	}

	appFee := int64(settings.ApplicationFeeAmount)

	params := &stripe.PaymentIntentParams{
		Amount:        stripe.Int64(finalAmount),
		Currency:      stripe.String("gbp"),
		CaptureMethod: stripe.String(string(stripe.PaymentIntentCaptureMethodManual)),
		Metadata: map[string]string{
			"contract_id":     contractID,
			"client_id":       clientID,
			"original_amount": fmt.Sprintf("%d", amountInCents),
			"promo_discount":  fmt.Sprintf("%d", promoDiscount),
			"ref_discount":    fmt.Sprintf("%d", refDiscount),
			"app_fee":         fmt.Sprintf("%d", appFee),
		},
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create payment intent: %w", err)
	}

	// Persist the PaymentIntent ID on the contract
	if err := s.db.Model(&models.Contract{}).Where("id = ?", contractID).Updates(map[string]interface{}{
		"escrow_payment_intent_id": pi.ID,
		"escrow_status":            EscrowStatusPending,
		"escrow_amount":            finalAmount,
	}).Error; err != nil {
		return "", 0, err
	}

	return pi.ClientSecret, finalAmount, nil
}

// MarkEscrowFunded updates the contract escrow status to "funded" once Stripe confirms authorization.
func (s *PaymentService) MarkEscrowFunded(paymentIntentID string) error {
	return s.db.Model(&models.Contract{}).
		Where("escrow_payment_intent_id = ?", paymentIntentID).
		Update("escrow_status", EscrowStatusFunded).Error
}

// CaptureEscrow captures the held funds and releases them to the freelancer.
// The actual payment transaction and wallet credit happen in the payment_intent.succeeded webhook,
// which is the single source of truth for payment records.
func (s *PaymentService) CaptureEscrow(contractID string) error {
	stripe.Key = s.config.SecretKey

	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return errors.New("contract not found")
	}

	if contract.EscrowPaymentIntentID == "" {
		return errors.New("no escrow payment intent for this contract")
	}

	if contract.EscrowStatus != EscrowStatusFunded && contract.EscrowStatus != "release_pending" {
		return fmt.Errorf("escrow is not in funded/release_pending state (current: %s)", contract.EscrowStatus)
	}

	_, err := paymentintent.Capture(contract.EscrowPaymentIntentID, nil)
	if err != nil {
		s.WriteAuditLog(AuditLogEntry{
			Action:     "escrow_capture_failed",
			EntityType: "contract",
			EntityID:   contractID,
			UserID:     contract.FreelancerID,
			Amount:     contract.EscrowAmount,
			Status:     "failed",
			Details:    map[string]interface{}{"error": err.Error()},
		})
		return fmt.Errorf("failed to capture payment: %w", err)
	}

	// Mark escrow as released — the payment_intent.succeeded webhook
	// will handle creating the payment transaction and crediting the freelancer wallet.
	return s.db.Model(&models.Contract{}).Where("id = ?", contractID).
		Update("escrow_status", EscrowStatusReleased).Error
}

// RefundEscrow cancels the PaymentIntent, returning funds to the client.
func (s *PaymentService) RefundEscrow(contractID string) error {
	stripe.Key = s.config.SecretKey

	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return errors.New("contract not found")
	}

	if contract.EscrowPaymentIntentID == "" {
		return errors.New("no escrow payment intent for this contract")
	}

	if contract.EscrowStatus == EscrowStatusReleased {
		return errors.New("escrow already released, cannot refund")
	}

	_, err := paymentintent.Cancel(contract.EscrowPaymentIntentID, nil)
	if err != nil {
		return fmt.Errorf("failed to cancel payment intent: %w", err)
	}

	return s.db.Model(&models.Contract{}).Where("id = ?", contractID).
		Update("escrow_status", EscrowStatusRefunded).Error
}

// GetEscrowStatus returns the escrow status for a contract.
func (s *PaymentService) GetEscrowStatus(contractID string) (map[string]interface{}, error) {
	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return nil, errors.New("contract not found")
	}

	result := map[string]interface{}{
		"contract_id":             contractID,
		"escrow_status":           contract.EscrowStatus,
		"escrow_amount":           contract.EscrowAmount,
		"escrow_payment_intent_id": contract.EscrowPaymentIntentID,
	}

	return result, nil
}

// GetPromotionalDiscount calculates the best promotional discount for the user.
func (s *PaymentService) GetPromotionalDiscount(userID string, amount int64) int64 {
	// Count user's completed jobs (as freelancer) and posted jobs (as client)
	var completedJobs int64
	s.db.Model(&models.Contract{}).
		Where("freelancer_id = ? AND status = ?", userID, models.ContractStatusCompleted).
		Count(&completedJobs)

	var postedJobs int64
	s.db.Model(&models.JobPost{}).
		Where("created_by_id = ? AND status = ?", userID, models.JobStatusCompleted).
		Count(&postedJobs)

	var offers []models.PromotionalOffer
	s.db.Where("is_active = ? AND (expires_at IS NULL OR expires_at > NOW())", true).
		Order("discount_percentage DESC, discount_amount DESC").
		Find(&offers)

	var bestDiscount int64
	for _, offer := range offers {
		qualifies := true
		if offer.MinJobsCompleted > 0 && int(completedJobs) < offer.MinJobsCompleted {
			qualifies = false
		}
		if offer.MinJobsPosted > 0 && int(postedJobs) < offer.MinJobsPosted {
			qualifies = false
		}
		if !qualifies {
			continue
		}

		var discount int64
		if offer.DiscountAmount > 0 {
			discount = offer.DiscountAmount
		} else if offer.DiscountPercentage > 0 {
			discount = int64(float64(amount) * offer.DiscountPercentage / 100)
		}

		if offer.MaxDiscountAmount > 0 && discount > offer.MaxDiscountAmount {
			discount = offer.MaxDiscountAmount
		}

		if discount > bestDiscount {
			bestDiscount = discount
		}
	}

	return bestDiscount
}
