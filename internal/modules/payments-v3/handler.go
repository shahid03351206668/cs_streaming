package paymentsv3

import (
	"net/http"
	"strconv"
	"tasksy/config"
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

	if err := h.service.AddBankAccount(&user, body.AccountHolderName, body.SortCode, body.AccountNumber); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
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
