package paymentsv3

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/bankaccount"
	"github.com/stripe/stripe-go/v84/customer"
	"github.com/stripe/stripe-go/v84/paymentintent"
	"github.com/stripe/stripe-go/v84/token"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type SystemSetting struct {
	AppFee               float64
	ClientCommission     float64
	FreelancerCommission float64
}

type TypeWalletTransaction struct {
	Amount      float64   `json:"amount"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Date        time.Time `json:"date"`
	ID          string    `json:"id"`
}

type UserWalletVal struct {
	Balance           float64
	TotalBalance      float64
	PendingSettlement float64
	EscrowAmount      float64
	Transactions      []TypeWalletTransaction
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) GetSystemSettings() (*SystemSetting, error) {
	var settings models.SystemSettings

	if err := s.db.First(&settings).Error; err != nil {
		return nil, err
	}

	return &SystemSetting{
		AppFee:               settings.ApplicationFeeAmount,
		FreelancerCommission: settings.FreelancerCommissionPercentage,
		ClientCommission:     settings.ClientCommissionPercentage,
	}, nil
}

var (
	ErrBidBelowFees        = errors.New("bid amount is too low to cover the freelancer commission and platform fee")
	ErrAlreadyPaid         = errors.New("this proposal has already been paid for")
	ErrProposalNotAccepted = errors.New("only an accepted proposal can be paid for")
)

func (s *Service) CreatePaymentIntent(client, freelancer *models.User, proposal *models.Proposal) (*stripe.PaymentIntent, *FeeBreakdown, error) {

	if freelancer.StripeConnectAccountID == "" {
		return nil, nil, errors.New("freelancer does not have a stripe account")
	}

	var contract models.Contract
	if err := s.db.Where("proposal_id = ?", proposal.ID).First(&contract).Error; err == nil {
		var existing int64
		s.db.Model(&models.PaymentTransactionV3{}).
			Where("contract_id = ? AND status IN ?", contract.ID, []string{escrowHeld, escrowReleasing, escrowReleased}).
			Count(&existing)
		if existing > 0 {
			return nil, nil, ErrAlreadyPaid
		}
	}

	settings, err := s.GetSystemSettings()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load system settings: %w", err)
	}

	fees := CalculateFees(proposal.BidAmount, settings)
	if !fees.FreelancerNet.IsPositive() {
		return nil, nil, ErrBidBelowFees
	}

	customerID, err := s.ensureStripeCustomer(client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set up stripe customer: %w", err)
	}

	metadata := fees.Metadata()
	metadata["proposal_id"] = proposal.ID
	metadata["from_user_id"] = client.ID
	metadata["to_user_id"] = freelancer.ID
	metadata["freelancer_connect_id"] = freelancer.StripeConnectAccountID

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(ToPence(fees.ClientTotal)),
		Currency: stripe.String(string(stripe.CurrencyGBP)),
		Customer: stripe.String(customerID),
		Metadata: metadata,
	}

	params.IdempotencyKey = stripe.String("v3-intent-" + proposal.ID)
	intent, err := paymentintent.New(params)
	// Concurrent double taps share a key; retry until the first request finishes and returns its intent.
	for wait := 300 * time.Millisecond; isIdempotencyKeyInUse(err) && wait <= 2400*time.Millisecond; wait *= 2 {
		time.Sleep(wait)
		intent, err = paymentintent.New(params)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := s.db.Model(&models.Proposal{}).Where("id = ?", proposal.ID).
		Update("stripe_payment_intent_id", intent.ID).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to save payment intent on proposal: %w", err)
	}
	return intent, &fees, nil
}

func isIdempotencyKeyInUse(err error) bool {
	var se *stripe.Error
	return errors.As(err, &se) && se.Code == stripe.ErrorCodeIdempotencyKeyInUse
}

// Created on first use so charges link to a customer in the Dashboard.
func (s *Service) ensureStripeCustomer(client *models.User) (string, error) {
	if client.StripeCustomerID != "" {
		return client.StripeCustomerID, nil
	}

	params := &stripe.CustomerParams{
		Email: stripe.String(client.Email),
		Name:  stripe.String(strings.TrimSpace(client.FirstName + " " + client.LastName)),
		Metadata: map[string]string{
			"user_id": client.ID,
		},
	}
	// Per-user key so a double-tapped first payment can't create two customers.
	params.SetIdempotencyKey("v3-customer-" + client.ID)
	cust, err := customer.New(params)
	for wait := 300 * time.Millisecond; isIdempotencyKeyInUse(err) && wait <= 2400*time.Millisecond; wait *= 2 {
		time.Sleep(wait)
		cust, err = customer.New(params)
	}
	if err != nil {
		return "", err
	}

	if err := s.db.Model(&models.User{}).Where("id = ?", client.ID).
		Update("stripe_customer_id", cust.ID).Error; err != nil {
		return "", fmt.Errorf("failed to save stripe customer id: %w", err)
	}

	client.StripeCustomerID = cust.ID
	return cust.ID, nil
}

func (s *Service) tokenizeBankAccount(accountHolderName, sortCode, accountNumber string) (*stripe.Token, error) {
	return token.New(&stripe.TokenParams{
		BankAccount: &stripe.BankAccountParams{
			Country:           stripe.String("GB"),
			Currency:          stripe.String("gbp"),
			AccountHolderName: stripe.String(accountHolderName),
			AccountHolderType: stripe.String("individual"),
			RoutingNumber:     stripe.String(sortCode),
			AccountNumber:     stripe.String(accountNumber),
		},
	})
}

func (s *Service) AddBankAccount(user *models.User, accountHolderName, sortCode, accountNumber string) (*models.UserBankAccount, error) {
	if user.StripeConnectAccountID == "" {
		return nil, errors.New("stripe account not provisioned for this user")
	}

	tok, err := s.tokenizeBankAccount(accountHolderName, sortCode, accountNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize bank account: %w", err)
	}

	ba, err := bankaccount.New(&stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
		Token:   stripe.String(tok.ID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach bank account: %w", err)
	}

	var count int64
	s.db.Model(&models.UserBankAccount{}).Where("user_id = ?", user.ID).Count(&count)

	record := models.UserBankAccount{
		UserID:                 user.ID,
		StripeConnectAccountID: user.StripeConnectAccountID,
		StripeBankAccountID:    ba.ID,
		AccountHolderName:      accountHolderName,
		AccountNumber:          accountNumber,
		SortCode:               sortCode,
		AccountNumberLast4:     ba.Last4,
		BankName:               ba.BankName,
		Currency:               string(ba.Currency),
		IsDefault:              count == 0,
	}

	if err := s.db.Create(&record).Error; err != nil {
		return nil, fmt.Errorf("failed to save bank account: %w", err)
	}

	s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("stripe_connect_onboarded", true)
	return &record, nil
}

func (s *Service) UpdateBankAccount(user *models.User, bankAccountID, accountHolderName, sortCode, accountNumber string) (*models.UserBankAccount, error) {
	var existing models.UserBankAccount
	if err := s.db.First(&existing, "id = ? AND user_id = ?", bankAccountID, user.ID).Error; err != nil {
		return nil, errors.New("bank account not found")
	}

	oldBA, err := bankaccount.Get(existing.StripeBankAccountID, &stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch old bank account: %w", err)
	}

	tok, err := s.tokenizeBankAccount(accountHolderName, sortCode, accountNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize bank account: %w", err)
	}

	ba, err := bankaccount.New(&stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
		Token:   stripe.String(tok.ID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach new bank account: %w", err)
	}

	if oldBA.DefaultForCurrency {
		if _, err := bankaccount.Update(ba.ID, &stripe.BankAccountParams{
			Account:            stripe.String(user.StripeConnectAccountID),
			DefaultForCurrency: stripe.Bool(true),
		}); err != nil {
			bankaccount.Del(ba.ID, &stripe.BankAccountParams{Account: stripe.String(user.StripeConnectAccountID)})
			return nil, fmt.Errorf("failed to set new bank account as default: %w", err)
		}
	}

	if _, err := bankaccount.Del(existing.StripeBankAccountID, &stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
	}); err != nil {
		return nil, fmt.Errorf("failed to remove old bank account from stripe: %w", err)
	}

	if err := s.db.Model(&existing).Updates(map[string]any{
		"stripe_bank_account_id": ba.ID,
		"account_holder_name":    accountHolderName,
		"account_number":         accountNumber,
		"sort_code":              sortCode,
		"account_number_last4":   ba.Last4,
		"bank_name":              ba.BankName,
	}).Error; err != nil {
		return nil, err
	}

	s.db.First(&existing, "id = ?", bankAccountID)
	return &existing, nil
}

func (s *Service) DeleteBankAccount(user *models.User, bankAccountID string) error {
	var existing models.UserBankAccount
	if err := s.db.First(&existing, "id = ? AND user_id = ?", bankAccountID, user.ID).Error; err != nil {
		return errors.New("bank account not found")
	}

	oldBA, err := bankaccount.Get(existing.StripeBankAccountID, &stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
	})
	if err != nil {
		return fmt.Errorf("failed to fetch bank account: %w", err)
	}

	var next models.UserBankAccount
	hasNext := s.db.Where("user_id = ? AND id != ?", user.ID, bankAccountID).
		Order("created_at DESC").First(&next).Error == nil

	// Stripe refuses to delete a currency's default bank account, so promote another first.
	if oldBA.DefaultForCurrency && hasNext {
		if _, err := bankaccount.Update(next.StripeBankAccountID, &stripe.BankAccountParams{
			Account:            stripe.String(user.StripeConnectAccountID),
			DefaultForCurrency: stripe.Bool(true),
		}); err != nil {
			return fmt.Errorf("failed to set new default bank account: %w", err)
		}
	}

	if _, err := bankaccount.Del(existing.StripeBankAccountID, &stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
	}); err != nil {
		return fmt.Errorf("failed to remove bank account from stripe: %w", err)
	}

	if err := s.db.Delete(&existing).Error; err != nil {
		return fmt.Errorf("failed to delete bank account: %w", err)
	}

	if existing.IsDefault && hasNext {
		s.db.Model(&next).Update("is_default", true)
	}

	return nil
}

func (s *Service) GetBankAccounts(user *models.User) ([]models.UserBankAccount, error) {
	var accounts []models.UserBankAccount
	err := s.db.Where("user_id = ?", user.ID).Order("is_default DESC, created_at DESC").Find(&accounts).Error
	return accounts, err
}

func (s *Service) GetUserWallet(user *models.User) (*UserWalletVal, error) {
	transactions := make([]TypeWalletTransaction, 0)

	totalPending := decimal.Zero
	if err := s.db.Model(&models.EscrowTransactionV3{}).
		Where("user_id = ? AND status = ?", user.ID, escrowHeld).
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&totalPending).Error; err != nil {
		return nil, fmt.Errorf("failed to sum held escrows: %w", err)
	}

	balance := 0.0
	if err := s.db.Model(&models.EscrowTransactionV3{}).
		Where("user_id = ? AND status = ?", user.ID, escrowReleased).
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&balance).Error; err != nil {
		return nil, fmt.Errorf("failed to sum released escrows: %w", err)
	}

	var paymentTxns []models.PaymentTransactionV3
	if err := s.db.Where("from_user_id = ? OR to_user_id = ?", user.ID, user.ID).
		Find(&paymentTxns).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch payment transactions: %w", err)
	}

	for _, p := range paymentTxns {
		gross, _ := p.GrossAmount.Float64()
		net, _ := p.NetAmount.Float64()

		if p.FromUserID == user.ID {
			desc := "Contract payment sent"
			if p.Status == escrowRefunded {
				desc = "Contract payment sent (refunded)"
			}
			transactions = append(transactions, TypeWalletTransaction{
				Amount:      gross,
				Type:        "debit",
				Description: desc,
				Date:        p.CreatedAt,
				ID:          p.ID,
			})
		} else if p.ToUserID == user.ID && p.Status == escrowHeld {

			transactions = append(transactions, TypeWalletTransaction{
				Amount:      net,
				Type:        "credit",
				Description: "Payment received (held in escrow)",
				Date:        p.CreatedAt,
				ID:          p.ID,
			})
		}
	}

	var releasedEscrows []models.EscrowTransactionV3
	if err := s.db.Where("user_id = ? AND status = ?", user.ID, escrowReleased).
		Find(&releasedEscrows).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch escrow transactions: %w", err)
	}
	for _, e := range releasedEscrows {
		amt, _ := e.Amount.Float64()
		date := e.HeldAt
		if e.ReleasedAt != nil {
			date = *e.ReleasedAt
		}
		transactions = append(transactions, TypeWalletTransaction{
			Amount:      amt,
			Type:        "credit",
			Description: "Escrow released",
			Date:        date,
			ID:          e.ID,
		})
	}

	var withdrawals []models.WithdrawalV3
	if err := s.db.Where("user_id = ? AND status NOT IN ?", user.ID,
		[]string{string(stripe.PayoutStatusFailed), string(stripe.PayoutStatusCanceled)}).
		Find(&withdrawals).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch withdrawals: %w", err)
	}
	for _, w := range withdrawals {
		amt, _ := w.Amount.Float64()
		balance -= amt
		transactions = append(transactions, TypeWalletTransaction{
			Amount:      amt,
			Type:        "debit",
			Description: "Withdrawal to bank",
			Date:        w.CreatedAt,
			ID:          w.ID,
		})
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Date.After(transactions[j].Date)
	})

	escrowAmount, _ := totalPending.Float64()

	// Stripe holds released money until the card payment settles; if it can't be reached, claim nothing as withdrawable.
	available, pending := 0.0, balance
	if user.StripeConnectAccountID != "" {
		if bal, err := s.connectBalance(user.StripeConnectAccountID); err == nil {
			available, pending = bal.Available.InexactFloat64(), bal.Pending.InexactFloat64()
		}
	}

	return &UserWalletVal{
		Balance:           available,
		TotalBalance:      balance,
		PendingSettlement: pending,
		EscrowAmount:      escrowAmount,
		Transactions:      transactions,
	}, nil
}

func (s *Service) handleChargeSucceededV3(event *stripe.Event) error {
	var charge stripe.Charge

	var existingPayment *models.PaymentTransactionV3
	if err := s.db.Where("stripe_event_id = ?", event.ID).First(&existingPayment).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		return err
	}

	// Errors while there is no contract so Stripe retries; RecordProposalPayment records it once the contract exists.
	return s.recordCharge(&charge, event.ID)
}

var ErrContractNotCreatedYet = errors.New("no contract for this proposal yet; the payment will be recorded when the contract is created")

func (s *Service) paymentRecorded(paymentIntentID string) bool {
	var n int64
	s.db.Model(&models.PaymentTransactionV3{}).Where("stripe_payment_intent_id = ?", paymentIntentID).Count(&n)
	return n > 0
}

// Records a payment made before the contract existed (the webhook couldn't); no-op if absent or already recorded.
func (s *Service) RecordProposalPayment(proposalID string) error {
	var proposal models.Proposal
	if err := s.db.Select("id", "stripe_payment_intent_id").First(&proposal, "id = ?", proposalID).Error; err != nil {
		return err
	}
	if proposal.StripePaymentIntentID == "" || s.paymentRecorded(proposal.StripePaymentIntentID) {
		return nil
	}

	params := &stripe.PaymentIntentParams{}
	params.AddExpand("latest_charge")
	pi, err := paymentintent.Get(proposal.StripePaymentIntentID, params)
	if err != nil {
		return fmt.Errorf("failed to fetch payment intent: %w", err)
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded || pi.LatestCharge == nil {
		return nil
	}
	if pi.LatestCharge.PaymentIntent == nil {
		pi.LatestCharge.PaymentIntent = &stripe.PaymentIntent{ID: pi.ID}
	}
	return s.recordCharge(pi.LatestCharge, "reconciled:"+pi.LatestCharge.ID)
}

// Idempotent per PaymentIntent so webhook retries and RecordProposalPayment can't record twice.
func (s *Service) recordCharge(charge *stripe.Charge, eventID string) error {
	proposalID := charge.Metadata["proposal_id"]
	if proposalID == "" {
		return fmt.Errorf("proposal id is not found in the meta data")
	}

	stripePaymentIntentID := ""
	if charge.PaymentIntent != nil {
		stripePaymentIntentID = charge.PaymentIntent.ID
	}
	if stripePaymentIntentID != "" && s.paymentRecorded(stripePaymentIntentID) {
		return nil
	}

	fromUserID := charge.Metadata["from_user_id"]
	toUserID := charge.Metadata["to_user_id"]

	var contract models.Contract
	if err := s.db.Where("proposal_id = ?", proposalID).First(&contract).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrContractNotCreatedYet
		}
		return err
	}

	// Use the split stored on the intent; older intents recompute from the bid with current settings.
	fees, ok := FeesFromMetadata(charge.Metadata)
	if !ok {
		var proposal models.Proposal
		if err := s.db.First(&proposal, "id = ?", proposalID).Error; err != nil {
			return fmt.Errorf("proposal not found: %w", err)
		}
		settings, err := s.GetSystemSettings()
		if err != nil {
			return fmt.Errorf("failed to load system settings: %w", err)
		}
		fees = CalculateFees(proposal.BidAmount, settings)
	}

	grossAmount := fromPence(charge.Amount)
	clientFee := fees.ClientCommission.Add(fees.ClientPlatformFee)
	freelancerFee := fees.FreelancerCommission.Add(fees.FreelancerPlatformFee)
	netAmount := fees.FreelancerNet
	platformFee := grossAmount.Sub(netAmount)

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	transaction := models.PaymentTransactionV3{
		StripePaymentIntentID: stripePaymentIntentID,
		StripeChargeID:        charge.ID,
		StripeEventID:         eventID,
		FromUserID:            fromUserID,
		ToUserID:              toUserID,
		ContractID:            contract.ID,
		GrossAmount:           grossAmount,
		ClientFee:             clientFee,
		FreelancerFee:         freelancerFee,
		PlatformFee:           platformFee,
		NetAmount:             netAmount,
		Currency:              string(charge.Currency),
		Status:                "held",
		FlowVersion:           "v3",
	}

	if err := tx.Create(&transaction).Error; err != nil {
		tx.Rollback()
		// The webhook and RecordProposalPayment can race; the unique payment-intent index lets one win.
		if stripePaymentIntentID != "" && s.paymentRecorded(stripePaymentIntentID) {
			return nil
		}
		return fmt.Errorf("failed to create v3 transaction: %w", err)
	}

	escrow := models.EscrowTransactionV3{
		PaymentTransactionID: transaction.ID,
		ContractID:           contract.ID,
		UserID:               toUserID,
		Amount:               netAmount,
		Currency:             string(charge.Currency),
		Status:               "held",
		HeldAt:               time.Now(),
	}

	if err := tx.Create(&escrow).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create v3 escrow: %w", err)
	}

	return tx.Commit().Error
}

func (s *Service) handleTransferPaidV3(event *stripe.Event) error {
	var tr stripe.Transfer
	if err := json.Unmarshal(event.Data.Raw, &tr); err != nil {
		return err
	}

	// Match by transfer ID, or by escrow_id metadata when we created the transfer but failed to record it.
	var escrow models.EscrowTransactionV3
	err := s.db.Where("stripe_transfer_id = ?", tr.ID).First(&escrow).Error
	if err != nil && tr.Metadata["escrow_id"] != "" {
		err = s.db.Where("id = ?", tr.Metadata["escrow_id"]).First(&escrow).Error
	}
	if err != nil {
		return nil
	}

	if escrow.Status == escrowReleased || escrow.Status == "refunded" {
		return nil
	}

	return s.markReleased(escrow.ID, escrow.PaymentTransactionID, tr.ID)
}

func (s *Service) handleAccountUpdatedV3(event *stripe.Event) error {
	var acc stripe.Account
	if err := json.Unmarshal(event.Data.Raw, &acc); err != nil {
		return fmt.Errorf("failed to parse account.updated payload: %w", err)
	}

	// Only flip onboarded when account has no outstanding requirements and is not restricted.
	reqs := acc.Requirements
	if !acc.DetailsSubmitted ||
		(reqs != nil && len(reqs.CurrentlyDue) > 0) ||
		(reqs != nil && len(reqs.EventuallyDue) > 0) ||
		(reqs != nil && reqs.DisabledReason != "") {
		return nil
	}

	return s.db.Model(&models.User{}).
		Where("stripe_connect_account_id = ?", acc.ID).
		Update("stripe_connect_onboarded", true).Error
}

func (s *Service) handleChargeRefundedV3(event *stripe.Event) error {
	var charge stripe.Charge
	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		return err
	}

	var transaction models.PaymentTransactionV3
	if err := s.db.Where("stripe_charge_id = ?", charge.ID).First(&transaction).Error; err != nil {
		return nil
	}

	if transaction.Status == "refunded" {
		return nil
	}

	// charge.refunded also fires for partial refunds; only a full refund ends the escrow.
	if !charge.Refunded {
		logger.Log.Warn("partial refund made outside the app; escrow left held",
			zap.String("transaction_id", transaction.ID), zap.String("charge_id", charge.ID),
			zap.Int64("amount_refunded_pence", charge.AmountRefunded))
		return nil
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Model(&transaction).Updates(map[string]any{
		"status":          escrowRefunded,
		"refunded_amount": transaction.GrossAmount,
	}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Model(&models.EscrowTransactionV3{}).
		Where("payment_transaction_id = ?", transaction.ID).
		Update("status", "refunded").Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
