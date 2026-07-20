package paymentsv3

import (
	"net/http"
	"strconv"
	"tasksy/config"
	"tasksy/lib"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/refund"
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

	var body struct {
		ProposalID string  `json:"proposal_id" binding:"required"`
		Amount     float64 `json:"amount" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	var proposal models.Proposal
	if err := h.service.db.First(&proposal, "id = ?", body.ProposalID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "proposal not found"})
		return
	}

	var freelancer models.User
	if err := h.service.db.First(&freelancer, "id = ?", proposal.FreelancerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "freelancer not found"})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey
	intent, err := h.service.CreatePaymentIntent(&client, &freelancer, body.ProposalID, body.Amount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"client_secret":     intent.ClientSecret,
			"payment_intent_id": intent.ID,
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

	if err := h.service.ReleaseEscrow(escrowID, user.ID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
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

func (h *Handler) HandleRefund(c *gin.Context) {
	transactionID := c.Param("id")

	var transaction models.PaymentTransactionV3
	if err := h.service.db.First(&transaction, "id = ?", transactionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "payment transaction not found"})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey

	if _, err := refund.New(&stripe.RefundParams{
		PaymentIntent: stripe.String(transaction.StripePaymentIntentID),
	}); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	if err := h.service.db.Model(&transaction).Update("status", "refunded").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
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

	bidAmount := lib.Float(proposal.BidAmount)
	appFees := lib.Float(settings.AppFee)
	commissionPct := lib.Float(settings.ClientCommission)

	commissionAmount := lib.Float(0)
	if commissionPct > 0 {
		commissionAmount = bidAmount / 100.0 * commissionPct
	}

	discountAmount := lib.Float(0)
	referralCode := ""
	// var discountPct float64
	// var userReferral models.ReferralUsage
	// if err := h.service.db.Preload("ReferralCode").
	// 	Where("referee_id = ? AND is_qualified = ?", user.ID, false).
	// 	First(&userReferral).Error; err == nil {

	// 	referralCode = userReferral.ReferralCode.Code
	// 	discountPct = float64(userReferral.ReferralCode.DiscountPercentage)

	// 	if discountPct > 0 {
	// 		discountAmount = int64(float64(commissionAmount) * discountPct / 100.0)
	// 	}
	// 	if discountAmount > commissionAmount {
	// 		discountAmount = commissionAmount
	// 	}
	// }

	finalCommission := commissionAmount - discountAmount
	grandTotal := bidAmount + finalCommission + appFees

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"proposal_id": proposal.ID,
			"currency":    "usd",
			"bid_amount":  bidAmount,
			"commission": gin.H{
				"original_amount":  commissionAmount,
				"percentage":       commissionPct,
				"discount_applied": discountAmount,
				"final_amount":     finalCommission,
				
			},
			"app_fees": appFees,
			"referral": gin.H{
				"code":       referralCode,
				"percentage": 0,
				"saved":      discountAmount,
			},
			"grand_total": grandTotal,
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

	settings, _ := h.service.GetSystemSettings()

	budgetAmount := lib.Float(jobPost.Budget)
	appFees := lib.Float(settings.AppFee)
	commissionPct := lib.Float(settings.ClientCommission)

	commissionAmount := 0.00
	if commissionPct > 0 {
		commissionAmount = budgetAmount / 100.0 * commissionPct
	}

	referralCode := ""
	var discountPct float64

	// discountAmount := int64(0)
	// var userReferral models.ReferralUsage
	// if err := h.service.db.Preload("ReferralCode").
	// 	Where("referee_id = ? AND is_qualified = ?", user.ID, false).
	// 	First(&userReferral).Error; err == nil {

	// 	referralCode = userReferral.ReferralCode.Code
	// 	discountPct = float64(userReferral.ReferralCode.DiscountPercentage)

	// 	if discountPct > 0 {
	// 		discountAmount = int64(float64(commissionAmount) * discountPct / 100.0)
	// 	}
	// 	if discountAmount > commissionAmount {
	// 		discountAmount = commissionAmount
	// 	}
	// }

	finalCommission := commissionAmount
	grandTotal := budgetAmount + finalCommission + appFees

	budgetDescription := "Fixed budget"
	if jobPost.OpenBudget {
		budgetDescription = "Budget is flexible, final amount may vary"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"job_post_id": jobPost.ID,
			"currency":    "usd",
			"open_budget": jobPost.OpenBudget,
			"budget": gin.H{
				"amount":      budgetAmount,
				"is_open":     jobPost.OpenBudget,
				"description": budgetDescription,
			},
			"commission": gin.H{
				"original_amount": commissionAmount,
				"percentage":      commissionPct,
				// "discount_applied": toDollars(discountAmount),
				"final_amount": finalCommission,
			},
			"app_fees": appFees,
			"referral": gin.H{
				"code":       referralCode,
				"percentage": discountPct,
				// "saved":      toDollars(discountAmount),
			},
			"grand_total": grandTotal,
			"summary": gin.H{
				"budget":     budgetAmount,
				"commission": finalCommission,
				"app_fees":   appFees,
				"total_fees": finalCommission + appFees,
				// "discount_saved": toDollars(discountAmount),
				"amount_due": grandTotal,
			},
		},
	})
}

func (h *Handler) ProposalPaymentDetail(c *gin.Context) {
	proposalID := c.Param("id")
	var proposal models.Proposal

	if err := h.service.db.Where("id = ?", proposalID).First(&proposal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proposal not found"})
		return
	}

	settings, _ := h.service.GetSystemSettings()

	bidAmount := lib.Float(proposal.BidAmount)
	appFees := lib.Float(settings.AppFee)
	commissionPct := lib.Float(settings.ClientCommission)

	commissionAmount := lib.Float(0)
	if commissionPct > 0 {
		commissionAmount = bidAmount / 100.0 * commissionPct
	}

	discountAmount := lib.Float(0)
	referralCode := ""

	// var discountPct float64
	// var userReferral models.ReferralUsage
	// if err := h.service.db.Preload("ReferralCode").
	// 	Where("referee_id = ? AND is_qualified = ?", user.ID, false).
	// 	First(&userReferral).Error; err == nil {

	// 	referralCode = userReferral.ReferralCode.Code
	// 	discountPct = float64(userReferral.ReferralCode.DiscountPercentage)

	// 	if discountPct > 0 {
	// 		discountAmount = int64(float64(commissionAmount) * discountPct / 100.0)
	// 	}
	// 	if discountAmount > commissionAmount {
	// 		discountAmount = commissionAmount
	// 	}
	// }

	finalCommission := commissionAmount - discountAmount
	grandTotal := bidAmount + finalCommission + appFees

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"proposal_id": proposal.ID,
			"currency":    "usd",
			"bid_amount":  bidAmount,
			"commission": gin.H{
				"original_amount":  commissionAmount,
				"percentage":       commissionPct,
				"discount_applied": discountAmount,
				"final_amount":     finalCommission,
			},
			"app_fees": appFees,
			"referral": gin.H{
				"code":       referralCode,
				"percentage": 0,
				"saved":      discountAmount,
			},
			"grand_total": grandTotal,
		},
	})
}
