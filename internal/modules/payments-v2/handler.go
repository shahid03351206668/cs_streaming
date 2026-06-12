package paymentsv2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"tasksy/config"
	emailpkg "tasksy/internal/modules/email"
	"tasksy/lib"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
	// "golang.org/x/text/cases"
	// "gorm.io/gorm"
)

type Handler struct {
	config   *config.Config
	service  *Service
	emailSvc *emailpkg.Service
}

func NewHandler(config *config.Config, service *Service) *Handler {
	return &Handler{
		config:  config,
		service: service,
	}
}

func (h *Handler) SetEmailService(svc *emailpkg.Service) {
	h.emailSvc = svc
}

func (h *Handler) HandleUserWallet(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	wallet, err := h.service.GetUserWallet(&user, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": map[string]any{
			"balance":       wallet.Balance,
			"transactions":  wallet.Transactions,
			"escrow_amount": wallet.EscrowAmount,
		},
	})
}

func (h *Handler) HandleCreatePayout(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	type RequestData struct {
		Amount    float64 `json:"amount"`
		AccountID string  `json:"account_id"`
	}

	var userAccount models.UserAccountDetails
	var data RequestData

	if data.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Request amount cannot be negative",
		})
		return
	}

	h.service.db.Where("id = ?", data.AccountID).Find(&userAccount)
	if err := h.service.CreatePayout(&user, data.Amount, &userAccount); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
	})
}

func (h *Handler) HandleStripeWebhook(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(65536))
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error"})
	}

	stripeConfig := h.config.Stripe
	webhookSecret := stripeConfig.WebhookSecret
	signature := c.GetHeader("Stripe-Signature")

	event, err := webhook.ConstructEvent(payload, signature, webhookSecret)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	switch event.Type {
	case "charge.succeeded":
		err := h.service.handleChargeSucceeded(&event)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "succeed"})

	case "payout.paid":
		h.handlePayoutPaid(c, event)
	case "payout.failed":
		h.handlePayoutFailed(c, event)
	case "payout.canceled":
		h.handlePayoutCanceled(c, event)

	default:
		c.JSON(http.StatusContinue, gin.H{"message": "received", "info": fmt.Sprintf("unhandled event type %s", event.Type)})
	}
}

func (h *Handler) handlePayoutPaid(c *gin.Context, event stripe.Event) {
	var stripePayout stripe.Payout
	if err := json.Unmarshal(event.Data.Raw, &stripePayout); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse payout"})
		return
	}

	if err := h.service.UpdatePayoutStatus(stripePayout.ID, models.PayoutCompleted); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.emailSvc != nil {
		var payoutTx models.PayoutTransaction
		if err := h.service.db.Preload("User").Where("stripe_payout_id = ?", stripePayout.ID).First(&payoutTx).Error; err == nil && payoutTx.User.Email != "" {
			_ = h.emailSvc.SendTemplatedEmail("payout_completed", payoutTx.User.Email, map[string]string{
				"first_name": payoutTx.User.FirstName,
				"amount":     payoutTx.Amount.String(),
				"currency":   payoutTx.Currency,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) handlePayoutFailed(c *gin.Context, event stripe.Event) {
	var stripePayout stripe.Payout
	if err := json.Unmarshal(event.Data.Raw, &stripePayout); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse payout"})
		return
	}

	failureReason := ""
	if stripePayout.FailureMessage != "" {
		failureReason = stripePayout.FailureMessage
	}

	if err := h.service.UpdatePayoutStatusWithReason(stripePayout.ID, models.PayoutFailed, failureReason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.emailSvc != nil {
		var payoutTx models.PayoutTransaction
		if err := h.service.db.Preload("User").Where("stripe_payout_id = ?", stripePayout.ID).First(&payoutTx).Error; err == nil && payoutTx.User.Email != "" {
			_ = h.emailSvc.SendTemplatedEmail("payout_failed", payoutTx.User.Email, map[string]string{
				"first_name":     payoutTx.User.FirstName,
				"amount":         payoutTx.Amount.String(),
				"currency":       payoutTx.Currency,
				"failure_reason": failureReason,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) handlePayoutCanceled(c *gin.Context, event stripe.Event) {
	var stripePayout stripe.Payout
	if err := json.Unmarshal(event.Data.Raw, &stripePayout); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse payout"})
		return
	}

	if err := h.service.UpdatePayoutStatus(stripePayout.ID, models.PayoutCanceled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) AddUserPaymentAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	type RequestData struct {
		AccountNo     string `json:"account_no"`
		RoutingNo     string `json:"routing_no"`
		Currency      string `json:"currency"`
		CountryCode   string `json:"country_code"`
		AccountHolder string `json:"account_holder_name"`
		BankName      string `json:"bank_name"`
	}

	var data RequestData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.AddUserBankAccount(&user, data.AccountNo, data.RoutingNo, data.Currency, data.CountryCode, data.AccountHolder, data.BankName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
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

func (h *Handler) GetJobPostPaymentDetails(c *gin.Context) {
	jobPostID := c.Param("id")
	// user := c.MustGet("user").(models.User)

	var jobPost models.JobPost
	if err := h.service.db.Where("id = ?", jobPostID).First(&jobPost).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job post not found"})
		return
	}

	settings, err := h.service.GetSystemSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to load system settings"})
		return
	}

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

	// toDollars := func(cents int64) float64 {
	// 	return float64(cents) / 100.0
	// }

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

func (h *Handler) GetUserAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	var data []models.UserAccountDetails

	if err := h.service.db.Where("user_id  = ? AND enabled = true", user.ID).Find(&data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "error": err.Error()})
		return
	}

	if data == nil {
		c.JSON(http.StatusOK, gin.H{"message": "success", "data": make([]any, 0)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": data})
}

func (h *Handler) HandleStripePayoutHook(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}
