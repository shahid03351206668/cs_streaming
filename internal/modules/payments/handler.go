package payments

import (
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

// GetPaymentGateways returns available payment gateways
func (s *PaymentHandler) GetPaymentGateways(c *gin.Context) {
	gateways := []string{"Stripe", "PayPal"} // Add more as needed
	c.JSON(http.StatusOK, gin.H{
		"message":  "success",
		"gateways": gateways,
	})
}

func (s *PaymentHandler) GetPaymentTransactions(c *gin.Context) {
	var params TransactionListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Invalid query parameters: " + err.Error(),
		})
		return
	}

	transactions, total, err := s.service.GetPaymentTransactions(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch transactions: " + err.Error(),
		})
		return
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transactions,
		"meta": gin.H{
			"total":       total,
			"page":        params.Page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

func (s *PaymentHandler) GetPaymentTransactionByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Transaction ID is required",
		})
		return
	}

	transaction, err := s.service.GetPaymentTransactionByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "error",
			"error":   "Transaction not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transaction,
	})
}

func (s *PaymentHandler) GetUserPaymentTransactions(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	userID := user.ID

	var params TransactionListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Invalid query parameters: " + err.Error(),
		})
		return
	}

	params.UserID = userID
	transactions, total, err := s.service.GetPaymentTransactions(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch transactions: " + err.Error(),
		})
		return
	}
	if params.Limit == 0 {
		params.Limit = 10
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transactions,
		"meta": gin.H{
			"total":       total,
			"page":        params.Page,
			"limit":       params.Limit,
			"total_pages": (total + int64(params.Limit) - 1) / int64(params.Limit),
		},
	})
}

func (s *PaymentHandler) GetProposalPaymentDetails(c *gin.Context) {
	proposalID := c.Param("id")
	user := c.MustGet("user").(models.User)

	var proposal models.Proposal
	if err := s.service.db.Where("id = ?", proposalID).First(&proposal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proposal not found"})
		return
	}

	settings, _ := GetSystemSettings()

	bidAmount := int64(proposal.BidAmount * 100)
	appFees := int64(settings.ApplicationFeeAmount)
	commissionPct := float64(settings.ClientCommissionPercentage)
	commissionAmount := int64(0)

	if commissionPct > 0 {
		commissionAmount = int64(float64(bidAmount) * commissionPct / 100.0)
	}

	discountAmount := int64(0)
	referralCode := ""

	var discountPct float64 = 0
	var userReferral models.ReferralUsage

	if err := s.service.db.Preload("ReferralCode").
		Where("referee_id = ? AND is_qualified = ?", user.ID, false).
		First(&userReferral).Error; err == nil {

		referralCode = userReferral.ReferralCode.Code
		discountPct = float64(userReferral.ReferralCode.DiscountPercentage)

		if discountPct > 0 {
			discountAmount = int64(float64(commissionAmount) * discountPct / 100.0)
		}

		if discountAmount > commissionAmount {
			discountAmount = commissionAmount
		}
	}

	finalCommission := commissionAmount - discountAmount
	grandTotal := bidAmount + finalCommission + appFees

	// Helper to convert cents to dollars
	toDollars := func(cents int64) float64 {
		return float64(cents) / 100.0
	}

	response := gin.H{
		"proposal_id": proposal.ID,
		"currency":    "gbp",
		"bid_amount":  toDollars(bidAmount),
		"commission": gin.H{
			"original_amount":  toDollars(commissionAmount),
			"percentage":       commissionPct,
			"discount_applied": toDollars(discountAmount),
			"final_amount":     toDollars(finalCommission),
		},
		"app_fees": toDollars(appFees),
		"referral": gin.H{
			"code":       referralCode,
			"percentage": discountPct,
			"saved":      toDollars(discountAmount),
		},
		"grand_total": toDollars(grandTotal),
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    response,
	})
}

func (s *PaymentHandler) GetJobPostPaymentDetails(c *gin.Context) {
	jobPostID := c.Param("id")
	user := c.MustGet("user").(models.User)

	var jobPost models.JobPost
	if err := s.service.db.Where("id = ?", jobPostID).First(&jobPost).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job post not found"})
		return
	}

	settings, _ := GetSystemSettings()

	// Base amount from job budget
	budgetAmount := int64(jobPost.Budget * 100)
	appFees := int64(settings.ApplicationFeeAmount)
	commissionPct := float64(settings.FreelancerCommissionPercentage)
	commissionAmount := int64(0)

	if commissionPct > 0 {
		commissionAmount = int64(float64(budgetAmount) * commissionPct / 100.0)
	}

	discountAmount := int64(0)
	referralCode := ""
	var discountPct float64 = 0
	var userReferral models.ReferralUsage

	if err := s.service.db.Preload("ReferralCode").
		Where("referee_id = ? AND is_qualified = ?", user.ID, false).
		First(&userReferral).Error; err == nil {

		referralCode = userReferral.ReferralCode.Code
		discountPct = float64(userReferral.ReferralCode.DiscountPercentage)

		if discountPct > 0 {
			discountAmount = int64(float64(commissionAmount) * discountPct / 100.0)
		}

		if discountAmount > commissionAmount {
			discountAmount = commissionAmount
		}
	}

	finalCommission := commissionAmount - discountAmount
	grandTotal := budgetAmount + finalCommission + appFees

	// Helper to convert cents to dollars
	toDollars := func(cents int64) float64 {
		return float64(cents) / 100.0
	}

	response := gin.H{
		"job_post_id": jobPost.ID,
		"currency":    "gbp",
		"open_budget": jobPost.OpenBudget,
		"budget": gin.H{
			"amount":  toDollars(budgetAmount),
			"is_open": jobPost.OpenBudget,
			"description": func() string {
				if jobPost.OpenBudget {
					return "Budget is flexible, final amount may vary"
				}
				return "Fixed budget"
			}(),
		},
		"commission": gin.H{
			"original_amount":  toDollars(commissionAmount),
			"percentage":       commissionPct,
			"discount_applied": toDollars(discountAmount),
			"final_amount":     toDollars(finalCommission),
		},
		"app_fees": toDollars(appFees),
		"referral": gin.H{
			"code":       referralCode,
			"percentage": discountPct,
			"saved":      toDollars(discountAmount),
		},
		"grand_total": toDollars(grandTotal),
		"summary": gin.H{
			"budget":         toDollars(budgetAmount),
			"commission":     toDollars(finalCommission),
			"app_fees":       toDollars(appFees),
			"total_fees":     toDollars(finalCommission + appFees),
			"discount_saved": toDollars(discountAmount),
			"amount_due":     toDollars(grandTotal),
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    response,
	})
}
