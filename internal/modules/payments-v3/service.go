package paymentsv3

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"tasksy/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/bankaccount"
	"github.com/stripe/stripe-go/v84/paymentintent"
	"github.com/stripe/stripe-go/v84/token"
	"github.com/stripe/stripe-go/v84/transfer"
	"gorm.io/gorm"
)

type SystemSetting struct {
	AppFee               int64
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
	Balance      float64
	Transactions []TypeWalletTransaction
	EscrowAmount float64
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

func (s *Service) CreatePaymentIntent(client, freelancer *models.User, proposalID string, amount float64) (*stripe.PaymentIntent, error) {
	if freelancer.StripeConnectAccountID == "" {
		return nil, errors.New("freelancer does not have a stripe account")
	}

	settings, err := s.GetSystemSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to load system settings: %w", err)
	}

	platformFee := 0.0
	if settings.ClientCommission > 0 {
		platformFee = amount * (settings.ClientCommission / 100)
	}

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(int64(amount * 100)),
		Currency: stripe.String(string(stripe.CurrencyGBP)),
		TransferData: &stripe.PaymentIntentTransferDataParams{
			Destination: stripe.String(freelancer.StripeConnectAccountID),
		},

		ApplicationFeeAmount: stripe.Int64(int64(platformFee * 100)),
		Metadata: map[string]string{
			"proposal_id":           proposalID,
			"from_user_id":          client.ID,
			"to_user_id":            freelancer.ID,
			"freelancer_connect_id": freelancer.StripeConnectAccountID,
			"platform_fee_amount":   fmt.Sprintf("%d", int64(platformFee*100)),
		},
	}

	params.IdempotencyKey = stripe.String("v3-intent-" + proposalID + uuid.NewString() + time.Now().String())
	return paymentintent.New(params)
}

func (s *Service) AddBankAccount(user *models.User, accountHolderName, sortCode, accountNumber string) error {
	if user.StripeConnectAccountID == "" {
		return errors.New("stripe account not provisioned for this user")
	}

	tok, err := token.New(&stripe.TokenParams{
		BankAccount: &stripe.BankAccountParams{
			Country:           stripe.String("GB"),
			Currency:          stripe.String("gbp"),
			AccountHolderName: stripe.String(accountHolderName),
			AccountHolderType: stripe.String("individual"),
			RoutingNumber:     stripe.String(sortCode),
			AccountNumber:     stripe.String(accountNumber),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tokenize bank account: %w", err)
	}

	_, err = bankaccount.New(&stripe.BankAccountParams{
		Params:  stripe.Params{},
		Account: stripe.String(user.StripeConnectAccountID),
		Token:   stripe.String(tok.ID),
	})
	if err != nil {
		return fmt.Errorf("failed to attach bank account: %w", err)
	}

	return s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("stripe_connect_onboarded", true).Error
}

func (s *Service) ReleaseEscrow(escrowID string, requestingUserID string) error {
	var escrow models.EscrowTransactionV3
	if err := s.db.First(&escrow, "id = ?", escrowID).Error; err != nil {
		return fmt.Errorf("escrow not found: %w", err)
	}

	if escrow.Status != "held" {
		return errors.New("escrow is not in held state")
	}

	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", escrow.ContractID).Error; err != nil {
		return fmt.Errorf("contract not found: %w", err)
	}

	if requestingUserID != contract.ClientID {
		return errors.New("only the client can release this escrow")
	}

	var freelancer models.User
	if err := s.db.First(&freelancer, "id = ?", escrow.UserID).Error; err != nil {
		return fmt.Errorf("freelancer not found: %w", err)
	}

	amountCents, _ := escrow.Amount.Mul(decimal.NewFromInt(100)).Float64()

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	tr, err := transfer.New(&stripe.TransferParams{
		Amount:      stripe.Int64(int64(amountCents)),
		Currency:    stripe.String(escrow.Currency),
		Destination: stripe.String(freelancer.StripeConnectAccountID),
	})
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create stripe transfer: %w", err)
	}

	if err := tx.Model(&escrow).Updates(map[string]any{
		"status":             "released",
		"stripe_transfer_id": tr.ID,
		"released_at":        time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update escrow: %w", err)
	}

	return tx.Commit().Error
}

func (s *Service) GetUserWallet(user *models.User) (*UserWalletVal, error) {
	transactions := make([]TypeWalletTransaction, 0)

	totalPending := decimal.Zero
	if err := s.db.Model(&models.EscrowTransactionV3{}).
		Where("user_id = ? AND status = ?", user.ID, "held").
		Select("COALESCE(SUM(amount::numeric), 0)").
		Scan(&totalPending).Error; err != nil {
		return nil, fmt.Errorf("failed to sum held escrows: %w", err)
	}

	balance := 0.0
	if err := s.db.Model(&models.EscrowTransactionV3{}).
		Where("user_id = ? AND status = ?", user.ID, "released").
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
			transactions = append(transactions, TypeWalletTransaction{
				Amount:      gross,
				Type:        "debit",
				Description: "Contract payment sent",
				Date:        p.CreatedAt,
				ID:          p.ID,
			})
		} else if p.ToUserID == user.ID {
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
	if err := s.db.Where("user_id = ? AND status = ?", user.ID, "released").
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

	proposalID := charge.Metadata["proposal_id"]
	if proposalID == "" {
		return fmt.Errorf("proposal id is not found in the meta data")
	}

	fromUserID := charge.Metadata["from_user_id"]
	toUserID := charge.Metadata["to_user_id"]

	var contract models.Contract
	if err := s.db.Where("proposal_id = ?", proposalID).First(&contract).Error; err != nil {
		return err
	}

	settings, err := s.GetSystemSettings()
	if err != nil {
		return fmt.Errorf("failed to load system settings: %w", err)
	}

	grossAmount := decimal.NewFromInt(charge.Amount).Div(decimal.NewFromInt(100))
	platformFeePct := decimal.NewFromFloat(settings.ClientCommission)
	platformFee := decimal.Zero
	if platformFeePct.IsPositive() {
		platformFee = grossAmount.Mul(platformFeePct).Div(decimal.NewFromInt(100))
	}
	netAmount := grossAmount.Sub(platformFee)

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	transaction := models.PaymentTransactionV3{
		StripePaymentIntentID: charge.PaymentIntent.ID,
		StripeChargeID:        charge.ID,
		StripeEventID:         event.ID,
		FromUserID:            fromUserID,
		ToUserID:              toUserID,
		ContractID:            contract.ID,
		GrossAmount:           grossAmount,
		PlatformFee:           platformFee,
		NetAmount:             netAmount,
		Currency:              string(charge.Currency),
		Status:                "held",
		FlowVersion:           "v3",
	}

	if err := tx.Create(&transaction).Error; err != nil {
		tx.Rollback()
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

	var escrow models.EscrowTransactionV3
	if err := s.db.Where("stripe_transfer_id = ?", tr.ID).First(&escrow).Error; err != nil {
		return nil
	}

	if escrow.Status == "released" || escrow.Status == "refunded" {
		return nil
	}

	now := time.Now()
	return s.db.Model(&escrow).Updates(map[string]any{
		"status":      "released",
		"released_at": &now,
	}).Error
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

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Model(&transaction).Update("status", "refunded").Error; err != nil {
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
