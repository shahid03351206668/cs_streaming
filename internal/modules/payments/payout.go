package payments

import (
	"tasksy/config"

	"gorm.io/gorm"
)

type PayoutService struct {
	db     *gorm.DB
	config *config.StripeConfig
}

func NewPayoutService(db *gorm.DB, cfg *config.StripeConfig) *PayoutService {
	return &PayoutService{db: db, config: cfg}
}
