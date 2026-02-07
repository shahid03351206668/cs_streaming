package models

import (
	"time"

	"gorm.io/datatypes"
)

const (
	PaymentStatusSuccess  = "succeeded"
	PaymentStatusPending  = "pending"
	PaymentStatusFailed   = "failed"
	PaymentStatusRefunded = "refunded"
	PaymentStatusDisputed = "disputed"
)

const (
	ReferralStatusPending   = "pending"
	ReferralStatusQualified = "qualified"
	ReferralStatusRewarded  = "rewarded"
	ReferralStatusExpired   = "expired"
)

type PaymentTransaction struct {
	BaseModel
	MetaData        datatypes.JSON `gorm:"type:jsonb"`
	TransactionDate time.Time      `gorm:"not null" json:"transaction_date"`
	FromUserID      string         `gorm:"index;not null" json:"from_user_id"`
	ToUserID        string         `gorm:"index;not null" json:"to_user_id"`

	FromUser User `gorm:"foreignKey:FromUserID;constraint:OnDelete:CASCADE" json:"from_user"`
	ToUser   User `gorm:"foreignKey:ToUserID;constraint:OnDelete:CASCADE" json:"to_user"`

	ReferenceType string `gorm:"index" json:"reference_type"`
	ReferenceID   string `gorm:"index" json:"reference_id"`

	PaymentIntentID string `gorm:"uniqueIndex;type:varchar(100);not null"`
	ChargeID        string `gorm:"index;type:varchar(100)"`
	StripeEventID   string `gorm:"uniqueIndex;type:varchar(100)"`

	Amount         int64  `gorm:"not null" json:"amount"`
	AppFeeAmount   int64  `gorm:"default:0" json:"app_fee_amount"`
	DiscountAmount int64  `gorm:"default:0" json:"discount_amount"`
	NetAmount      int64  `gorm:"not null" json:"net_amount"`
	Currency       string `gorm:"type:varchar(3);default:'gbp'"`

	// Referral tracking
	ReferralCodeID *string       `gorm:"index" json:"referral_code_id,omitempty"`
	ReferralCode   *ReferralCode `gorm:"foreignKey:ReferralCodeID" json:"referral_code,omitempty"`

	Status string `gorm:"index;not null"`
}

func (PaymentTransaction) TableName() string {
	return "payment_transaction"
}

type PayoutTransaction struct {
	BaseModel

	TransactionDate time.Time `gorm:"not null" json:"transaction_date"`
	UserID          string    `gorm:"index;not null" json:"user_id"`
	User            User      `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"user"`

	StripeID string `gorm:"not null" json:"stripe_id"`

	Amount       int64 `gorm:"default:0" json:"amount"`
	NetAmount    int64 `gorm:"default:0" json:"net_amount"`
	AppFeeAmount int64 `gorm:"default:0" json:"app_fee_amount"`

	Currency string `gorm:"type:varchar(3);default:'gbp'" json:"currency"`
	Status   string `gorm:"index;not null;default:'pending'" json:"status"`
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
