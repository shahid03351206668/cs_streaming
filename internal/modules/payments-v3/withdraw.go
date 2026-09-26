package paymentsv3

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/account"
	"github.com/stripe/stripe-go/v84/balance"
	"github.com/stripe/stripe-go/v84/payout"
	"go.uber.org/zap"
)

var (
	ErrNoStripeAccount      = errors.New("your payout account is not set up yet")
	ErrNoBankAccount        = errors.New("add a bank account before withdrawing")
	ErrBankAccountNotFound  = errors.New("bank account not found")
	ErrBankAccountNotLinked = errors.New("this bank account is not linked to your payout account; please remove and re-add it")
	ErrNothingToWithdraw    = errors.New("no available balance to withdraw")
	ErrInvalidAmount        = errors.New("withdrawal amount must be greater than zero")
	ErrInsufficientBalance  = errors.New("amount exceeds your available balance")
)

// ManualPayoutSettings puts a Connect account on a manual payout schedule so
// money stays in the Stripe balance until the tasker taps Withdraw.
func ManualPayoutSettings() *stripe.AccountSettingsParams {
	return &stripe.AccountSettingsParams{
		Payouts: &stripe.AccountSettingsPayoutsParams{
			Schedule: &stripe.AccountSettingsPayoutsScheduleParams{
				Interval: stripe.String("manual"),
			},
		},
	}
}

// ensureManualPayouts migrates accounts created before manual payouts were
// introduced (new accounts get the setting at creation), and returns the
// account so callers can inspect its attached bank accounts.
func (s *Service) ensureManualPayouts(accountID string) (*stripe.Account, error) {
	acc, err := account.GetByID(accountID, nil)
	if err != nil {
		return nil, err
	}
	if acc.Settings != nil && acc.Settings.Payouts != nil && acc.Settings.Payouts.Schedule != nil &&
		acc.Settings.Payouts.Schedule.Interval == "manual" {
		return acc, nil
	}
	return account.Update(accountID, &stripe.AccountParams{Settings: ManualPayoutSettings()})
}

// stripeBankAccounts returns the GBP bank accounts attached to a Connect account.
func stripeBankAccounts(acc *stripe.Account) []*stripe.BankAccount {
	var banks []*stripe.BankAccount
	if acc == nil || acc.ExternalAccounts == nil {
		return banks
	}
	for _, ext := range acc.ExternalAccounts.Data {
		if ext != nil && ext.BankAccount != nil && string(ext.BankAccount.Currency) == PaymentCurrency {
			banks = append(banks, ext.BankAccount)
		}
	}
	return banks
}

type PayoutBalance struct {
	Available decimal.Decimal
	Pending   decimal.Decimal
}

// GetPayoutBalance reads the tasker's live Stripe Connect balance. Released
// payments sit in Pending until the underlying card charge settles.
func (s *Service) GetPayoutBalance(user *models.User) (*PayoutBalance, error) {
	if user.StripeConnectAccountID == "" {
		return &PayoutBalance{}, nil
	}

	if _, err := s.ensureManualPayouts(user.StripeConnectAccountID); err != nil {
		logger.Log.Warn("failed to enforce manual payout schedule",
			zap.String("user_id", user.ID), zap.Error(err))
	}

	return s.connectBalance(user.StripeConnectAccountID)
}

func (s *Service) connectBalance(accountID string) (*PayoutBalance, error) {
	params := &stripe.BalanceParams{}
	params.SetStripeAccount(accountID)
	bal, err := balance.Get(params)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stripe balance: %w", err)
	}

	sum := func(amounts []*stripe.BalanceAmount) decimal.Decimal {
		total := int64(0)
		for _, a := range amounts {
			if string(a.Currency) == PaymentCurrency {
				total += a.Amount
			}
		}
		return fromPence(total)
	}

	return &PayoutBalance{Available: sum(bal.Available), Pending: sum(bal.Pending)}, nil
}

// Withdraw pays out `amount` (or the full available balance when nil) from the
// tasker's Connect balance. bankAccountID selects one of the user's saved bank
// accounts; empty means Stripe's default GBP bank account.
func (s *Service) Withdraw(user *models.User, amount *float64, bankAccountID string) (*models.WithdrawalV3, error) {
	if user.StripeConnectAccountID == "" {
		return nil, ErrNoStripeAccount
	}

	// Stripe rejects manual payouts while the account is on an automatic schedule.
	acc, err := s.ensureManualPayouts(user.StripeConnectAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare payout account: %w", err)
	}

	// Stripe, not our DB, decides where money can go: some bank accounts were
	// attached directly on Stripe without a local row.
	banks := stripeBankAccounts(acc)
	if len(banks) == 0 {
		return nil, ErrNoBankAccount
	}

	var destination, bankRowID, last4 string
	if bankAccountID != "" {
		var bank models.UserBankAccount
		if err := s.db.First(&bank, "id = ? AND user_id = ?", bankAccountID, user.ID).Error; err != nil {
			return nil, ErrBankAccountNotFound
		}
		for _, b := range banks {
			if b.ID == bank.StripeBankAccountID {
				destination, bankRowID, last4 = b.ID, bank.ID, b.Last4
				break
			}
		}
		if destination == "" {
			return nil, ErrBankAccountNotLinked
		}
	} else {
		for _, b := range banks {
			if b.DefaultForCurrency {
				last4 = b.Last4
				break
			}
		}
	}

	bal, err := s.connectBalance(user.StripeConnectAccountID)
	if err != nil {
		return nil, err
	}

	amt := bal.Available
	if amount != nil {
		amt = decimal.NewFromFloat(*amount).Round(2)
		if !amt.IsPositive() {
			return nil, ErrInvalidAmount
		}
	}
	if !amt.IsPositive() {
		return nil, ErrNothingToWithdraw
	}
	if amt.GreaterThan(bal.Available) {
		return nil, ErrInsufficientBalance
	}

	withdrawal := models.WithdrawalV3{
		UserID:          user.ID,
		StripeAccountID: user.StripeConnectAccountID,
		Amount:          amt,
		Currency:        PaymentCurrency,
		Status:          string(stripe.PayoutStatusPending),
		BankAccountID:   bankRowID,
		DestinationID:   destination,
		BankLast4:       last4,
	}
	if err := s.db.Create(&withdrawal).Error; err != nil {
		return nil, fmt.Errorf("failed to record withdrawal: %w", err)
	}

	params := &stripe.PayoutParams{
		Amount:   stripe.Int64(ToPence(amt)),
		Currency: stripe.String(PaymentCurrency),
		Metadata: map[string]string{
			"withdrawal_id": withdrawal.ID,
			"user_id":       user.ID,
		},
	}
	if destination != "" {
		params.Destination = stripe.String(destination)
	}
	params.SetStripeAccount(user.StripeConnectAccountID)
	params.SetIdempotencyKey("v3-payout-" + withdrawal.ID)

	p, err := payout.New(params)
	if err != nil {
		s.db.Model(&withdrawal).Updates(map[string]any{
			"status":          string(stripe.PayoutStatusFailed),
			"failure_message": err.Error(),
		})
		return nil, fmt.Errorf("stripe payout failed: %w", err)
	}

	s.applyPayout(&withdrawal, p)
	return &withdrawal, nil
}

func (s *Service) ListWithdrawals(userID string) ([]models.WithdrawalV3, error) {
	withdrawals := make([]models.WithdrawalV3, 0)
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&withdrawals).Error
	return withdrawals, err
}

func (s *Service) applyPayout(w *models.WithdrawalV3, p *stripe.Payout) {
	updates := map[string]any{
		"stripe_payout_id": p.ID,
		"status":           string(p.Status),
		"failure_message":  p.FailureMessage,
	}
	if p.Destination != nil && p.Destination.ID != "" {
		updates["destination_id"] = p.Destination.ID
	}
	if p.ArrivalDate > 0 {
		arrival := time.Unix(p.ArrivalDate, 0)
		updates["arrival_date"] = &arrival
	}
	if err := s.db.Model(w).Updates(updates).Error; err != nil {
		logger.Log.Error("failed to update withdrawal from payout",
			zap.String("withdrawal_id", w.ID), zap.String("payout_id", p.ID), zap.Error(err))
	}
}

// handlePayoutEventV3 syncs withdrawal status from payout.* Connect events.
func (s *Service) handlePayoutEventV3(event *stripe.Event) error {
	var p stripe.Payout
	if err := json.Unmarshal(event.Data.Raw, &p); err != nil {
		return err
	}

	var w models.WithdrawalV3
	err := s.db.Where("stripe_payout_id = ?", p.ID).First(&w).Error
	if err != nil && p.Metadata["withdrawal_id"] != "" {
		err = s.db.Where("id = ?", p.Metadata["withdrawal_id"]).First(&w).Error
	}
	if err != nil {
		return nil // payout not created through the withdraw flow
	}

	s.applyPayout(&w, &p)
	return nil
}
