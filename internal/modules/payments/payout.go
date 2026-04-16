package payments

import (
	"fmt"
	"net/http"
	"tasksy/config"
	"tasksy/models"
	"tasksy/pkg/logger"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	stripepayout "github.com/stripe/stripe-go/v84/payout"
	stripetransfer "github.com/stripe/stripe-go/v84/transfer"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type PayoutService struct {
	db     *gorm.DB
	config *config.StripeConfig
	ledger *LedgerService
}

func NewPayoutService(db *gorm.DB, cfg *config.StripeConfig, ledger *LedgerService) *PayoutService {
	return &PayoutService{db: db, config: cfg, ledger: ledger}
}

// GetWalletBalance handles GET /api/v1/wallet/balance
// Balance is derived from the double-entry ledger (sum of all gl_entries for the user's wallet account).
func (s *PayoutService) GetWalletBalance(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	balance, err := s.ledger.GetUserWalletBalance(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"wallet_balance":          balance,
			"referral_reward_balance": int64(0),
			"total_available":         balance,
			"currency":                "gbp",
		},
	})
}

func (s *PayoutService) RequestPayout(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	stripe.Key = s.config.SecretKey

	var req struct {
		Amount        int64  `json:"amount" binding:"required,min=1"`
		Currency      string `json:"currency"`
		BankAccountID string `json:"bank_account_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}
	if req.Currency == "" {
		req.Currency = "gbp"
	}

	// Check balance from ledger (source of truth).
	currentBalance, err := s.ledger.GetUserWalletBalance(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch balance"})
		return
	}
	if req.Amount > currentBalance {
		c.JSON(http.StatusBadRequest, gin.H{
			"message":           "error",
			"error":             "insufficient wallet balance",
			"available_balance": currentBalance,
			"requested_amount":  req.Amount,
		})
		return
	}

	// Find bank account — use provided ID or fall back to default.
	var bankAccount models.UserBankAccount
	query := s.db.Where("user_id = ?", user.ID)
	if req.BankAccountID != "" {
		query = query.Where("id = ?", req.BankAccountID)
	} else {
		query = query.Where("is_default = ?", true)
	}
	if err := query.First(&bankAccount).Error; err != nil {
		msg := "no default bank account found. Please add a bank account first."
		if req.BankAccountID != "" {
			msg = "bank account not found"
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": msg})
		return
	}

	if bankAccount.StripeConnectAccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "bank account is not linked to a payout account. Please re-add your bank account."})
		return
	}

	// Atomically create the payout ledger entry and payout record within a single transaction.
	// Debit: UserWallet (-amount), Credit: External (+amount).
	// This prevents double-spending — the balance reflects the pending withdrawal immediately.
	baID := bankAccount.ID
	payoutTx := models.PayoutTransaction{
		TransactionDate: time.Now(),
		UserID:          user.ID,
		Amount:          req.Amount,
		NetAmount:       req.Amount,
		AppFeeAmount:    0,
		Currency:        req.Currency,
		Status:          models.PaymentStatusPending,
		BankAccountID:   &baID,
	}

	var ledgerTxID string
	if txErr := s.db.Transaction(func(tx *gorm.DB) error {
		ledgerInTx := NewLedgerService(tx)

		userWallet, err := ledgerInTx.GetOrCreateUserAccount(user.ID)
		if err != nil {
			return fmt.Errorf("failed to get user wallet account: %w", err)
		}
		externalAcct, err := ledgerInTx.GetSystemAccount(models.AccountTypeExternal)
		if err != nil {
			return fmt.Errorf("external account not found: %w", err)
		}

		// Debit user wallet, Credit external — sum = -amount + amount = 0 ✓
		ledgerTxn, err := ledgerInTx.CreateLedgerTransaction(
			models.LedgerTxPayout,
			user.ID,
			fmt.Sprintf("Wallet withdrawal of %d %s", req.Amount, req.Currency),
			[]EntryInput{
				{AccountID: userWallet.ID, Amount: -req.Amount, Category: "wallet_withdrawal"},
				{AccountID: externalAcct.ID, Amount: req.Amount, Category: "wallet_withdrawal"},
			},
		)
		if err != nil {
			return fmt.Errorf("failed to create payout ledger entries: %w", err)
		}
		ledgerTxID = ledgerTxn.ID

		return tx.Create(&payoutTx).Error
	}); txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to initiate withdrawal"})
		return
	}

	// Transfer from platform balance → freelancer's Connect account.
	transferParams := &stripe.TransferParams{
		Amount:      stripe.Int64(req.Amount),
		Currency:    stripe.String(req.Currency),
		Destination: stripe.String(bankAccount.StripeConnectAccountID),
	}
	tr, err := stripetransfer.New(transferParams)
	if err != nil {
		// Stripe failed — create a reversing ledger entry to restore the balance (immutability rule).
		s.reversePayoutLedger(user.ID, req.Amount, req.Currency, ledgerTxID)
		s.db.Model(&payoutTx).Update("status", models.PaymentStatusFailed)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to initiate transfer: " + err.Error()})
		return
	}

	// Payout from Connect account → bank account.
	payoutParams := &stripe.PayoutParams{
		Amount:   stripe.Int64(req.Amount),
		Currency: stripe.String(req.Currency),
	}
	payoutParams.SetStripeAccount(bankAccount.StripeConnectAccountID)
	po, poErr := stripepayout.New(payoutParams)

	stripePayoutID := ""
	payoutStatus := models.PaymentStatusPending
	var payoutWarning string
	if poErr != nil {
		logger.Log.Error("stripe payout creation failed after transfer",
			zap.String("user_id", user.ID),
			zap.String("transfer_id", tr.ID),
			zap.String("connect_account", bankAccount.StripeConnectAccountID),
			zap.Error(poErr),
		)
		payoutWarning = "Transfer succeeded but payout to bank is pending. Contact support if funds are not received within 2 business days."
	} else {
		stripePayoutID = po.ID
		if po.Status == stripe.PayoutStatusPaid {
			payoutStatus = models.PaymentStatusSuccess
		}
	}

	// Update payout record with Stripe IDs and final status.
	s.db.Model(&payoutTx).Updates(map[string]interface{}{
		"stripe_id":       tr.ID,
		"stripe_payout_id": stripePayoutID,
		"status":          payoutStatus,
	})

	remainingBalance := currentBalance - req.Amount
	resp := gin.H{
		"payout_id":          payoutTx.ID,
		"amount":             payoutTx.Amount,
		"currency":           payoutTx.Currency,
		"status":             payoutStatus,
		"stripe_transfer_id": tr.ID,
		"stripe_payout_id":   stripePayoutID,
		"bank_account": gin.H{
			"account_holder_name":  bankAccount.AccountHolderName,
			"sort_code":            bankAccount.SortCode,
			"account_number":       bankAccount.AccountNumber,
			"account_number_last4": bankAccount.AccountNumberLast4,
			"bank_name":            bankAccount.BankName,
		},
		"remaining_balance": remainingBalance,
	}
	if payoutWarning != "" {
		resp["warning"] = payoutWarning
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

// reversePayoutLedger creates a reversing ledger transaction when a Stripe transfer fails.
// Credit: UserWallet (+amount), Debit: External (-amount) — exactly cancels the original entry.
func (s *PayoutService) reversePayoutLedger(userID string, amount int64, currency string, originalLedgerTxID string) {
	userWallet, err := s.ledger.GetOrCreateUserAccount(userID)
	if err != nil {
		logger.Log.Error("reversal: failed to get user wallet account", zap.String("user_id", userID), zap.Error(err))
		return
	}
	externalAcct, err := s.ledger.GetSystemAccount(models.AccountTypeExternal)
	if err != nil {
		logger.Log.Error("reversal: external account not found", zap.Error(err))
		return
	}
	_, err = s.ledger.CreateLedgerTransaction(
		models.LedgerTxPayout,
		originalLedgerTxID,
		fmt.Sprintf("Reversal of failed withdrawal of %d %s", amount, currency),
		[]EntryInput{
			{AccountID: userWallet.ID, Amount: amount, Category: "withdrawal_reversal"},
			{AccountID: externalAcct.ID, Amount: -amount, Category: "withdrawal_reversal"},
		},
	)
	if err != nil {
		logger.Log.Error("reversal: failed to create reversing ledger entry", zap.String("user_id", userID), zap.Error(err))
	}
}

// GetPayoutHistory handles GET /api/v1/wallet/withdrawals
func (s *PayoutService) GetPayoutHistory(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	type BankAccountSummary struct {
		AccountHolderName  string `json:"account_holder_name"`
		BankName           string `json:"bank_name"`
		SortCode           string `json:"sort_code"`
		AccountNumber      string `json:"account_number"`
		AccountNumberLast4 string `json:"account_number_last4"`
		BankLogoURL        string `json:"bank_logo_url"`
	}
	type PayoutRow struct {
		ID              string              `json:"id"`
		TransactionDate time.Time           `json:"transaction_date"`
		Amount          int64               `json:"amount"`
		NetAmount       int64               `json:"net_amount"`
		Currency        string              `json:"currency"`
		Status          string              `json:"status"`
		StripeID        string              `json:"stripe_transfer_id"`
		StripePayoutID  string              `json:"stripe_payout_id"`
		BankAccount     *BankAccountSummary `json:"bank_account,omitempty"`
	}

	type rawPayout struct {
		models.PayoutTransaction
		BankAccount *models.UserBankAccount `gorm:"foreignKey:BankAccountID"`
	}

	var payouts []models.PayoutTransaction
	if err := s.db.Where("user_id = ?", user.ID).
		Preload("BankAccount").
		Order("transaction_date DESC").
		Find(&payouts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch withdrawal history"})
		return
	}

	rows := make([]PayoutRow, 0, len(payouts))
	for _, p := range payouts {
		row := PayoutRow{
			ID:              p.ID,
			TransactionDate: p.TransactionDate,
			Amount:          p.Amount,
			NetAmount:       p.NetAmount,
			Currency:        p.Currency,
			Status:          p.Status,
			StripeID:        p.StripeID,
			StripePayoutID:  p.StripePayoutID,
		}
		if p.BankAccount != nil {
			row.BankAccount = &BankAccountSummary{
				AccountHolderName:  p.BankAccount.AccountHolderName,
				BankName:           p.BankAccount.BankName,
				SortCode:           p.BankAccount.SortCode,
				AccountNumber:      p.BankAccount.AccountNumber,
				AccountNumberLast4: p.BankAccount.AccountNumberLast4,
				BankLogoURL:        p.BankAccount.BankLogoURL,
			}
		}
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": rows})
}

// AdminListPayouts handles GET /api/v1/admin/payouts
func (s *PayoutService) AdminListPayouts(c *gin.Context) {
	type PayoutRow struct {
		ID              string    `json:"id"`
		TransactionDate time.Time `json:"transaction_date"`
		UserID          string    `json:"user_id"`
		UserFirstName   string    `json:"user_first_name"`
		UserLastName    string    `json:"user_last_name"`
		UserEmail       string    `json:"user_email"`
		Amount          int64     `json:"amount"`
		NetAmount       int64     `json:"net_amount"`
		AppFeeAmount    int64     `json:"app_fee_amount"`
		Currency        string    `json:"currency"`
		Status          string    `json:"status"`
		StripeID        string    `json:"stripe_transfer_id"`
		StripePayoutID  string    `json:"stripe_payout_id"`
		BankName        string    `json:"bank_name"`
		AccountLast4    string    `json:"account_number_last4"`
		SortCode        string    `json:"sort_code"`
	}

	status := c.Query("status")
	userID := c.Query("user_id")

	limit := 20
	page := 0
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := s.db.Model(&models.PayoutTransaction{}).
		Joins("JOIN users ON users.id = payout_transactions.user_id").
		Joins("LEFT JOIN user_bank_accounts ON user_bank_accounts.id = payout_transactions.bank_account_id").
		Select(`payout_transactions.id, payout_transactions.transaction_date,
			payout_transactions.user_id, users.first_name AS user_first_name,
			users.last_name AS user_last_name, users.email AS user_email,
			payout_transactions.amount, payout_transactions.net_amount,
			payout_transactions.app_fee_amount, payout_transactions.currency,
			payout_transactions.status, payout_transactions.stripe_id,
			payout_transactions.stripe_payout_id,
			user_bank_accounts.bank_name, user_bank_accounts.account_number_last4,
			user_bank_accounts.sort_code`)

	if status != "" {
		query = query.Where("payout_transactions.status = ?", status)
	}
	if userID != "" {
		query = query.Where("payout_transactions.user_id = ?", userID)
	}

	var total int64
	query.Count(&total)

	var rows []PayoutRow
	if err := query.Order("payout_transactions.transaction_date DESC").
		Limit(limit).Offset(page * limit).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	if rows == nil {
		rows = []PayoutRow{}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"total":   total,
		"page":    page,
		"limit":   limit,
		"data":    rows,
	})
}
