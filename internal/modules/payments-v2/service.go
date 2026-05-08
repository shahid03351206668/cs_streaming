package paymentsv2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"tasksy/models"
	// "tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v84"

	// "go.uber.org/zap"
	"gorm.io/gorm"
)

type SystemSetting struct {
	AppFee               decimal.Decimal
	ClientCommission     float64
	FreelancerCommission float64
}

type Service struct {
	db *gorm.DB
}

func (s *Service) GetSystemSettings() (*SystemSetting, error) {
	var data models.SystemSettings
	if err := s.db.First(&data).Error; err != nil {
		return nil, err
	}

	return &SystemSetting{
		AppFee:               data.ApplicationFeeAmount,
		FreelancerCommission: data.FreelancerCommissionPercentage,
		ClientCommission:     data.ClientCommissionPercentage,
	}, nil
}

type GLEntry struct {
	UserID string      `gorm:"not null;index" json:"user_id"`
	User   models.User `gorm:"foreignKey:UserID" json:"user,omitempty"`

	PostingDate *time.Time      `gorm:"not null;index" json:"posting_date"`
	Credit      decimal.Decimal `gorm:"not null" json:"credit"`
	Debit       decimal.Decimal `gorm:"not null" json:"debit"`

	ReferenceType string `json:"reference_type"`
	ReferenceID   string `json:"reference_id"`
}

func (s *Service) handleChargeSucceeded(c *gin.Context, event *stripe.Event) {
	var charge stripe.Charge

	tx := s.db.Begin()
	if tx.Error != nil {
		fmt.Println(tx.Error.Error())
		// fmt.Printf("failed to begin transaction: %w", tx.Error)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			// logger.Log.Log(fmt.Sprintf("error on handle stripe succeed transaction %v\n", r))
			tx.Rollback()
		}
	}()

	settings, _ := s.GetSystemSettings()

	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		fmt.Println(err.Error())
		// logger.Log.Error(err.Error(), zap.String("stripe handle charge succeed event"))
		return
	}

	proposalID := charge.Metadata["proposal_id"]

	if proposalID != "" {
		return
	}

	var contract models.Contract

	if err := s.db.Where("proposal_id = ?", proposalID).First(&contract).Error; err != nil {
		return
	}

	appFees := settings.AppFee
	clientComission := settings.ClientCommission
	freelancerComission := settings.FreelancerCommission

	amount := decimal.NewFromInt(charge.Amount / 100)

	if charge.Status == "succeeded" {
		transaction := models.PaymentTransactionV2{
			Amount:            amount,
			FromUserID:        contract.ClientID,
			ToUserID:          contract.FreelancerID,
			StripeEventID:     event.ID,
			AppFee:            decimal.Decimal(appFees),
			ClientCommPct:     clientComission,
			FreelancerCommPct: freelancerComission,
			Status:            models.PaymentStatusHeld,
		}

		s.ApplyDiscountOnTransaction(&transaction)
	}
}

func (s *Service) ApplyDiscountOnTransaction(transaction *models.PaymentTransactionV2) {
}

func (s *Service) GetUserWallet(user *models.User) {
	// query all user user trnasactions and show him thier balance including escrow payments
}

func (s *Service) GetUserTransactions(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	type Params struct {
		FromDate *time.Time `form:"from_date"`
		ToDate   *time.Time `form:"to_date"`
	}

	var args Params

	if err := c.BindQuery(&args); err != nil {
		fmt.Println(err.Error())
	}

	if args.FromDate == nil || args.ToDate == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "from_date and to_date are required",
		})
		return
	}

	type TransactionValue struct {
		JobTitle string
		Status   string          `json:"status"`
		Amount   decimal.Decimal `json:"amount"`
		Type     string          `json:"type"`
	}

	var data []models.PaymentTransactionV2

	if err := s.db.Preload("Contract.JobPost").Where("from_user_id = ? ", user.ID).
		Or("to_user_id = ?", user.ID).
		Where("posting_date BETWEEN ? AND ?", args.FromDate, args.ToDate).
		Find(&data).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	transactions := make([]TransactionValue, 0)

	for _, row := range data {
		var TransactionType string
		Amount := row.Amount

		if row.FromUserID == user.ID {
			TransactionType = "pay"
			Amount = Amount.Mul(decimal.NewFromInt(-1))
		} else {
			TransactionType = "receive"
		}

		transactions = append(transactions, TransactionValue{
			JobTitle: row.Contract.JobPost.Title,
			Amount:   Amount,
			Type:     TransactionType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})

}
