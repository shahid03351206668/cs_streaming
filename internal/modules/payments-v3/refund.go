package paymentsv3

import (
	"errors"
	"fmt"
	"time"

	"tasksy/models"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/refund"
	"github.com/stripe/stripe-go/v84/transferreversal"
)

var (
	ErrAlreadyRefunded      = errors.New("this payment has already been refunded")
	ErrRefundStateChanged   = errors.New("payment state changed while processing; please retry")
	ErrNoPaymentForContract = errors.New("no payment found for this contract")
)

const escrowRefunding = "refunding"

// AdminRefundContract refunds the client for a contract's payment. If the
// escrow was already released to the freelancer, the Stripe transfer is
// reversed first (pulling the money back from their connected balance) before
// the original charge is refunded — otherwise the platform would pay out to
// the freelancer AND refund the client, losing the full amount twice over.
//
// If the freelancer has already withdrawn the funds, the connected account
// won't have enough balance to reverse and this returns a clear error rather
// than proceeding with a refund that leaves the platform short.
func (s *Service) AdminRefundContract(contractID, adminID, reason string) (*models.PaymentTransactionV3, error) {
	var payment models.PaymentTransactionV3
	if err := s.db.Where("contract_id = ?", contractID).Order("created_at DESC").First(&payment).Error; err != nil {
		return nil, ErrNoPaymentForContract
	}
	if payment.Status == "refunded" {
		return nil, ErrAlreadyRefunded
	}

	var escrow models.EscrowTransactionV3
	if err := s.db.Where("payment_transaction_id = ?", payment.ID).First(&escrow).Error; err != nil {
		return nil, fmt.Errorf("escrow not found: %w", err)
	}
	if escrow.Status == "refunded" {
		return nil, ErrAlreadyRefunded
	}
	if escrow.Status != escrowHeld && escrow.Status != escrowReleased {
		return nil, ErrRefundStateChanged
	}

	// Claim the escrow so a concurrent release/refund can't race this one —
	// same pattern as releaseEscrow.
	wasReleased := escrow.Status == escrowReleased
	claim := s.db.Model(&models.EscrowTransactionV3{}).
		Where("id = ? AND status = ?", escrow.ID, escrow.Status).
		Update("status", escrowRefunding)
	if claim.Error != nil {
		return nil, claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil, ErrRefundStateChanged
	}

	unclaim := func(back string) {
		s.db.Model(&models.EscrowTransactionV3{}).
			Where("id = ? AND status = ?", escrow.ID, escrowRefunding).
			Update("status", back)
	}

	var reversalID string
	if wasReleased {
		params := &stripe.TransferReversalParams{ID: stripe.String(escrow.StripeTransferID)}
		rev, err := transferreversal.New(params)
		if err != nil {
			unclaim(escrowReleased)
			return nil, fmt.Errorf("failed to reverse stripe transfer (the freelancer may have already withdrawn the funds): %w", err)
		}
		reversalID = rev.ID
	}

	ref, err := refund.New(&stripe.RefundParams{
		PaymentIntent: stripe.String(payment.StripePaymentIntentID),
	})
	if err != nil {
		unclaim(escrow.Status)
		return nil, fmt.Errorf("stripe refund failed: %w", err)
	}

	now := time.Now()
	if err := s.db.Model(&payment).Updates(map[string]any{
		"status":               "refunded",
		"stripe_refund_id":     ref.ID,
		"stripe_reversal_id":   reversalID,
		"refunded_by_admin_id": adminID,
		"refund_reason":        reason,
		"refunded_at":          &now,
	}).Error; err != nil {
		return nil, fmt.Errorf("refunded on stripe (refund %s) but failed to record it: %w", ref.ID, err)
	}
	if err := s.db.Model(&escrow).Update("status", "refunded").Error; err != nil {
		return nil, fmt.Errorf("refunded on stripe (refund %s) but failed to update escrow: %w", ref.ID, err)
	}

	s.db.First(&payment, "id = ?", payment.ID)
	return &payment, nil
}
