package paymentsv3

import (
	"net/http"
	"tasksy/config"
	"tasksy/models"

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
