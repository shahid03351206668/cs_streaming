package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type PaymentTransactionV3 struct {
	BaseModel

	StripePaymentIntentID string `gorm:"uniqueIndex;not null" json:"stripe_payment_intent_id"`
	StripeChargeID        string `json:"stripe_charge_id"`
	StripeEventID         string `gorm:"uniqueIndex;not null" json:"stripe_event_id"`

	FromUserID string `gorm:"index;not null" json:"from_user_id"`
	ToUserID   string `gorm:"index;not null" json:"to_user_id"`
	FromUser   User   `gorm:"foreignKey:FromUserID;constraint:OnDelete:CASCADE" json:"from_user"`
	ToUser     User   `gorm:"foreignKey:ToUserID;constraint:OnDelete:CASCADE" json:"to_user"`

	ContractID string   `gorm:"index;not null" json:"contract_id"`
	Contract   Contract `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"contract"`

	GrossAmount    decimal.Decimal `gorm:"default:0" json:"gross_amount"`
	ClientFee      decimal.Decimal `gorm:"default:0" json:"client_fee"`      // % charged to client
	FreelancerFee  decimal.Decimal `gorm:"default:0" json:"freelancer_fee"`  // % charged to freelancer
	PlatformFee    decimal.Decimal `gorm:"default:0" json:"platform_fee"`    // ClientFee + FreelancerFee
	NetAmount      decimal.Decimal `gorm:"default:0" json:"net_amount"`      // freelancer payout = GrossAmount - ClientFee - FreelancerFee
	Currency       string          `gorm:"default:usd" json:"currency"`
	Status         string          `gorm:"default:held" json:"status"`
	FlowVersion    string          `gorm:"default:v3" json:"flow_version"`
}

func (PaymentTransactionV3) TableName() string {
	return "payment_transactions_v3"
}

type EscrowTransactionV3 struct {
	BaseModel

	PaymentTransactionID string               `gorm:"index;not null" json:"payment_transaction_id"`
	PaymentTransaction    PaymentTransactionV3 `gorm:"foreignKey:PaymentTransactionID;constraint:OnDelete:CASCADE" json:"-"`
	ContractID            string               `gorm:"index;not null" json:"contract_id"`
	UserID                string               `gorm:"index;not null" json:"user_id"`
	User                  User                 `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	StripeTransferID      string               `json:"stripe_transfer_id"`
	Amount                decimal.Decimal      `gorm:"default:0" json:"amount"`
	Currency              string               `gorm:"default:usd" json:"currency"`
	Status                string               `gorm:"default:held" json:"status"`
	HeldAt                time.Time            `json:"held_at"`
	ReleasedAt            *time.Time           `json:"released_at"`
}

func (EscrowTransactionV3) TableName() string {
	return "escrow_transactions_v3"
}
