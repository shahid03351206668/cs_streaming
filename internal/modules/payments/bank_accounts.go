package payments

import (
	"net/http"
	"strings"
	"time"

	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	stripeaccount "github.com/stripe/stripe-go/v84/account"
	stripebankaccount "github.com/stripe/stripe-go/v84/bankaccount"
)

// AddBankAccount handles POST /api/v1/wallet/bank-accounts
// Stores the user's UK bank account details and registers it with Stripe Connect.
func (s *PayoutService) AddBankAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	stripe.Key = s.config.SecretKey

	var body struct {
		AccountHolderName string `json:"account_holder_name" binding:"required"`
		SortCode          string `json:"sort_code" binding:"required"` // e.g. "108800" or "10-88-00"
		AccountNumber     string `json:"account_number" binding:"required"`
		Currency          string `json:"currency"`
		SetAsDefault      bool   `json:"set_as_default"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}
	if body.Currency == "" {
		body.Currency = "gbp"
	}

	// Normalise sort code — strip dashes/spaces
	sortCode := strings.NewReplacer("-", "", " ", "").Replace(body.SortCode)
	if len(sortCode) != 6 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "sort code must be 6 digits (e.g. 108800 or 10-88-00)"})
		return
	}

	// Re-fetch user for current connect account ID
	var freshUser models.User
	if err := s.db.Select("id, email, stripe_connect_account_id").
		First(&freshUser, "id = ?", user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch user"})
		return
	}

	connectAccountID := freshUser.StripeConnectAccountID

	// Create a Stripe Connect Custom account if the user doesn't have one yet
	if connectAccountID == "" {
		acct, err := createConnectAccount(freshUser.Email, c.ClientIP())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "error",
				"error":   "failed to create payout account: " + err.Error(),
			})
			return
		}
		connectAccountID = acct.ID
		if err := s.db.Model(&models.User{}).Where("id = ?", user.ID).
			UpdateColumn("stripe_connect_account_id", connectAccountID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to save connect account"})
			return
		}
	}

	// Add bank account directly to the Connect account via Stripe API
	baParams := &stripe.BankAccountParams{
		Account:           stripe.String(connectAccountID),
		AccountHolderName: stripe.String(body.AccountHolderName),
		AccountHolderType: stripe.String("individual"),
		Country:           stripe.String("GB"),
		Currency:          stripe.String(body.Currency),
		RoutingNumber:     stripe.String(sortCode),
		AccountNumber:     stripe.String(body.AccountNumber),
	}
	stripeBa, err := stripebankaccount.New(baParams)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "invalid bank account details: " + err.Error()})
		return
	}

	// Format sort code for display: 108800 → 10-88-00
	displaySortCode := sortCode[:2] + "-" + sortCode[2:4] + "-" + sortCode[4:]

	// Count existing accounts — first one is default automatically
	var count int64
	s.db.Model(&models.UserBankAccount{}).Where("user_id = ?", user.ID).Count(&count)
	isDefault := body.SetAsDefault || count == 0

	// If setting as default, clear existing defaults
	if isDefault {
		s.db.Model(&models.UserBankAccount{}).Where("user_id = ?", user.ID).
			UpdateColumn("is_default", false)
	}

	ba := models.UserBankAccount{
		UserID:                 user.ID,
		StripeConnectAccountID: connectAccountID,
		StripeBankAccountID:    stripeBa.ID,
		AccountHolderName:      body.AccountHolderName,
		SortCode:               displaySortCode,
		AccountNumberLast4:     stripeBa.Last4,
		BankName:               stripeBa.BankName,
		Currency:               body.Currency,
		IsDefault:              isDefault,
	}
	if err := s.db.Create(&ba).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to save bank account"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "success", "data": ba})
}

// ListBankAccounts handles GET /api/v1/wallet/bank-accounts
func (s *PayoutService) ListBankAccounts(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var accounts []models.UserBankAccount
	if err := s.db.Where("user_id = ?", user.ID).
		Order("is_default DESC, created_at DESC").
		Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to fetch bank accounts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": accounts})
}

// SetDefaultBankAccount handles PUT /api/v1/wallet/bank-accounts/:id/default
func (s *PayoutService) SetDefaultBankAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	baID := c.Param("id")

	var ba models.UserBankAccount
	if err := s.db.Where("id = ? AND user_id = ?", baID, user.ID).First(&ba).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "bank account not found"})
		return
	}

	s.db.Model(&models.UserBankAccount{}).Where("user_id = ?", user.ID).
		UpdateColumn("is_default", false)
	s.db.Model(&ba).UpdateColumn("is_default", true)

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// DeleteBankAccount handles DELETE /api/v1/wallet/bank-accounts/:id
func (s *PayoutService) DeleteBankAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	baID := c.Param("id")
	stripe.Key = s.config.SecretKey

	var ba models.UserBankAccount
	if err := s.db.Where("id = ? AND user_id = ?", baID, user.ID).First(&ba).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "bank account not found"})
		return
	}

	// Remove from Stripe (best-effort)
	delParams := &stripe.BankAccountParams{
		Account: stripe.String(ba.StripeConnectAccountID),
	}
	stripebankaccount.Del(ba.StripeBankAccountID, delParams) //nolint:errcheck

	if err := s.db.Delete(&ba).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "failed to delete bank account"})
		return
	}

	// If this was the default, promote the next account
	if ba.IsDefault {
		var next models.UserBankAccount
		if err := s.db.Where("user_id = ?", user.ID).Order("created_at DESC").First(&next).Error; err == nil {
			s.db.Model(&next).UpdateColumn("is_default", true)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// createConnectAccount creates a Stripe Custom Connect account for a user.
// Uses the "recipient" service agreement which only requires payout capabilities.
func createConnectAccount(email, clientIP string) (*stripe.Account, error) {
	params := &stripe.AccountParams{
		Type:         stripe.String("custom"),
		Country:      stripe.String("GB"),
		Email:        stripe.String(email),
		BusinessType: stripe.String("individual"),
		Capabilities: &stripe.AccountCapabilitiesParams{
			Transfers: &stripe.AccountCapabilitiesTransfersParams{
				Requested: stripe.Bool(true),
			},
		},
		TOSAcceptance: &stripe.AccountTOSAcceptanceParams{
			ServiceAgreement: stripe.String("recipient"),
			Date:             stripe.Int64(time.Now().Unix()),
			IP:               stripe.String(clientIP),
		},
	}
	return stripeaccount.New(params)
}
