package payments

import (
	"net/http"
	"strings"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

// InitiateEscrowDeposit handles POST /api/v1/escrow/contracts/:id/deposit
// The client calls this after a proposal is accepted to lock funds into escrow.
func (s *PaymentHandler) InitiateEscrowDeposit(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	contractID := c.Param("id")

	if contractID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "contract ID is required"})
		return
	}

	clientSecret, amount, err := s.service.InitiateEscrow(contractID, user.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	// Extract PI ID from client_secret (format: pi_xxx_secret_yyy)
	paymentIntentID := ""
	if parts := strings.SplitN(clientSecret, "_secret_", 2); len(parts) == 2 {
		paymentIntentID = parts[0]
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "success",
		"client_secret":     clientSecret,
		"payment_intent_id": paymentIntentID,
		"amount":            amount,
		"currency":          "gbp",
		"note":              "Use the client_secret to confirm payment on the client side. Funds will be held in escrow until job completion.",
	})
}

// GetEscrowStatus handles GET /api/v1/escrow/contracts/:id/status
func (s *PaymentHandler) GetEscrowStatus(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	contractID := c.Param("id")

	if contractID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "contract ID is required"})
		return
	}

	// Verify the user is a party to this contract
	var contract models.Contract
	if err := s.service.db.First(&contract, "id = ?", contractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "contract not found"})
		return
	}

	if contract.ClientID != user.ID && contract.FreelancerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": "you are not a party to this contract"})
		return
	}

	status, err := s.service.GetEscrowStatus(contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    status,
	})
}

// RefundEscrow handles POST /api/v1/escrow/contracts/:id/refund (admin or system use)
func (s *PaymentHandler) RefundEscrow(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	contractID := c.Param("id")

	if contractID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "contract ID is required"})
		return
	}

	var contract models.Contract
	if err := s.service.db.First(&contract, "id = ?", contractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "contract not found"})
		return
	}

	// Only client can request refund, and only if contract is cancelled/disputed
	if contract.ClientID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": "only the client can request a refund"})
		return
	}

	if contract.Status != models.ContractStatusCancelled && contract.Status != models.ContractStatusDisputed {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "refund is only available for cancelled or disputed contracts",
		})
		return
	}

	if err := s.service.RefundEscrow(contractID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"note":    "Escrow funds have been refunded to the client.",
	})
}

// GetPaymentSummaryWithPromotion handles GET /api/v1/payments/contracts/:id/summary
// Returns full breakdown including promotional and referral discounts.
func (s *PaymentHandler) GetPaymentSummaryWithPromotion(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	contractID := c.Param("id")

	var contract models.Contract
	if err := s.service.db.First(&contract, "id = ?", contractID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "contract not found"})
		return
	}

	if contract.ClientID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": "only the client can view payment summary"})
		return
	}

	settings, _ := GetSystemSettings()
	amountInCents := int64(contract.TotalAmount * 100)
	appFee := int64(settings.ApplicationFeeAmount)

	promoDiscount := s.service.GetPromotionalDiscount(user.ID, amountInCents)
	afterPromo := amountInCents - promoDiscount

	refDiscount, _, _ := s.service.CalculateReferralDiscount(user.ID, afterPromo)
	finalAmount := afterPromo - refDiscount

	toDollars := func(cents int64) float64 { return float64(cents) / 100.0 }

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"contract_id":         contractID,
			"contract_amount":     toDollars(amountInCents),
			"promotional_discount": toDollars(promoDiscount),
			"referral_discount":   toDollars(refDiscount),
			"app_fee":             toDollars(appFee),
			"total_due":           toDollars(finalAmount + appFee),
			"escrow_amount":       toDollars(finalAmount),
			"currency":            "gbp",
			"escrow_status":       contract.EscrowStatus,
		},
	})
}
