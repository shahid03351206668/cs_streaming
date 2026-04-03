package payments

import (
	"fmt"
	"net/http"
	"tasksy/config"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	stripepayout "github.com/stripe/stripe-go/v84/payout"
	stripetransfer "github.com/stripe/stripe-go/v84/transfer"
	"gorm.io/gorm"
)

type PayoutService struct {
	db     *gorm.DB
	config *config.StripeConfig
}

func NewPayoutService(db *gorm.DB, cfg *config.StripeConfig) *PayoutService {
	return &PayoutService{db: db, config: cfg}
}

// GetWalletBalance handles GET /api/v1/wallet/balance
func (s *PayoutService) GetWalletBalance(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var fresh models.User
	if err := s.db.Select("id, wallet_balance, referral_reward_balance").
		First(&fresh, "id = ?", user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch balance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"wallet_balance":          fresh.WalletBalance,
			"referral_reward_balance": fresh.ReferralRewardBalance,
			"total_available":         fresh.WalletBalance + fresh.ReferralRewardBalance,
			"currency":                "gbp",
		},
	})
}

// RequestPayout handles POST /api/v1/wallet/withdraw
// Deducts from wallet_balance, initiates a Stripe Transfer + Payout to the user's default bank account.
func (s *PayoutService) RequestPayout(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	stripe.Key = s.config.SecretKey

	var req struct {
		Amount   int64  `json:"amount" binding:"required,min=1"`
		Currency string `json:"currency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}
	if req.Currency == "" {
		req.Currency = "gbp"
	}

	// Re-fetch wallet balance
	var fresh models.User
	if err := s.db.Select("id, wallet_balance").First(&fresh, "id = ?", user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch balance"})
		return
	}
	if req.Amount > fresh.WalletBalance {
		c.JSON(http.StatusBadRequest, gin.H{
			"message":           "error",
			"error":             "insufficient wallet balance",
			"available_balance": fresh.WalletBalance,
			"requested_amount":  req.Amount,
		})
		return
	}

	// Find default bank account
	var bankAccount models.UserBankAccount
	if err := s.db.Where("user_id = ? AND is_default = ?", user.ID, true).
		First(&bankAccount).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "no default bank account found. Please add a bank account first.",
		})
		return
	}

	// Atomically deduct balance
	result := s.db.Model(&models.User{}).
		Where("id = ? AND wallet_balance >= ?", user.ID, req.Amount).
		UpdateColumn("wallet_balance", gorm.Expr("wallet_balance - ?", req.Amount))
	if result.Error != nil || result.RowsAffected == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "insufficient balance or concurrent update"})
		return
	}

	// Transfer from platform balance → freelancer's Connect account
	transferParams := &stripe.TransferParams{
		Amount:      stripe.Int64(req.Amount),
		Currency:    stripe.String(req.Currency),
		Destination: stripe.String(bankAccount.StripeConnectAccountID),
	}
	tr, err := stripetransfer.New(transferParams)
	if err != nil {
		// Rollback balance deduction
		s.db.Model(&models.User{}).Where("id = ?", user.ID).
			UpdateColumn("wallet_balance", gorm.Expr("wallet_balance + ?", req.Amount))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to initiate transfer: " + err.Error()})
		return
	}

	// Payout from Connect account → bank account
	payoutParams := &stripe.PayoutParams{
		Amount:   stripe.Int64(req.Amount),
		Currency: stripe.String(req.Currency),
	}	
	payoutParams.SetStripeAccount(bankAccount.StripeConnectAccountID)
	po, poErr := stripepayout.New(payoutParams)

	stripePayoutID := ""
	payoutStatus := models.PaymentStatusPending
	if poErr == nil {
		stripePayoutID = po.ID
		if po.Status == stripe.PayoutStatusPaid {
			payoutStatus = models.PaymentStatusSuccess
		}
	}

	baID := bankAccount.ID
	payoutTx := models.PayoutTransaction{
		TransactionDate: time.Now(),
		UserID:          user.ID,
		Amount:          req.Amount,
		NetAmount:       req.Amount,
		AppFeeAmount:    0,
		Currency:        req.Currency,
		Status:          payoutStatus,
		StripeID:        tr.ID,
		StripePayoutID:  stripePayoutID,
		BankAccountID:   &baID,
	}
	if err := s.db.Create(&payoutTx).Error; err != nil {
		// Rollback balance deduction
		s.db.Model(&models.User{}).Where("id = ?", user.ID).
			UpdateColumn("wallet_balance", gorm.Expr("wallet_balance + ?", req.Amount))
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to record withdrawal"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payout_id":          payoutTx.ID,
			"amount":             payoutTx.Amount,
			"currency":           payoutTx.Currency,
			"status":             payoutTx.Status,
			"stripe_transfer_id": tr.ID,
			"stripe_payout_id":   stripePayoutID,
			"bank_account": gin.H{
				"account_holder_name":  bankAccount.AccountHolderName,
				"sort_code":            bankAccount.SortCode,
				"account_number_last4": bankAccount.AccountNumberLast4,
				"bank_name":            bankAccount.BankName,
			},
			"remaining_balance": fresh.WalletBalance - req.Amount,
		},
	})
}

// GetPayoutHistory handles GET /api/v1/wallet/withdrawals
func (s *PayoutService) GetPayoutHistory(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var payouts []models.PayoutTransaction
	if err := s.db.Where("user_id = ?", user.ID).
		Preload("BankAccount").
		Order("transaction_date DESC").
		Find(&payouts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch withdrawal history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": payouts})
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
