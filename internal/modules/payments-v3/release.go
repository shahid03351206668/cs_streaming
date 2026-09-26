package paymentsv3

import (
	"errors"
	"fmt"
	"time"

	"tasksy/models"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/transfer"
	"gorm.io/gorm"
)

const (
	escrowHeld      = "held"
	escrowReleasing = "releasing"
	escrowReleased  = "released"
)

var (
	ErrContractNotCompleted = errors.New("both parties must mark the contract complete before payment is released")
	ErrContractDisputed     = errors.New("contract has an open dispute; payment is on hold until it is resolved")
	ErrContractClosed       = errors.New("payment cannot be released for a cancelled or terminated contract")
	ErrNothingToRelease     = errors.New("no held payment to release for this contract")
	ErrNotContractParty     = errors.New("you are not a party to this contract")
)

// ReleaseContractFunds transfers every held escrow on a contract to the
// freelancer's Connect account. It checks the completion flags rather than
// contract.status, because dispute resolution can reset status to "active".
func (s *Service) ReleaseContractFunds(contractID string) (int, error) {
	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return 0, fmt.Errorf("contract not found: %w", err)
	}

	if err := s.checkReleasable(&contract, true); err != nil {
		return 0, err
	}

	var escrows []models.EscrowTransactionV3
	if err := s.db.Where("contract_id = ? AND status = ?", contractID, escrowHeld).Find(&escrows).Error; err != nil {
		return 0, err
	}
	if len(escrows) == 0 {
		return 0, ErrNothingToRelease
	}

	released := 0
	for i := range escrows {
		ok, err := s.releaseEscrow(&escrows[i], &contract)
		if err != nil {
			return released, err
		}
		if ok {
			released++
		}
	}
	return released, nil
}

// checkReleasable applies the release rules. requireCompletion is false only
// for an admin override, which may release before both parties confirm but
// never while a dispute is open or the contract is cancelled/terminated.
func (s *Service) checkReleasable(contract *models.Contract, requireCompletion bool) error {
	if contract.Status == models.ContractStatusCancelled || contract.Status == models.ContractStatusTerminated {
		return ErrContractClosed
	}

	openDisputes, err := s.openDisputeCount(contract.ID)
	if err != nil {
		return err
	}
	if openDisputes > 0 || contract.Status == models.ContractStatusDisputed {
		return ErrContractDisputed
	}

	if requireCompletion && (!contract.ClientCompleted || !contract.FreelancerCompleted) {
		return ErrContractNotCompleted
	}
	return nil
}

func (s *Service) openDisputeCount(contractID string) (int64, error) {
	var n int64
	err := s.db.Model(&models.Dispute{}).
		Where("contract_id = ? AND status = ?", contractID, models.DisputeStatusOpen).
		Count(&n).Error
	return n, err
}

// AdminReleaseEscrow force-releases one escrow on an admin's decision (e.g. a
// client who never confirms completion). A note is required for the audit trail.
func (s *Service) AdminReleaseEscrow(escrowID, adminID, note string) error {
	var escrow models.EscrowTransactionV3
	if err := s.db.First(&escrow, "id = ?", escrowID).Error; err != nil {
		return fmt.Errorf("escrow not found: %w", err)
	}
	if escrow.Status != escrowHeld {
		return fmt.Errorf("%w (current status: %s)", ErrNothingToRelease, escrow.Status)
	}

	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", escrow.ContractID).Error; err != nil {
		return fmt.Errorf("contract not found: %w", err)
	}
	if err := s.checkReleasable(&contract, false); err != nil {
		return err
	}

	ok, err := s.releaseEscrow(&escrow, &contract)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNothingToRelease
	}

	return s.db.Model(&models.EscrowTransactionV3{}).Where("id = ?", escrow.ID).
		Updates(map[string]any{"released_by": adminID, "release_note": note}).Error
}

// ReleaseEscrow is the manual/retry entry point by escrow ID; either contract
// party may trigger it, and the same completion/dispute rules apply.
func (s *Service) ReleaseEscrow(escrowID, requestingUserID string) (int, error) {
	var escrow models.EscrowTransactionV3
	if err := s.db.First(&escrow, "id = ?", escrowID).Error; err != nil {
		return 0, fmt.Errorf("escrow not found: %w", err)
	}
	return s.ReleaseForParty(escrow.ContractID, requestingUserID)
}

func (s *Service) ReleaseForParty(contractID, requestingUserID string) (int, error) {
	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		return 0, fmt.Errorf("contract not found: %w", err)
	}
	if requestingUserID != contract.ClientID && requestingUserID != contract.FreelancerID {
		return 0, ErrNotContractParty
	}
	return s.ReleaseContractFunds(contractID)
}

// releaseEscrow moves one escrow held -> releasing -> released. The conditional
// claim means only one concurrent caller can create the transfer, and the
// per-escrow idempotency key stops a retry from paying twice.
func (s *Service) releaseEscrow(e *models.EscrowTransactionV3, contract *models.Contract) (bool, error) {
	claim := s.db.Model(&models.EscrowTransactionV3{}).
		Where("id = ? AND status = ?", e.ID, escrowHeld).
		Update("status", escrowReleasing)
	if claim.Error != nil {
		return false, claim.Error
	}
	if claim.RowsAffected == 0 {
		return false, nil
	}

	unclaim := func() {
		s.db.Model(&models.EscrowTransactionV3{}).
			Where("id = ? AND status = ?", e.ID, escrowReleasing).
			Update("status", escrowHeld)
	}

	var freelancer models.User
	if err := s.db.First(&freelancer, "id = ?", e.UserID).Error; err != nil {
		unclaim()
		return false, fmt.Errorf("freelancer not found: %w", err)
	}
	if freelancer.StripeConnectAccountID == "" {
		unclaim()
		return false, errors.New("freelancer has no stripe account to receive the payment")
	}

	var payment models.PaymentTransactionV3
	if err := s.db.First(&payment, "id = ?", e.PaymentTransactionID).Error; err != nil {
		unclaim()
		return false, fmt.Errorf("payment transaction not found: %w", err)
	}

	params := &stripe.TransferParams{
		Amount:        stripe.Int64(ToPence(e.Amount)),
		Currency:      stripe.String(e.Currency),
		Destination:   stripe.String(freelancer.StripeConnectAccountID),
		TransferGroup: stripe.String(contract.ID),
		Metadata: map[string]string{
			"escrow_id":   e.ID,
			"contract_id": contract.ID,
		},
	}
	// Tie the transfer to the original charge so it doesn't depend on the
	// platform's available balance while card funds are still pending.
	if payment.StripeChargeID != "" {
		params.SourceTransaction = stripe.String(payment.StripeChargeID)
	}
	params.SetIdempotencyKey("v3-transfer-" + e.ID)

	tr, err := transfer.New(params)
	if err != nil {
		unclaim()
		return false, fmt.Errorf("failed to create stripe transfer: %w", err)
	}

	// If this write fails the escrow stays "releasing"; the transfer.created
	// webhook reconciles it via the escrow_id metadata.
	if err := s.markReleased(e.ID, e.PaymentTransactionID, tr.ID); err != nil {
		return false, fmt.Errorf("transfer %s created but failed to record release: %w", tr.ID, err)
	}
	return true, nil
}

func (s *Service) markReleased(escrowID, paymentTransactionID, transferID string) error {
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.EscrowTransactionV3{}).
			Where("id = ?", escrowID).
			Updates(map[string]any{
				"status":             escrowReleased,
				"stripe_transfer_id": transferID,
				"released_at":        &now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&models.PaymentTransactionV3{}).
			Where("id = ?", paymentTransactionID).
			Update("status", escrowReleased).Error
	})
}
