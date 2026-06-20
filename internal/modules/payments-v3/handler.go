package paymentsv3

import (
	"net/http"
	"tasksy/config"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/refund"
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
