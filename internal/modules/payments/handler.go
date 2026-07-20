package payments

import (
	"fmt"
	"net/http"

	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func (s *PaymentHandler) GetPaymentGateways(c *gin.Context) {
	gateways := []string{"Stripe", "PayPal"}
	c.JSON(http.StatusOK, gin.H{
		"message":  "success",
		"gateways": gateways,
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

	toDollars := func(cents int64) float64 {
		return float64(cents) / 100.0
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
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
		},
	})
}

func parseIntParam(s string) (int, error) {
	v := 0
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
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

	toDollars := func(cents int64) float64 {
		return float64(cents) / 100.0
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
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
		},
	})
}
