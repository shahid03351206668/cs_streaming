package paymentsv3

import (
	"encoding/json"
	"errors"
	"time"

	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/paymentintent"
	"github.com/stripe/stripe-go/v84/refund"
	"go.uber.org/zap"
)

var ErrChargebackOpen = errors.New("the client's bank is disputing this card payment; wait for the chargeback to close")

func (s *Service) chargebackPayment(event *stripe.Event) (*stripe.Dispute, *models.PaymentTransactionV3, error) {
	var d stripe.Dispute
	if err := json.Unmarshal(event.Data.Raw, &d); err != nil {
		return nil, nil, err
	}
	if d.Charge == nil {
		return &d, nil, nil
	}
	var payment models.PaymentTransactionV3
	if err := s.db.Where("stripe_charge_id = ?", d.Charge.ID).First(&payment).Error; err != nil {
		return &d, nil, nil
	}
	return &d, &payment, nil
}

// A held escrow is frozen while the bank disputes the card payment, so the
// freelancer isn't paid money the bank may take back.
func (s *Service) handleChargebackOpenedV3(event *stripe.Event) error {
	d, payment, err := s.chargebackPayment(event)
	if err != nil || payment == nil {
		return err
	}

	var escrow models.EscrowTransactionV3
	if err := s.db.Where("payment_transaction_id = ?", payment.ID).First(&escrow).Error; err != nil {
		return err
	}
	switch escrow.Status {
	case escrowHeld:
		return s.db.Model(&models.EscrowTransactionV3{}).
			Where("id = ? AND status = ?", escrow.ID, escrowHeld).
			Update("status", escrowChargeback).Error
	case escrowReleasing:
		return errors.New("release in progress; retry once it settles")
	case escrowReleased:
		logger.Log.Error("chargeback opened after the freelancer was already paid",
			zap.String("transaction_id", payment.ID), zap.String("dispute_id", d.ID), zap.Int64("amount_pence", d.Amount))
	}
	return nil
}

func (s *Service) handleChargebackClosedV3(event *stripe.Event) error {
	d, payment, err := s.chargebackPayment(event)
	if err != nil || payment == nil {
		return err
	}

	if d.Status != stripe.DisputeStatusLost {
		return s.db.Model(&models.EscrowTransactionV3{}).
			Where("payment_transaction_id = ? AND status = ?", payment.ID, escrowChargeback).
			Update("status", escrowHeld).Error
	}

	now := time.Now()
	tx := s.db.Begin()
	if err := tx.Model(&models.PaymentTransactionV3{}).Where("id = ? AND status <> ?", payment.ID, escrowRefunded).
		Updates(map[string]any{
			"status":          escrowRefunded,
			"refunded_amount": payment.GrossAmount,
			"refund_reason":   "chargeback lost: the client's bank returned the payment",
			"refunded_at":     &now,
		}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Model(&models.EscrowTransactionV3{}).
		Where("payment_transaction_id = ? AND status = ?", payment.ID, escrowChargeback).
		Update("status", escrowRefunded).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// RefundUnrecordedProposalPayment refunds a client who paid for a proposal
// that was then rejected or withdrawn before a contract existed; with no
// contract the payment was never recorded, so nothing else would return it.
func (s *Service) RefundUnrecordedProposalPayment(proposalID string) error {
	var proposal models.Proposal
	if err := s.db.Select("id", "stripe_payment_intent_id").First(&proposal, "id = ?", proposalID).Error; err != nil {
		return err
	}
	if proposal.StripePaymentIntentID == "" || s.paymentRecorded(proposal.StripePaymentIntentID) {
		return nil
	}

	pi, err := paymentintent.Get(proposal.StripePaymentIntentID, nil)
	if err != nil {
		return err
	}
	switch pi.Status {
	case stripe.PaymentIntentStatusSucceeded:
		params := &stripe.RefundParams{PaymentIntent: stripe.String(pi.ID)}
		params.SetIdempotencyKey("v3-closed-proposal-refund-" + pi.ID)
		_, err := refund.New(params)
		return err
	case stripe.PaymentIntentStatusCanceled:
		return nil
	default:
		_, err := paymentintent.Cancel(pi.ID, nil)
		return err
	}
}
