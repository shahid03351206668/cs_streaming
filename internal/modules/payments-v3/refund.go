package paymentsv3

import (
	"errors"
	"fmt"
	"time"

	"tasksy/models"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/refund"
	"github.com/stripe/stripe-go/v84/transferreversal"
)

var (
	ErrAlreadyRefunded      = errors.New("this payment has already been refunded")
	ErrRefundStateChanged   = errors.New("payment state changed while processing; please retry")
	ErrNoPaymentForContract = errors.New("no payment found for this contract")
	ErrInvalidRefundAmount  = errors.New("refund amount must be greater than zero and no more than the amount the client paid")
)

const escrowRefunding = "refunding"

// A nil amount refunds everything; a released escrow's transfer is reversed first, and if that fails nothing is refunded.
func (s *Service) AdminRefundContract(contractID, adminID, reason string, amount *decimal.Decimal) (*models.PaymentTransactionV3, error) {
	var payment models.PaymentTransactionV3
	if err := s.db.Where("contract_id = ?", contractID).Order("created_at DESC").First(&payment).Error; err != nil {
		return nil, ErrNoPaymentForContract
	}
	if payment.Status == escrowRefunded {
		return nil, ErrAlreadyRefunded
	}

	refundAmount := payment.GrossAmount
	if amount != nil {
		refundAmount = amount.Round(2)
		if !refundAmount.IsPositive() || refundAmount.GreaterThan(payment.GrossAmount) {
			return nil, ErrInvalidRefundAmount
		}
	}

	var escrow models.EscrowTransactionV3
	if err := s.db.Where("payment_transaction_id = ?", payment.ID).First(&escrow).Error; err != nil {
		return nil, fmt.Errorf("escrow not found: %w", err)
	}
	if escrow.Status == escrowRefunded {
		return nil, ErrAlreadyRefunded
	}
	if escrow.Status != escrowHeld && escrow.Status != escrowReleased {
		return nil, ErrRefundStateChanged
	}

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
		rev, err := transferreversal.New(&stripe.TransferReversalParams{ID: stripe.String(escrow.StripeTransferID)})
		if err != nil {
			unclaim(escrowReleased)
			return nil, fmt.Errorf("failed to reverse stripe transfer: %w", err)
		}
		reversalID = rev.ID
	}

	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(payment.StripePaymentIntentID),
		Amount:        stripe.Int64(ToPence(refundAmount)),
	}
	params.SetIdempotencyKey(fmt.Sprintf("v3-refund-%s-%d", payment.ID, ToPence(refundAmount)))
	ref, err := refund.New(params)
	if err != nil {
		unclaim(escrow.Status)
		return nil, fmt.Errorf("stripe refund failed: %w", err)
	}

	now := time.Now()
	if err := s.db.Model(&payment).Updates(map[string]any{
		"status":               escrowRefunded,
		"refunded_amount":      refundAmount,
		"stripe_refund_id":     ref.ID,
		"stripe_reversal_id":   reversalID,
		"refunded_by_admin_id": adminID,
		"refund_reason":        reason,
		"refunded_at":          &now,
	}).Error; err != nil {
		return nil, fmt.Errorf("refunded on stripe (refund %s) but failed to record it: %w", ref.ID, err)
	}
	if err := s.db.Model(&escrow).Update("status", escrowRefunded).Error; err != nil {
		return nil, fmt.Errorf("refunded on stripe (refund %s) but failed to update escrow: %w", ref.ID, err)
	}

	s.db.First(&payment, "id = ?", payment.ID)
	return &payment, nil
}
