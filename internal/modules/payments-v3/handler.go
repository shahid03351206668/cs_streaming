package paymentsv3

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"tasksy/config"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"
	"gorm.io/gorm"
)

type Handler struct {
	config  *config.Config
	service *Service
}

func NewHandler(config *config.Config, service *Service) *Handler {
	return &Handler{config: config, service: service}
}

func (h *Handler) HandleCreatePaymentIntent(c *gin.Context) {
	client := c.MustGet("user").(models.User)

	// "amount" is accepted for backward compatibility but ignored — the charge
	// is always computed server-side from the proposal bid.
	var body struct {
		ProposalID string `json:"proposal_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	var proposal models.Proposal
	if err := h.service.db.Preload("JobPost").First(&proposal, "id = ?", body.ProposalID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "proposal not found"})
		return
	}

	if proposal.JobPost.CreatedByID != client.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": "only the job owner can pay for this proposal"})
		return
	}

	var freelancer models.User
	if err := h.service.db.First(&freelancer, "id = ?", proposal.FreelancerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "freelancer not found"})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey
	intent, fees, err := h.service.CreatePaymentIntent(&client, &freelancer, &proposal)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"client_secret":     intent.ClientSecret,
			"payment_intent_id": intent.ID,
			"amount":            fees.ClientTotal.InexactFloat64(),
			"currency":          PaymentCurrency,
			"payment_summary":   fees.Summary(),
		},
	})
}

func (h *Handler) HandleAddBankAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var body struct {
		AccountHolderName string `json:"account_holder_name" binding:"required"`
		SortCode          string `json:"sort_code" binding:"required"`
		AccountNumber     string `json:"account_number" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey

	record, err := h.service.AddBankAccount(&user, body.AccountHolderName, body.SortCode, body.AccountNumber)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": record})
}

func (h *Handler) HandleUpdateBankAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	bankAccountID := c.Param("id")

	var body struct {
		AccountHolderName string `json:"account_holder_name" binding:"required"`
		SortCode          string `json:"sort_code" binding:"required"`
		AccountNumber     string `json:"account_number" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey
	record, err := h.service.UpdateBankAccount(&user, bankAccountID, body.AccountHolderName, body.SortCode, body.AccountNumber)
	if err != nil {
		if err.Error() == "bank account not found" {
			c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": record})
}

func (h *Handler) HandleDeleteBankAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	bankAccountID := c.Param("id")

	stripe.Key = h.config.Stripe.SecretKey

	if err := h.service.DeleteBankAccount(&user, bankAccountID); err != nil {
		if err.Error() == "bank account not found" {
			c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) HandleGetBankAccounts(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	accounts, err := h.service.GetBankAccounts(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": accounts})
}

func (h *Handler) HandleReleaseEscrow(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	escrowID := c.Param("id")

	stripe.Key = h.config.Stripe.SecretKey

	released, err := h.service.ReleaseEscrow(escrowID, user.ID)
	respondRelease(c, released, err)
}

// HandleReleaseContract lets either party retry release by contract ID, e.g.
// after auto-release failed or a dispute on a completed contract was resolved.
func (h *Handler) HandleReleaseContract(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	stripe.Key = h.config.Stripe.SecretKey

	released, err := h.service.ReleaseForParty(c.Param("id"), user.ID)
	respondRelease(c, released, err)
}

func respondRelease(c *gin.Context, released int, err error) {
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, ErrNotContractParty):
			status = http.StatusForbidden
		case errors.Is(err, ErrContractDisputed), errors.Is(err, ErrContractNotCompleted), errors.Is(err, ErrNothingToRelease):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"message": "error", "error": err.Error(), "released": released})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "released": released})
}

func (h *Handler) HandleGetPayoutBalance(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	stripe.Key = h.config.Stripe.SecretKey

	bal, err := h.service.GetPayoutBalance(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"currency":  PaymentCurrency,
			"available": bal.Available.InexactFloat64(),
			"pending":   bal.Pending.InexactFloat64(),
		},
	})
}

func (h *Handler) HandleWithdraw(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	// Omit "amount" to withdraw everything available; omit "bank_account_id"
	// to use the default bank account on Stripe.
	var body struct {
		Amount        *float64 `json:"amount"`
		BankAccountID string   `json:"bank_account_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey

	withdrawal, err := h.service.Withdraw(&user, body.Amount, body.BankAccountID)
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, ErrBankAccountNotFound):
			status = http.StatusNotFound
		case errors.Is(err, ErrNoStripeAccount), errors.Is(err, ErrNoBankAccount),
			errors.Is(err, ErrBankAccountNotLinked),
			errors.Is(err, ErrNothingToWithdraw), errors.Is(err, ErrInvalidAmount),
			errors.Is(err, ErrInsufficientBalance):
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": withdrawal})
}

func (h *Handler) HandleListWithdrawals(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	withdrawals, err := h.service.ListWithdrawals(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": withdrawals})
}

func (h *Handler) HandleGetWallet(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	wallet, err := h.service.GetUserWallet(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"balance":       wallet.Balance,
			"transactions":  wallet.Transactions,
			"escrow_amount": wallet.EscrowAmount,
		},
	})
}

// HandleRefund is admin-only (see router.go) and delegates to the same
// escrow-aware refund logic as HandleAdminRefundContract: it refuses a
// transaction that's already refunded and reverses the Stripe transfer first
// if the escrow was already released to the freelancer.
func (h *Handler) HandleRefund(c *gin.Context) {
	admin := c.MustGet("user").(models.User)
	transactionID := c.Param("id")

	var body struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "a reason for the refund is required"})
		return
	}

	var transaction models.PaymentTransactionV3
	if err := h.service.db.First(&transaction, "id = ?", transactionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "payment transaction not found"})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey

	refunded, err := h.service.AdminRefundContract(transaction.ContractID, admin.ID, body.Reason)
	if err != nil {
		respondRefund(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": refunded})
}

func (h *Handler) HandleListTransactions(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	type TxRow struct {
		ID              string    `json:"id"`
		TransactionDate time.Time `json:"transaction_date"`
		FromUserID      string    `json:"from_user_id"`
		ToUserID        string    `json:"to_user_id"`
		ContractID      string    `json:"contract_id"`
		GrossAmount     float64   `json:"gross_amount"`
		PlatformFee     float64   `json:"platform_fee"`
		NetAmount       float64   `json:"net_amount"`
		Currency        string    `json:"currency"`
		Status          string    `json:"status"`
		Direction       string    `json:"direction"` // "paid" | "received"
	}

	var paid []TxRow
	h.service.db.Model(&models.PaymentTransactionV3{}).
		Where("from_user_id = ?", user.ID).
		Select("id, created_at as transaction_date, from_user_id, to_user_id, contract_id, gross_amount, platform_fee, net_amount, currency, status").
		Order("created_at DESC").
		Find(&paid)
	for i := range paid {
		paid[i].Direction = "paid"
	}

	var received []TxRow
	h.service.db.Model(&models.PaymentTransactionV3{}).
		Where("to_user_id = ?", user.ID).
		Select("id, created_at as transaction_date, from_user_id, to_user_id, contract_id, gross_amount, platform_fee, net_amount, currency, status").
		Order("created_at DESC").
		Find(&received)
	for i := range received {
		received[i].Direction = "received"
	}

	transactions := append(paid, received...)

	var totalPaid, totalEarned, totalPlatformFees float64
	for _, t := range paid {
		if t.Status == "held" || t.Status == "released" {
			totalPaid += t.GrossAmount
			totalPlatformFees += t.PlatformFee
		}
	}
	for _, t := range received {
		if t.Status == "released" {
			totalEarned += t.NetAmount
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"summary": gin.H{
			"total_paid":          totalPaid,
			"total_earned":        totalEarned,
			"total_platform_fees": totalPlatformFees,
			"total_transactions":  len(transactions),
			"paid_count":          len(paid),
			"received_count":      len(received),
		},
		"transactions": transactions,
	})
}

func (h *Handler) HandleAdminListTransactions(c *gin.Context) {
	page := 1
	limit := 20
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	status := c.Query("status")
	fromDate := c.Query("from_date")
	toDate := c.Query("to_date")
	search := c.Query("search")

	query := h.service.db.Model(&models.PaymentTransactionV3{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if fromDate != "" {
		if t, err := time.Parse("2006-01-02", fromDate); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if toDate != "" {
		if t, err := time.Parse("2006-01-02", toDate); err == nil {
			query = query.Where("created_at <= ?", t.Add(24*time.Hour))
		}
	}
	if search != "" {
		pattern := "%" + search + "%"
		query = query.Where("id ILIKE ? OR contract_id ILIKE ? OR stripe_payment_intent_id ILIKE ?", pattern, pattern, pattern)
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * limit
	var transactions []models.PaymentTransactionV3
	query.
		Preload("FromUser").
		Preload("ToUser").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transactions,
		"meta": gin.H{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

func (h *Handler) HandleAdminGetTransaction(c *gin.Context) {
	id := c.Param("id")

	var transaction models.PaymentTransactionV3
	if err := h.service.db.Preload("FromUser").Preload("ToUser").
		First(&transaction, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "transaction not found"})
		return
	}

	var escrow models.EscrowTransactionV3
	h.service.db.Where("payment_transaction_id = ?", transaction.ID).First(&escrow)

	var contract models.Contract
	var jobPost models.JobPost
	var proposalCount int64
	var proposal models.Proposal

	if transaction.ContractID != "" {
		h.service.db.First(&contract, "id = ?", transaction.ContractID)
		if contract.JobPostID != "" {
			h.service.db.First(&jobPost, "id = ?", contract.JobPostID)
			h.service.db.Model(&models.Proposal{}).Where("job_post_id = ?", contract.JobPostID).Count(&proposalCount)
		}
		if contract.ProposalID != "" {
			h.service.db.First(&proposal, "id = ?", contract.ProposalID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"transaction": transaction,
			"escrow":      escrow,
			"contract": gin.H{
				"id":                   contract.ID,
				"title":                contract.Title,
				"status":               contract.Status,
				"total_amount":         contract.TotalAmount,
				"client_completed":     contract.ClientCompleted,
				"freelancer_completed": contract.FreelancerCompleted,
				"start_date":           contract.StartDate,
				"end_date":             contract.EndDate,
			},
			"job_post": gin.H{
				"id":          jobPost.ID,
				"title":       jobPost.Title,
				"status":      jobPost.Status,
				"budget":      jobPost.Budget,
				"open_budget": jobPost.OpenBudget,
				"posted_at":   jobPost.CreatedAt,
			},
			"proposal": gin.H{
				"id":         proposal.ID,
				"status":     proposal.Status,
				"bid_amount": proposal.BidAmount,
				"duration":   proposal.Duration,
			},
			"total_proposals": proposalCount,
		},
	})
}

func (h *Handler) HandleAdminPaymentStats(c *gin.Context) {
	days := 30
	if d, err := strconv.Atoi(c.Query("days")); err == nil && d > 0 && d <= 365 {
		days = d
	}
	since := time.Now().AddDate(0, 0, -days)

	type DailyRow struct {
		Date        string  `json:"date"`
		GrossVolume float64 `json:"gross_volume"`
		PlatformFee float64 `json:"platform_fee"`
		Count       int64   `json:"count"`
	}

	daily := make([]DailyRow, 0)
	var query *gorm.DB = h.service.db.Model(&models.PaymentTransactionV3{})
	query.
		Select("TO_CHAR(created_at, 'YYYY-MM-DD') as date, COALESCE(SUM(gross_amount::numeric), 0) as gross_volume, COALESCE(SUM(platform_fee::numeric), 0) as platform_fee, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("TO_CHAR(created_at, 'YYYY-MM-DD')").
		Order("date ASC").
		Scan(&daily)

	type StatusRow struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	statusBreakdown := make([]StatusRow, 0)
	h.service.db.Model(&models.PaymentTransactionV3{}).
		Select("status, COUNT(*) as count").
		Where("created_at >= ?", since).
		Group("status").
		Scan(&statusBreakdown)

	var totalVolume, totalFees, totalNet float64
	h.service.db.Model(&models.PaymentTransactionV3{}).
		Where("created_at >= ?", since).
		Select("COALESCE(SUM(gross_amount::numeric), 0)").Scan(&totalVolume)
	h.service.db.Model(&models.PaymentTransactionV3{}).
		Where("created_at >= ?", since).
		Select("COALESCE(SUM(platform_fee::numeric), 0)").Scan(&totalFees)
	h.service.db.Model(&models.PaymentTransactionV3{}).
		Where("created_at >= ?", since).
		Select("COALESCE(SUM(net_amount::numeric), 0)").Scan(&totalNet)

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"daily":            daily,
			"status_breakdown": statusBreakdown,
			"summary": gin.H{
				"total_volume": totalVolume,
				"total_fees":   totalFees,
				"total_net":    totalNet,
			},
		},
	})
}

func (h *Handler) GetProposalPaymentDetails(c *gin.Context) {
	proposalID := c.Param("id")
	var proposal models.Proposal

	if err := h.service.db.Where("id = ?", proposalID).First(&proposal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proposal not found"})
		return
	}

	settings, err := h.service.GetSystemSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to load system settings"})
		return
	}

	fees := CalculateFees(proposal.BidAmount, settings)
	n := func(d decimal.Decimal) float64 { return d.InexactFloat64() }

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"proposal_id": proposal.ID,
			"currency":    PaymentCurrency,
			"bid_amount":  n(fees.BidAmount),
			"commission": gin.H{
				"original_amount":  n(fees.ClientCommission),
				"percentage":       n(fees.ClientCommissionPct),
				"discount_applied": 0,
				"final_amount":     n(fees.ClientCommission),
			},
			"app_fees": n(fees.ClientPlatformFee),
			"referral": gin.H{
				"code":       "",
				"percentage": 0,
				"saved":      0,
			},
			"grand_total":     n(fees.ClientTotal),
			"payment_summary": fees.Summary(),
		},
	})
}

func (h *Handler) GetJobPaymentDetails(c *gin.Context) {
	jobID := c.Param("id")

	var jobPost models.JobPost
	if err := h.service.db.Where("id = ?", jobID).First(&jobPost).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job post not found"})
		return
	}

	settings, err := h.service.GetSystemSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to load system settings"})
		return
	}

	fees := CalculateFees(jobPost.Budget, settings)
	n := func(d decimal.Decimal) float64 { return d.InexactFloat64() }

	budgetDescription := "Fixed budget"
	if jobPost.OpenBudget {
		budgetDescription = "Budget is flexible, final amount may vary"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"job_post_id": jobPost.ID,
			"currency":    PaymentCurrency,
			"open_budget": jobPost.OpenBudget,
			"budget": gin.H{
				"amount":      n(fees.BidAmount),
				"is_open":     jobPost.OpenBudget,
				"description": budgetDescription,
			},
			"commission": gin.H{
				"original_amount": n(fees.ClientCommission),
				"percentage":      n(fees.ClientCommissionPct),
				"final_amount":    n(fees.ClientCommission),
			},
			"app_fees": n(fees.ClientPlatformFee),
			"referral": gin.H{
				"code":       "",
				"percentage": 0,
			},
			"grand_total": n(fees.ClientTotal),
			"summary": gin.H{
				"budget":     n(fees.BidAmount),
				"commission": n(fees.ClientCommission),
				"app_fees":   n(fees.ClientPlatformFee),
				"total_fees": n(fees.ClientCommission.Add(fees.ClientPlatformFee)),
				"amount_due": n(fees.ClientTotal),
			},
			"payment_summary": fees.Summary(),
		},
	})
}
