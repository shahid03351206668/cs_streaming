package payments

import (
	"net/http"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)
	

type PayoutService struct {
	db *gorm.DB
}

func NewPayoutService(db *gorm.DB) *PayoutService {
	return &PayoutService{db: db}
}

// RequestPayout handles POST /api/v1/payouts/request
func (s *PayoutService) RequestPayout(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	var req struct {
		Amount   int64  `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	// Validate balance
	if user.ReferralRewardBalance < req.Amount || req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Insufficient balance or invalid amount"})
		return
	}
	// TODO: Initiate Stripe payout here
	// On success, create payout transaction
	payout := models.PayoutTransaction{
		TransactionDate: time.Now(),
		UserID:          user.ID,
		Amount:          req.Amount,
		NetAmount:       req.Amount, // update if fees
		AppFeeAmount:    0,
		Currency:        req.Currency,
		Status:          models.PaymentStatusPending,
		StripeID:        "stripe_payout_id", // update after Stripe call
	}
	if err := s.db.Create(&payout).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create payout transaction"})
		return
	}
	// Deduct balance
	if err := s.db.Model(&models.User{}).Where("id = ?", user.ID).UpdateColumn("referral_reward_balance", gorm.Expr("referral_reward_balance - ?", req.Amount)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update balance"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Payout requested", "data": payout})
}

// GetPayoutHistory handles GET /api/v1/payouts/history
func (s *PayoutService) GetPayoutHistory(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	var payouts []models.PayoutTransaction
	if err := s.db.Where("user_id = ?", user.ID).Order("transaction_date DESC").Find(&payouts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch payout history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": payouts})
}
