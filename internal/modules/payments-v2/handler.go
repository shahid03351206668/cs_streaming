package paymentsv2

import (
	"fmt"
	"io"
	"net/http"
	"tasksy/config"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84/webhook"
	// "golang.org/x/text/cases"
	// "gorm.io/gorm"
)

type Handler struct {
	config  *config.Config
	service *Service
}

func NewHandler(config *config.Config, service *Service) *Handler {
	return &Handler{
		config:  config,
		service: service,
	}
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
			"balance":      wallet.Balance,
			"transactions": wallet.Transactions,
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
			fmt.Println(err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "succeed"})
	default:
		c.JSON(http.StatusContinue, gin.H{"message": "received", "info": fmt.Sprintf("unhandled event type %s", event.Type)})
	}
}

func (h *Handler) AddUserPaymentAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	type RequestData struct {
		AccountNo   string `json:"account_no"`
		RoutingNo   string `json:"routing_no"`
		Currency    string `jsom:"currency"`
		CountryCode string `jsom:"country_code"`
	}

	var data RequestData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.AddUserBankAccount(&user, data.AccountNo, data.RoutingNo, data.Currency, data.CountryCode); err != nil {
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

func (h *Handler) GetUserAccount(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	var data models.UserAccountDetails

	if err := h.service.db.Where("user_id  = ? ", user.ID).Find(&data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": data})
}
