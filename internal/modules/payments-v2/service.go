package paymentsv2

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"tasksy/models"

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

	tx := s.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	settings, _ := s.GetSystemSettings()

	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		fmt.Println(err.Error())
		return err
	}

	proposalID := charge.Metadata["proposal_id"]
	if proposalID == "" {
		return fmt.Errorf("proposal id is not found in the meta data")
	}

	var contract models.Contract
	if err := s.db.Where("proposal_id = ?", proposalID).First(&contract).Error; err != nil {
		return err
	}

	amount := decimal.NewFromInt(charge.Amount / 100)
	appFees := settings.AppFee

	clientComission := settings.ClientCommission
	freelancerComission := settings.FreelancerCommission

	FreelancerComissionAmount := amount.Div(decimal.NewFromFloat(100)).Mul(decimal.NewFromFloat(settings.FreelancerCommission))
	clientComissionAmount := amount.Div(decimal.NewFromInt(100)).Mul(decimal.NewFromFloat(settings.ClientCommission))

	totalCharges := FreelancerComissionAmount.Add(clientComissionAmount).Add(decimal.NewFromInt(appFees))

	netAmount := amount.Sub(totalCharges)

	if charge.Status == "succeeded" {
		transaction := models.PaymentTransactionV2{
			PostingDate:       time.Now(),
			Amount:            amount,
			FromUserID:        contract.ClientID,
			ToUserID:          contract.FreelancerID,
			StripeEventID:     event.ID,
			AppFee:            decimal.NewFromInt(appFees),
			NetAmount:         netAmount,
			ClientCommPct:     clientComission,
			FreelancerCommPct: freelancerComission,
			Status:            models.PaymentStatusHeld,
		}

		if err := tx.Create(&transaction).Error; err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		escrow := models.EscrowTransaction{
			UserID:        contract.FreelancerID,
			Amount:        netAmount,
			TransactionID: transaction.ID,
			Status:        models.EscrowStatusHeld,
			ContractID:    contract.ID,
		}

		if err := tx.Create(&escrow).Error; err != nil {
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
	escrowAmount float64
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

func (s *Service) GetUserWallet(user *models.User, fromDate *time.Time, toDate *time.Time) (*UserWalletVal, error) {
	transactions := make([]TypeWalletTransaction, 0)

	totalReleased := decimal.Zero
	if err := s.db.Model(&models.EscrowTransaction{}).
		Where("user_id = ? AND status = ?", user.ID, models.EscrowStatusReleased).
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&totalReleased).Error; err != nil {
		return nil, fmt.Errorf("failed to sum released escrows: %w", err)
	}

	totalPaidOut := decimal.Zero
	if err := s.db.Model(&models.PayoutTransaction{}).
		Where("user_id = ? AND status = ?", user.ID, models.PayoutSuccess).
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&totalPaidOut).Error; err != nil {
		return nil, fmt.Errorf("failed to sum successful payouts: %w", err)
	}

	// totalPending := decimal.Zero
	// if err := s.db.Model(&models.EscrowTransaction{}).
	// 	Where("user_id = ? AND status = ?", user.ID, models.PayoutPending).
	// 	Select("COALESCE(SUM(amount::numeric), 0)").
	// 	Scan(&totalPending).Error; err != nil {
	// 	return nil, fmt.Errorf("failed to sum pending payouts: %w", err)
	// }

	balance, _ := totalReleased.Sub(totalPaidOut).Float64()
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
	totalPending := decimal.Zero
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
			totalPending.Add(decimal.NewFromFloat(netAmt))
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
			ID:          *&e.ID,
		})
	}

	payoutQuery := s.db.Where("user_id = ?", user.ID)
	if fromDate != nil {
		payoutQuery = payoutQuery.Where("transaction_date >= ?", fromDate)
	}
	if toDate != nil {
		payoutQuery = payoutQuery.Where("transaction_date <= ?", toDate)
	}

	var filteredPayouts []models.PayoutTransaction
	if err := payoutQuery.Find(&filteredPayouts).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch payout transactions: %w", err)
	}
	for _, p := range filteredPayouts {
		amt, _ := p.Amount.Float64()
		transactions = append(transactions, TypeWalletTransaction{
			Amount:      amt,
			Type:        "debit",
			Description: "Payout to bank",
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
		escrowAmount: escrowAmount,
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

	payoutAmount := int64(amount / 100)
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

	params.IdempotencyKey = stripe.String("payout-" + user.ID + "-" + fmt.Sprint(time.Now().Unix()))
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
