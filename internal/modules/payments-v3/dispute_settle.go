package paymentsv3

import (
	"errors"

	"tasksy/models"

	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

const (
	PayoutNone                  = "none"
	PayoutReleased              = "released_to_freelancer"
	PayoutAlreadyReleased       = "already_released"
	PayoutRefundedFull          = "refunded_in_full"
	PayoutRefundedExcludingFees = "refunded_excluding_fees"
	PayoutRefundedAmount        = "refunded_custom_amount"
	PayoutAlreadyRefunded       = "already_refunded"
)

// Ignores the open dispute (it is the one being closed) and is safe to call again after a partial failure.
func (s *Service) SettleDispute(contractID, adminID, note, outcome string, refundFees bool, amount *decimal.Decimal) (string, error) {
	var payment models.PaymentTransactionV3
	if err := s.db.Where("contract_id = ?", contractID).Order("created_at DESC").First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PayoutNone, nil
		}
		return "", err
	}

	switch outcome {
	case models.DisputeOutcomeFreelancer:
		return s.settleForFreelancer(contractID, adminID, note, &payment)
	case models.DisputeOutcomeClient:
		return s.settleForClient(contractID, adminID, note, refundFees, amount, &payment)
	}
	return PayoutNone, nil
}

func (s *Service) settleForFreelancer(contractID, adminID, note string, payment *models.PaymentTransactionV3) (string, error) {
	var escrow models.EscrowTransactionV3
	if err := s.db.Where("payment_transaction_id = ?", payment.ID).First(&escrow).Error; err != nil {
		return "", err
	}

	switch escrow.Status {
	case escrowReleased:
		return PayoutAlreadyReleased, nil
	case escrowRefunded:
		return "", ErrAlreadyRefunded
	case escrowChargeback:
		return "", ErrChargebackOpen
	case escrowHeld:
	default:
		return "", ErrRefundStateChanged
	}

	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return "", err
	}
	if contract.Status == models.ContractStatusCancelled || contract.Status == models.ContractStatusTerminated {
		return "", ErrContractClosed
	}
	if err := s.releaseHeld(&escrow, &contract, adminID, note); err != nil {
		return "", err
	}
	return PayoutReleased, nil
}

func (s *Service) settleForClient(contractID, adminID, note string, refundFees bool, amount *decimal.Decimal, payment *models.PaymentTransactionV3) (string, error) {
	if payment.Status == escrowRefunded {
		return PayoutAlreadyRefunded, nil
	}

	if amount != nil {
		if _, err := s.AdminRefundContract(contractID, adminID, note, amount); err != nil {
			return "", err
		}
		if amount.Round(2).Equal(payment.GrossAmount) {
			return PayoutRefundedFull, nil
		}
		return PayoutRefundedAmount, nil
	}

	action := PayoutRefundedFull
	if refundFees {
		if _, err := s.AdminRefundContract(contractID, adminID, note, nil); err != nil {
			return "", err
		}
		return action, nil
	}

	bid := payment.GrossAmount.Sub(payment.ClientFee)
	if !bid.IsPositive() {
		bid = payment.GrossAmount
	}
	if bid.LessThan(payment.GrossAmount) {
		action = PayoutRefundedExcludingFees
	}
	if _, err := s.AdminRefundContract(contractID, adminID, note, &bid); err != nil {
		return "", err
	}
	return action, nil
}

type DisputePayment struct {
	HasPayment      bool    `json:"has_payment"`
	PaymentID       string  `json:"payment_id,omitempty"`
	Status          string  `json:"status,omitempty"`
	Currency        string  `json:"currency,omitempty"`
	ClientPaid      float64 `json:"client_paid"`
	Bid             float64 `json:"bid"`
	FreelancerNet   float64 `json:"freelancer_net"`
	PlatformFees    float64 `json:"platform_fees"`
	AlreadyRefunded float64 `json:"already_refunded"`
	MaxRefundable   float64 `json:"max_refundable"`
	EscrowStatus    string  `json:"escrow_status,omitempty"`
	CanRefund       bool    `json:"can_refund"`
}

// DisputePaymentSummary shows what the client paid and what an admin can still refund.
func (s *Service) DisputePaymentSummary(contractID string) (*DisputePayment, error) {
	var payment models.PaymentTransactionV3
	if err := s.db.Where("contract_id = ?", contractID).Order("created_at DESC").First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &DisputePayment{}, nil
		}
		return nil, err
	}
	var escrow models.EscrowTransactionV3
	s.db.Where("payment_transaction_id = ?", payment.ID).First(&escrow)

	f := func(d decimal.Decimal) float64 { return d.InexactFloat64() }
	out := &DisputePayment{
		HasPayment:      true,
		PaymentID:       payment.ID,
		Status:          payment.Status,
		Currency:        payment.Currency,
		ClientPaid:      f(payment.GrossAmount),
		Bid:             f(payment.GrossAmount.Sub(payment.ClientFee)),
		FreelancerNet:   f(payment.NetAmount),
		PlatformFees:    f(payment.PlatformFee),
		AlreadyRefunded: f(payment.RefundedAmount),
		EscrowStatus:    escrow.Status,
	}
	if payment.Status != escrowRefunded && (escrow.Status == escrowHeld || escrow.Status == escrowReleased) {
		out.CanRefund = true
		out.MaxRefundable = f(payment.GrossAmount)
	}
	return out, nil
}
