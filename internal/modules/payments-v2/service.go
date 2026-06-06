package paymentsv2

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"tasksy/lib"
	"tasksy/models"

	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v84/token"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/payout"

	// "go.uber.org/zap"
	"gorm.io/gorm"
)

type SystemSetting struct {
	AppFee               int64
	ClientCommission     float64
	FreelancerCommission float64
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) GetSystemSettings() (*SystemSetting, error) {
	var data models.SystemSettings
	if err := s.db.First(&data).Error; err != nil {
		return nil, err
	}
	return &SystemSetting{
		AppFee:               data.ApplicationFeeAmount,
		FreelancerCommission: data.FreelancerCommissionPercentage,
		ClientCommission:     data.ClientCommissionPercentage,
	}, nil
}

func (s *Service) handleChargeSucceeded(event *stripe.Event) error {
	var charge stripe.Charge

	var existingPayment *models.PaymentTransactionV2
	if err := s.db.Where("stripe_event_id = ?", event.ID).First(&existingPayment).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	settings, _ := s.GetSystemSettings()
	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		return err
	}

	proposalID := charge.Metadata["proposal_id"]
	if proposalID == "" {
		return fmt.Errorf("proposal id is not found in the meta data")
	}

	var contract models.Contract
	if err := s.db.Where("proposal_id = ?", proposalID).First(&contract).Error; err != nil {
		tx.Rollback()
		return err
	}

	amount := lib.Float(charge.Amount / 100)
	appFees := lib.Float(settings.AppFee)

	clientComission := settings.ClientCommission
	freelancerComission := settings.FreelancerCommission

	FreelancerComissionAmount := amount / 100 * settings.FreelancerCommission
	clientComissionAmount := amount / 100 * settings.ClientCommission
	totalCharges := FreelancerComissionAmount + clientComissionAmount + appFees

	netAmount := amount - totalCharges

	if charge.Status == "succeeded" {
		transaction := models.PaymentTransactionV2{
			PostingDate:       time.Now(),
			Amount:            decimal.NewFromFloat(amount),
			FromUserID:        contract.ClientID,
			JobPostID:         contract.JobPostID,
			ToUserID:          contract.FreelancerID,
			StripeEventID:     event.ID,
			AppFee:            decimal.NewFromFloat(amount),
			NetAmount:         decimal.NewFromFloat(netAmount),
			ClientCommPct:     clientComission,
			FreelancerCommPct: freelancerComission,
			Status:            models.PaymentStatusHeld,
		}

		if err := tx.Create(&transaction).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		escrow := models.EscrowTransaction{
			UserID:        contract.FreelancerID,
			Amount:        decimal.NewFromFloat(netAmount),
			TransactionID: transaction.ID,
			Status:        models.EscrowStatusHeld,
			ContractID:    contract.ID,
		}

		if err := tx.Create(&escrow).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create escrow record: %w", err)
		}

		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

type TypeWalletTransaction struct {
	Amount      float64   `json:"amount"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Date        time.Time `json:"date"`
	ID          string    `json:"id"`
}

type UserWalletVal struct {
	Balance      float64
	Transactions []TypeWalletTransaction
	EscrowAmount float64
}

func (s *Service) AddUserBankAccount(user *models.User, accountNo string, routingNo string, currency string, countryCode string, accountHolder string, bankName string) error {
	account := models.UserAccountDetails{
		UserID:        user.ID,
		AccountNo:     accountNo,
		CountryCode:   countryCode,
		Currency:      currency,
		RoutingNumber: routingNo,
		AccountHolder: accountHolder,
		BankName:      bankName,
	}

	if err := s.db.Create(&account).Error; err != nil {
		return err
	}
	return nil
}

func (s *Service) GetUserBalance(user *models.User) (float64, error) {
	released := 0.00
	paidout := 0.00

	if err := s.db.Model(&models.EscrowTransaction{}).
		Where("user_id = ? AND status = ? ", user.ID, models.EscrowStatusReleased).
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&released).Error; err != nil {

		return 0.00, fmt.Errorf("failed to sum escrows: %w", err)
	}

	if err := s.db.Model(&models.PayoutTransaction{}).
		Where("user_id = ? AND status = ?", user.ID, models.PayoutSuccess).
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&paidout).Error; err != nil {

		return 0.00, fmt.Errorf("failed to sum payouts: %w", err)
	}
	balance := released - paidout

	return balance, nil
}

func (s *Service) GetUserWallet(user *models.User, fromDate *time.Time, toDate *time.Time) (*UserWalletVal, error) {
	transactions := make([]TypeWalletTransaction, 0)

	totalPending := decimal.Zero
	if err := s.db.Model(&models.EscrowTransaction{}).
		Where("user_id = ? AND status = ?", user.ID, "held").
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&totalPending).Error; err != nil {
		return nil, fmt.Errorf("failed to sum held escrows: %w", err)
	}

	balance, err := s.GetUserBalance(user)
	if err != nil {
		return nil, err
	}

	paymentQuery := s.db.Model(&models.PaymentTransactionV2{}).
		Where("(from_user_id = ? OR to_user_id = ?)", user.ID, user.ID)
	if fromDate != nil {
		paymentQuery = paymentQuery.Where("posting_date >= ?", fromDate)
	}
	if toDate != nil {
		paymentQuery = paymentQuery.Where("posting_date <= ?", toDate)
	}

	var paymentTxns []models.PaymentTransactionV2
	if err := paymentQuery.Find(&paymentTxns).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch payment transactions: %w", err)
	}

	for _, p := range paymentTxns {
		amt, _ := p.Amount.Float64()
		netAmt, _ := p.NetAmount.Float64()

		if p.FromUserID == user.ID {
			transactions = append(transactions, TypeWalletTransaction{
				Amount:      amt,
				Type:        "debit",
				Description: "Contract payment sent",
				Date:        p.PostingDate,
				ID:          p.ID,
			})

		} else if p.ToUserID == user.ID {
			transactions = append(transactions, TypeWalletTransaction{
				Amount:      netAmt,
				Type:        "credit",
				Description: "Payment received (held in escrow)",
				Date:        p.PostingDate,
				ID:          p.ID,
			})
		}
	}

	escrowQuery := s.db.Where("user_id = ? AND status = ?", user.ID, models.EscrowStatusReleased)
	if fromDate != nil {
		escrowQuery = escrowQuery.Where("released_at >= ?", fromDate)
	}
	if toDate != nil {
		escrowQuery = escrowQuery.Where("released_at <= ?", toDate)
	}

	var filteredEscrows []models.EscrowTransaction
	if err := escrowQuery.Find(&filteredEscrows).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch escrow transactions: %w", err)
	}
	for _, e := range filteredEscrows {
		amt, _ := e.Amount.Float64()
		transactions = append(transactions, TypeWalletTransaction{
			Amount:      amt,
			Type:        "credit",
			Description: "Escrow released",
			Date:        *e.ReleasedAt,
			ID:          e.ID,
		})
	}

	payoutQuery := s.db.Where("user_id = ?", user.ID)
	if fromDate != nil {
		payoutQuery = payoutQuery.Where("transaction_date >= ?", fromDate)
	}
	if toDate != nil {
		payoutQuery = payoutQuery.Where("transaction_date <= ?", toDate)
	}

	var payoutsTnx []models.PayoutTransaction
	if err := payoutQuery.Find(&payoutsTnx).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch payout transactions: %w", err)
	}
	for _, p := range payoutsTnx {
		amt, _ := p.Amount.Float64()
		transactions = append(transactions, TypeWalletTransaction{
			Amount:      amt,
			Type:        "debit",
			Description: "Payout to bank account no:",
			Date:        p.TransactionDate,
		})
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Date.After(transactions[j].Date)
	})

	escrowAmount, _ := totalPending.Float64()

	return &UserWalletVal{
		Balance:      balance,
		Transactions: transactions,
		EscrowAmount: escrowAmount,
	}, nil
}

func (s *Service) CreatePayout(user *models.User, amount float64, account *models.UserAccountDetails) error {
	wallet, err := s.GetUserWallet(user, nil, nil)
	if err != nil {
		return err
	}

	balance := wallet.Balance
	if balance < amount {
		return fmt.Errorf("the user dont have that balance")
	}

	payoutAmount := int64(amount * 100)
	tx := s.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	tokenParams := &stripe.TokenParams{
		BankAccount: &stripe.BankAccountParams{
			Country:       stripe.String(account.CountryCode),
			Currency:      stripe.String(strings.ToLower(account.Currency)),
			AccountNumber: stripe.String(account.AccountNo),
			RoutingNumber: stripe.String(account.RoutingNumber),
		},
	}

	bankToken, err := token.New(tokenParams)
	if err != nil {
		return fmt.Errorf("failed to create bank token: %w", err)
	}

	params := &stripe.PayoutParams{
		Amount:      stripe.Int64(payoutAmount),
		Currency:    stripe.String(string(stripe.CurrencyUSD)),
		Destination: stripe.String(bankToken.ID),
		Method:      stripe.String("standard"),
	}

	params.IdempotencyKey = stripe.String("payout-" + user.ID + "-" + uuid.NewString())
	stripePayout, err := payout.New(params)

	if err != nil {
		return err
	}

	payoutTransaction := models.PayoutTransaction{
		UserID:          user.ID,
		Amount:          decimal.NewFromFloat(amount),
		BankAccountID:   &account.ID,
		Currency:        account.Currency,
		TransactionDate: time.Now(),
		Status:          models.PayoutPending,
		StripePayoutID:  stripePayout.ID,
	}

	if err := tx.Create(&payoutTransaction).Error; err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}
