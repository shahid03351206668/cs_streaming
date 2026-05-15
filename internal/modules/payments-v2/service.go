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
	AppFee               int64
	ClientCommission     float64
	FreelancerCommission float64
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
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

func (s *Service) handleChargeSucceeded(c *gin.Context, event *stripe.Event) error {
	var charge stripe.Charge

	tx := s.db.Begin()
	if tx.Error != nil {
		fmt.Println(tx.Error.Error())
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	settings, _ := s.GetSystemSettings()

	if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
		fmt.Println(err.Error())
		return err
	}

	proposalID := charge.Metadata["proposal_id"]

	if proposalID == "" {
		return fmt.Errorf("proposal id is not found in the meta data")
	}

	var contract models.Contract
	if err := s.db.Where("proposal_id = ?", proposalID).First(&contract).Error; err != nil {
		fmt.Println("error in proposal id querys")
		return err
	}

	amount := decimal.NewFromInt(charge.Amount / 100)
	appFees := settings.AppFee

	clientComission := settings.ClientCommission
	freelancerComission := settings.FreelancerCommission

	FreelancerComissionAmount := amount.Div(decimal.NewFromFloat(100)).Mul(decimal.NewFromFloat(settings.FreelancerCommission))
	clientComissionAmount := amount.Div(decimal.NewFromInt(100)).Mul(decimal.NewFromFloat(settings.ClientCommission))

	totalCharges := FreelancerComissionAmount.Add(clientComissionAmount).Add(decimal.NewFromInt(appFees))

	netAmount := amount.Sub(totalCharges)

	if charge.Status == "succeeded" {
		transaction := models.PaymentTransactionV2{
			PostingDate:       time.Now(),
			Amount:            amount,
			FromUserID:        contract.ClientID,
			ToUserID:          contract.FreelancerID,
			StripeEventID:     event.ID,
			AppFee:            decimal.NewFromInt(appFees),
			NetAmount:         netAmount,
			ClientCommPct:     clientComission,
			FreelancerCommPct: freelancerComission,
			Status:            models.PaymentStatusHeld,
		}

		s.ApplyDiscountOnTransaction(&transaction)

		if err := tx.Create(&transaction).Error; err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		escrow := models.EscrowTransaction{
			UserID:        contract.FreelancerID,
			Amount:        netAmount,
			TransactionID: transaction.ID,
			Status:        models.EscrowStatusHeld,
			ContractID:    contract.ID,
		}

		if err := tx.Create(&escrow).Error; err != nil {
			return fmt.Errorf("failed to create escrow record: %w", err)
		}

		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}
	return nil
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
