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
	escrowRefunded  = "refunded"
)

var (
	ErrContractNotCompleted = errors.New("both parties must mark the contract complete before payment is released")
	ErrContractDisputed     = errors.New("contract has an open dispute; payment is on hold until it is resolved")
	ErrContractClosed       = errors.New("payment cannot be released for a cancelled or terminated contract")
	ErrNothingToRelease     = errors.New("no held payment to release for this contract")
	ErrNotContractParty     = errors.New("you are not a party to this contract")
)

// Checks completion flags, not contract.status, because dispute resolution can reset status to "active".
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

// requireCompletion is false only for the admin override; disputes and closed contracts always block.
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
	return s.releaseHeld(&escrow, &contract, adminID, note)
}

func (s *Service) releaseHeld(escrow *models.EscrowTransactionV3, contract *models.Contract, adminID, note string) error {
	ok, err := s.releaseEscrow(escrow, contract)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNothingToRelease
	}
	return s.db.Model(&models.EscrowTransactionV3{}).Where("id = ?", escrow.ID).
		Updates(map[string]any{"released_by": adminID, "release_note": note}).Error
}

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

// held -> releasing -> released; the conditional claim and per-escrow idempotency key prevent a double payout.
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

	// A transfer may exist already (our DB write failed, webhook missed); record it instead of paying twice.
	if existing := findEscrowTransfer(contract.ID, e.ID); existing != "" {
		if err := s.markReleased(e.ID, e.PaymentTransactionID, existing); err != nil {
			return false, fmt.Errorf("transfer %s exists but failed to record release: %w", existing, err)
		}
		return true, nil
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
	// Tie the transfer to the charge so it doesn't depend on the platform's available balance.
	if payment.StripeChargeID != "" {
		params.SourceTransaction = stripe.String(payment.StripeChargeID)
	}

	attempt := e.TransferAttempt
	for tries := 0; ; tries++ {
		params.SetIdempotencyKey(transferIdempotencyKey(e.ID, attempt))
		tr, err := transfer.New(params)
		if err == nil {
			// If this write fails the escrow stays "releasing" and the transfer.created webhook reconciles it.
			if err := s.markReleased(e.ID, e.PaymentTransactionID, tr.ID); err != nil {
				return false, fmt.Errorf("transfer %s created but failed to record release: %w", tr.ID, err)
			}
			return true, nil
		}

		rejected, replayed := rejectedByStripe(err)
		if rejected {
			// Stripe replays a rejected request under the same idempotency key for 24h, so use a new key.
			attempt++
			s.db.Model(&models.EscrowTransactionV3{}).Where("id = ?", e.ID).Update("transfer_attempt", attempt)
			if replayed && tries == 0 {
				continue // the error was an earlier attempt's cached result; try for real
			}
		}
		unclaim()
		return false, fmt.Errorf("failed to create stripe transfer: %w", err)
	}
}

// Stable per attempt so a timed-out call that Stripe completed can't pay twice.
func transferIdempotencyKey(escrowID string, attempt int) string {
	if attempt == 0 {
		return "v3-transfer-" + escrowID
	}
	return fmt.Sprintf("v3-transfer-%s-%d", escrowID, attempt)
}

// Reports a 4xx (no transfer exists, so a new key is safe) and whether it replayed an earlier request.
func rejectedByStripe(err error) (rejected, replayed bool) {
	var se *stripe.Error
	if !errors.As(err, &se) || se.HTTPStatusCode < 400 || se.HTTPStatusCode >= 500 {
		return false, false
	}
	replayed = se.LastResponse != nil && se.LastResponse.Header.Get("Idempotent-Replayed") == "true"
	return true, replayed
}

func findEscrowTransfer(contractID, escrowID string) string {
	it := transfer.List(&stripe.TransferListParams{TransferGroup: stripe.String(contractID)})
	for it.Next() {
		if t := it.Transfer(); t.Metadata["escrow_id"] == escrowID && !t.Reversed {
			return t.ID
		}
	}
	return ""
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
