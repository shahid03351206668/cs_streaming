package models

import (
	"time"
)

const (
	PaymentStatusCreated   = "created"
	PaymentStatusPending   = "pending"
	PaymentStatusHeld      = "held"
	PaymentStatusDisputed  = "disputed"
	PaymentStatusRefunded  = "refunded"
	PaymentStatusSuccess   = "succeeded"
	PaymentStatusSucceed   = "succeed"
	PaymentStatusFailed    = "failed"
	PaymentStatusCancelled = "cancelled"
)

const (
	ReferralStatusPending   = "pending"
	ReferralStatusQualified = "qualified"
	ReferralStatusRewarded  = "rewarded"
	ReferralStatusExpired   = "expired"
)


type UserBankAccount struct {
	BaseModel

	UserID                 string `gorm:"index;not null" json:"user_id"`
	User                   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	StripeConnectAccountID string `gorm:"type:varchar(100);not null" json:"stripe_connect_account_id"`
	StripeBankAccountID    string `gorm:"type:varchar(100);not null" json:"stripe_bank_account_id"`
	AccountHolderName      string `gorm:"type:varchar(255);not null" json:"account_holder_name"`
	SortCode               string `gorm:"type:varchar(10)" json:"sort_code"`
	AccountNumber          string `gorm:"type:varchar(20)" json:"account_number"`
	AccountNumberLast4     string `gorm:"type:varchar(4)" json:"account_number_last4"`
	BankName               string `gorm:"type:varchar(100)" json:"bank_name"`
	BankLogoURL            string `gorm:"type:varchar(500)" json:"bank_logo_url"`
	Currency               string `gorm:"type:varchar(3);default:'gbp'" json:"currency"`
	IsDefault              bool   `gorm:"default:false" json:"is_default"`
}

func (UserBankAccount) TableName() string {
	return "user_bank_accounts"
}

type ReferralCode struct {
	BaseModel
	Code    string `gorm:"type:varchar(20);uniqueIndex;not null" json:"code"`
	OwnerID string `gorm:"index;not null" json:"owner_id"`
	Owner   User   `gorm:"foreignKey:OwnerID" json:"owner,omitempty"`
	Type    string `gorm:"type:varchar(20);default:'standard'" json:"type"`

	DiscountAmount     int64 `gorm:"default:0" json:"discount_amount"`
	DiscountPercentage int64 `gorm:"default:0" json:"discount_percentage"`

	MaxUses     int `gorm:"default:-1" json:"max_uses"`
	CurrentUses int `gorm:"default:0" json:"current_uses"`

	IsActive  bool       `gorm:"default:true" json:"is_active"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (ReferralCode) TableName() string {
	return "referral_codes"
}

type ReferralUsage struct {
	BaseModel

	ReferralCodeID string       `gorm:"index;not null" json:"referral_code_id"`
	ReferralCode   ReferralCode `gorm:"foreignKey:ReferralCodeID" json:"referral_code,omitempty"`

	ReferrerID string `gorm:"index;not null" json:"referrer_id"`
	Referrer   User   `gorm:"foreignKey:ReferrerID" json:"referrer,omitempty"`
	RefereeID  string `gorm:"uniqueIndex;not null" json:"referee_id"`
	Referee    User   `gorm:"foreignKey:RefereeID" json:"referee,omitempty"`

	DiscountApplied int64  `gorm:"default:0" json:"discount_applied"`
	RewardAmount    int64  `gorm:"default:0" json:"reward_amount"`
	Status          string `gorm:"type:varchar(20);default:'pending'" json:"status"`

	IsQualified           bool       `gorm:"default:false" json:"is_qualified"`
	QualifiedAt           *time.Time `json:"qualified_at,omitempty"`
	FirstTransactionID    *string    `gorm:"index" json:"first_transaction_id,omitempty"`
	DiscountTransactionID *string    `gorm:"index" json:"discount_transaction_id,omitempty"`
}

func (ReferralUsage) TableName() string {
	return "referral_usages"
}

type Bank struct {
	BaseModel
	Name     string `gorm:"type:varchar(100);not null" json:"name"`
	SortCode string `gorm:"type:varchar(10)" json:"sort_code"`
	LogoURL  string `gorm:"type:varchar(500)" json:"logo_url"`
	IsActive bool   `gorm:"default:true" json:"is_active"`
}

func (Bank) TableName() string {
	return "banks"
}

