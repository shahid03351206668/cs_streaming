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

	GrossAmount   decimal.Decimal `gorm:"default:0" json:"gross_amount"`
	ClientFee     decimal.Decimal `gorm:"default:0" json:"client_fee"`     // % charged to client
	FreelancerFee decimal.Decimal `gorm:"default:0" json:"freelancer_fee"` // % charged to freelancer
	PlatformFee   decimal.Decimal `gorm:"default:0" json:"platform_fee"`   // ClientFee + FreelancerFee
	NetAmount     decimal.Decimal `gorm:"default:0" json:"net_amount"`     // freelancer payout = GrossAmount - ClientFee - FreelancerFee
	Currency      string          `gorm:"default:usd" json:"currency"`
	Status        string          `gorm:"default:held" json:"status"`
	FlowVersion   string          `gorm:"default:v3" json:"flow_version"`
}

func (PaymentTransactionV3) TableName() string {
	return "payment_transactions_v3"
}

type EscrowTransactionV3 struct {
	BaseModel

	PaymentTransactionID string               `gorm:"index;not null" json:"payment_transaction_id"`
	PaymentTransaction   PaymentTransactionV3 `gorm:"foreignKey:PaymentTransactionID;constraint:OnDelete:CASCADE" json:"-"`
	ContractID           string               `gorm:"index;not null" json:"contract_id"`
	UserID               string               `gorm:"index;not null" json:"user_id"`
	User                 User                 `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	StripeTransferID     string               `json:"stripe_transfer_id"`
	Amount               decimal.Decimal      `gorm:"default:0" json:"amount"`
	Currency             string               `gorm:"default:usd" json:"currency"`
	Status               string               `gorm:"default:held" json:"status"`
	HeldAt               time.Time            `json:"held_at"`
	ReleasedAt           *time.Time           `json:"released_at"`
	// Audit trail for admin-forced releases; empty for automatic releases.
	ReleasedBy  string `gorm:"type:varchar(64)" json:"released_by,omitempty"`
	ReleaseNote string `gorm:"type:text" json:"release_note,omitempty"`
}

func (EscrowTransactionV3) TableName() string {
	return "escrow_transactions_v3"
}

// WithdrawalV3 is a tasker-initiated Stripe payout from their Connect balance
// to their bank. Status mirrors Stripe: pending, in_transit, paid, failed, canceled.
type WithdrawalV3 struct {
	BaseModel

	UserID          string          `gorm:"index;not null" json:"user_id"`
	StripeAccountID string          `gorm:"type:varchar(100);not null" json:"-"`
	StripePayoutID  string          `gorm:"type:varchar(100);index" json:"stripe_payout_id"`
	Amount          decimal.Decimal `gorm:"default:0" json:"amount"`
	Currency        string          `gorm:"default:gbp" json:"currency"`
	Status          string          `gorm:"type:varchar(20);default:pending;index" json:"status"`
	FailureMessage  string          `gorm:"type:text" json:"failure_message,omitempty"`
	ArrivalDate     *time.Time      `json:"arrival_date,omitempty"`
	// Bank the payout went to. BankAccountID is empty when Stripe's default was used.
	BankAccountID string `gorm:"type:varchar(64)" json:"bank_account_id,omitempty"`
	DestinationID string `gorm:"type:varchar(100)" json:"destination_id,omitempty"`
	BankLast4     string `gorm:"type:varchar(4)" json:"bank_last4,omitempty"`
}

func (WithdrawalV3) TableName() string {
	return "withdrawals_v3"
}
