package payments

import (
	"tasksy/config"
	"tasksy/models"

	"gorm.io/gorm"
)

type PaymentService struct {
	config *config.StripeConfig
	db     *gorm.DB
}

func NewService(config *config.StripeConfig, db *gorm.DB) *PaymentService {
	return &PaymentService{
		config: config,
		db:     db,
	}
}

type TransactionUser struct {
	ID           string `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Email        string `json:"email"`
	ProfilePhoto string `json:"profile_photo"`
}

type PaymentTransactionResponse struct {
	ID              string          `json:"id"`
	TransactionDate string          `json:"transaction_date"`
	FromUser        TransactionUser `json:"from_user"`
	ToUser          TransactionUser `json:"to_user"`
	ReferenceType   string          `json:"reference_type"`
	ReferenceID     string          `json:"reference_id"`
	Amount          int64           `json:"amount"`
	AppFeeAmount    int64           `json:"app_fee_amount"`
	DiscountAmount  int64           `json:"discount_amount"`
	NetAmount       int64           `json:"net_amount"`
	Currency        string          `json:"currency"`
	Status          string          `json:"status"`
	ReferralCodeID  *string         `json:"referral_code_id,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

type TransactionListParams struct {
	Page     int    `form:"page"`
	Limit    int    `form:"limit"`
	Status   string `form:"status"`
	UserID   string `form:"user_id"`
	FromDate string `form:"from_date"`
	ToDate   string `form:"to_date"`
	Search   string `form:"search"`
}

func (s *PaymentService) GetPaymentTransactions(params TransactionListParams) ([]PaymentTransactionResponse, int64, error) {
	var transactions []models.PaymentTransaction
	var total int64

	query := s.db.Model(&models.PaymentTransaction{})

	// Apply filters
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}

	if params.UserID != "" {
		query = query.Where("from_user_id = ? OR to_user_id = ?", params.UserID, params.UserID)
	}

	if params.FromDate != "" {
		query = query.Where("transaction_date >= ?", params.FromDate)
	}

	if params.ToDate != "" {
		query = query.Where("transaction_date <= ?", params.ToDate)
	}

	if params.Search != "" {
		searchPattern := "%" + params.Search + "%"
		query = query.Where("payment_intent_id ILIKE ? OR charge_id ILIKE ? OR reference_id ILIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}

	offset := (params.Page - 1) * params.Limit

	if err := query.
		Preload("FromUser").
		Preload("ToUser").
		Order("transaction_date DESC").
		Limit(params.Limit).
		Offset(offset).
		Find(&transactions).Error; err != nil {
		return nil, 0, err
	}

	// Transform to response format
	result := make([]PaymentTransactionResponse, 0, len(transactions))
	for _, txn := range transactions {
		result = append(result, PaymentTransactionResponse{
			ID:              txn.ID,
			TransactionDate: txn.TransactionDate.Format("2006-01-02T15:04:05Z"),
			FromUser: TransactionUser{
				ID:           txn.FromUser.ID,
				FirstName:    txn.FromUser.FirstName,
				LastName:     txn.FromUser.LastName,
				Email:        txn.FromUser.Email,
				ProfilePhoto: txn.FromUser.ProfilePhoto,
			},
			ToUser: TransactionUser{
				ID:           txn.ToUser.ID,
				FirstName:    txn.ToUser.FirstName,
				LastName:     txn.ToUser.LastName,
				Email:        txn.ToUser.Email,
				ProfilePhoto: txn.ToUser.ProfilePhoto,
			},
			ReferenceType:  txn.ReferenceType,
			ReferenceID:    txn.ReferenceID,
			Amount:         txn.Amount,
			AppFeeAmount:   txn.AppFeeAmount,
			DiscountAmount: txn.DiscountAmount,
			NetAmount:      txn.NetAmount,
			Currency:       txn.Currency,
			Status:         txn.Status,
			ReferralCodeID: txn.ReferralCodeID,
			CreatedAt:      txn.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return result, total, nil
}

func (s *PaymentService) GetPaymentTransactionByID(id string) (*PaymentTransactionResponse, error) {
	var txn models.PaymentTransaction

	if err := s.db.
		Preload("FromUser").
		Preload("ToUser").
		Where("id = ?", id).
		First(&txn).Error; err != nil {
		return nil, err
	}

	return &PaymentTransactionResponse{
		ID:              txn.ID,
		TransactionDate: txn.TransactionDate.Format("2006-01-02T15:04:05Z"),
		FromUser: TransactionUser{
			ID:           txn.FromUser.ID,
			FirstName:    txn.FromUser.FirstName,
			LastName:     txn.FromUser.LastName,
			Email:        txn.FromUser.Email,
			ProfilePhoto: txn.FromUser.ProfilePhoto,
		},
		ToUser: TransactionUser{
			ID:           txn.ToUser.ID,
			FirstName:    txn.ToUser.FirstName,
			LastName:     txn.ToUser.LastName,
			Email:        txn.ToUser.Email,
			ProfilePhoto: txn.ToUser.ProfilePhoto,
		},
		ReferenceType:  txn.ReferenceType,
		ReferenceID:    txn.ReferenceID,
		Amount:         txn.Amount,
		AppFeeAmount:   txn.AppFeeAmount,
		DiscountAmount: txn.DiscountAmount,
		NetAmount:      txn.NetAmount,
		Currency:       txn.Currency,
		Status:         txn.Status,
		ReferralCodeID: txn.ReferralCodeID,
		CreatedAt:      txn.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}
