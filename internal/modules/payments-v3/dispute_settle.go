package paymentsv3

import (
	"errors"

	"tasksy/models"

	"gorm.io/gorm"
)

const (
	PayoutNone                  = "none"
	PayoutReleased              = "released_to_freelancer"
	PayoutAlreadyReleased       = "already_released"
	PayoutRefundedFull          = "refunded_in_full"
	PayoutRefundedExcludingFees = "refunded_excluding_fees"
	PayoutAlreadyRefunded       = "already_refunded"
)

// Ignores the open dispute (it is the one being closed) and is safe to call again after a partial failure.
func (s *Service) SettleDispute(contractID, adminID, note, outcome string, refundFees bool) (string, error) {
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
		return s.settleForClient(contractID, adminID, note, refundFees, &payment)
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

func (s *Service) settleForClient(contractID, adminID, note string, refundFees bool, payment *models.PaymentTransactionV3) (string, error) {
	if payment.Status == escrowRefunded {
		return PayoutAlreadyRefunded, nil
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
