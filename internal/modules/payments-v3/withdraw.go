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
	"gorm.io/gorm"
)

var (
	ErrNoStripeAccount      = errors.New("your payout account is not set up yet")
	ErrNoBankAccount        = errors.New("add a bank account before withdrawing")
	ErrBankAccountNotFound  = errors.New("bank account not found")
	ErrBankAccountNotLinked = errors.New("this bank account is not linked to your payout account; please remove and re-add it")
	ErrNothingToWithdraw    = errors.New("no available balance to withdraw")
	ErrInvalidAmount        = errors.New("withdrawal amount must be greater than zero")
	ErrInsufficientBalance  = errors.New("amount exceeds your available balance")
	ErrBelowMinimumPayout   = errors.New("the minimum withdrawal is £1.00")
)

// MinimumPayout is Stripe's smallest GBP payout; Stripe rejects anything lower.
var MinimumPayout = decimal.NewFromInt(1)

// Money stays in the Stripe balance until the tasker taps Withdraw.
func ManualPayoutSettings() *stripe.AccountSettingsParams {
	return &stripe.AccountSettingsParams{
		Payouts: &stripe.AccountSettingsPayoutsParams{
			Schedule: &stripe.AccountSettingsPayoutsScheduleParams{
				Interval: stripe.String("manual"),
			},
		},
	}
}

// Migrates accounts created before manual payouts existed and returns the account.
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

// A nil amount withdraws everything available; an empty bankAccountID means Stripe's default GBP bank.
func (s *Service) Withdraw(user *models.User, amount *float64, bankAccountID string) (*models.WithdrawalV3, error) {
	if user.StripeConnectAccountID == "" {
		return nil, ErrNoStripeAccount
	}

	acc, err := s.ensureManualPayouts(user.StripeConnectAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare payout account: %w", err)
	}

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

	withdrawal := models.WithdrawalV3{
		UserID:          user.ID,
		StripeAccountID: user.StripeConnectAccountID,
		Currency:        PaymentCurrency,
		Status:          string(stripe.PayoutStatusPending),
		BankAccountID:   bankRowID,
		DestinationID:   destination,
		BankLast4:       last4,
	}

	// The user row lock serialises withdrawals: each one sees the earlier
	// pending rows in the balance, so simultaneous requests can't overdraw.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT id FROM users WHERE id = ? FOR UPDATE`, user.ID).Error; err != nil {
			return err
		}
		ledger, err := ledgerBalance(tx, user.ID)
		if err != nil {
			return fmt.Errorf("failed to compute balance: %w", err)
		}
		available := withdrawableNow(ledger, bal.Available)

		amt := available
		if amount != nil {
			amt = decimal.NewFromFloat(*amount).Round(2)
			if !amt.IsPositive() {
				return ErrInvalidAmount
			}
		}
		if !amt.IsPositive() {
			return ErrNothingToWithdraw
		}
		if amt.LessThan(MinimumPayout) {
			return fmt.Errorf("%w (you asked for £%s)", ErrBelowMinimumPayout, amt.StringFixed(2))
		}
		if amt.GreaterThan(available) {
			return ErrInsufficientBalance
		}

		withdrawal.Amount = amt
		if err := tx.Create(&withdrawal).Error; err != nil {
			return fmt.Errorf("failed to record withdrawal: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	amt := withdrawal.Amount

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
		return nil, &PayoutRejectedError{err: err}
	}

	s.applyPayout(&withdrawal, p)
	return &withdrawal, nil
}

// UserMessage hides Stripe's raw error (request ids, dashboard links) from app users.
type PayoutRejectedError struct{ err error }

func (e *PayoutRejectedError) Error() string { return "stripe payout failed: " + e.err.Error() }
func (e *PayoutRejectedError) Unwrap() error { return e.err }

func (e *PayoutRejectedError) Rejected() bool {
	var se *stripe.Error
	return errors.As(e.err, &se) && se.HTTPStatusCode >= 400 && se.HTTPStatusCode < 500
}

func (e *PayoutRejectedError) UserMessage() string {
	var se *stripe.Error
	if errors.As(e.err, &se) && se.Msg != "" {
		return "withdrawal failed: " + se.Msg
	}
	return "withdrawal failed; please try again later"
}

// syncWithdrawalStatuses re-checks unfinished payouts with Stripe, so a missed
// payout.* webhook can't leave a withdrawal stuck as pending.
func (s *Service) syncWithdrawalStatuses(userID string) {
	var open []models.WithdrawalV3
	if err := s.db.Where("user_id = ? AND stripe_payout_id <> '' AND status IN ?", userID,
		[]string{string(stripe.PayoutStatusPending), string(stripe.PayoutStatusInTransit)}).Find(&open).Error; err != nil {
		return
	}
	for i := range open {
		params := &stripe.PayoutParams{}
		params.SetStripeAccount(open[i].StripeAccountID)
		p, err := payout.Get(open[i].StripePayoutID, params)
		if err != nil {
			logger.Log.Warn("failed to refresh payout status", zap.String("payout_id", open[i].StripePayoutID), zap.Error(err))
			continue
		}
		if string(p.Status) != open[i].Status {
			s.applyPayout(&open[i], p)
		}
	}
}

func (s *Service) ListWithdrawals(userID string) ([]models.WithdrawalV3, error) {
	s.syncWithdrawalStatuses(userID)
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
