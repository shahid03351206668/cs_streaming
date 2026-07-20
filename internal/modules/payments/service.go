package payments

import (
	"tasksy/config"

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
