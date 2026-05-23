package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
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

	Amount        int64  `gorm:"not null" json:"amount"`
	Currency      string `gorm:"type:varchar(3);default:'gbp'"`
	PaymentMethod string `gorm:"type:varchar(50);not null" json:"payment_method"`
	GatewayRefID  string `gorm:"type:varchar(100);index" json:"gateway_ref_id"`

	FreelancerCommissionPercentage float64 `gorm:"default:0" json:"freelancer_commission_percentage"`
	FreelancerCommissionAmount     int64   `gorm:"default:0" json:"freelancer_commission_amount"`
	ClientCommissionPercentage     float64 `gorm:"default:0" json:"client_commission_percentage"`
	ClientCommissionAmount         int64   `gorm:"default:0" json:"client_commission_amount"`
	AppFeePercentage               float64 `gorm:"default:0" json:"app_fee_percentage"`
	AppFeeAmount                   int64   `gorm:"default:0" json:"app_fee_amount"`
	ReferralDiscountPercentage     float64 `gorm:"default:0" json:"referral_discount_percentage"`
	ReferralDiscountAmount         int64   `gorm:"default:0" json:"referral_discount_amount"`
	NetAmount                      int64   `gorm:"not null" json:"net_amount"`

	DiscountAmount int64 `gorm:"default:0" json:"discount_amount"`

	ReferralCodeID       *string       `gorm:"index" json:"referral_code_id,omitempty"`
	ReferralCode         *ReferralCode `gorm:"foreignKey:ReferralCodeID" json:"referral_code,omitempty"`
	ReferrerID           *string       `gorm:"index" json:"referrer_id,omitempty"`
	Referrer             *User         `gorm:"foreignKey:ReferrerID" json:"referrer,omitempty"`
	ReferralRewardAmount int64         `gorm:"default:0" json:"referral_reward_amount"`

	Status   string             `gorm:"index;not null"`
	PayoutID *string            `gorm:"index" json:"payout_id,omitempty"`
	Payout   *PayoutTransaction `gorm:"foreignKey:PayoutID" json:"payout,omitempty"`
}

func (PaymentTransaction) TableName() string {
	return "payment_transaction"
}

const (
	PayoutPending = "pending"
	PayoutSuccess = "success"
	PayoutFailed  = "failed"
)

type PayoutTransaction struct {
	BaseModel

	TransactionDate time.Time `gorm:"not null" json:"transaction_date"`
	UserID          string    `gorm:"index;not null" json:"user_id"`
	User            User      `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"user"`

	StripePayoutID string           `gorm:"type:varchar(100)" json:"stripe_payout_id"`
	BankAccountID  *string          `gorm:"index" json:"bank_account_id,omitempty"`
	BankAccount    *UserBankAccount `gorm:"foreignKey:BankAccountID" json:"bank_account,omitempty"`

	Amount   decimal.Decimal `gorm:"default:0" json:"amount"`
	Currency string          `gorm:"type:varchar(3);default:'gbp'" json:"currency"`
	Status   string          `gorm:"index;not null;default:'pending'" json:"status"`
}

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

type PaymentAuditLog struct {
	BaseModel

	EventType     string         `gorm:"type:varchar(50);index;not null" json:"event_type"`
	StripeEventID string         `gorm:"type:varchar(100);index" json:"stripe_event_id"`
	Action        string         `gorm:"type:varchar(100);not null" json:"action"`
	EntityType    string         `gorm:"type:varchar(50);index" json:"entity_type"` // payment_transaction, payout_transaction, contract
	EntityID      string         `gorm:"type:varchar(100);index" json:"entity_id"`
	UserID        string         `gorm:"index" json:"user_id,omitempty"`
	Amount        int64          `gorm:"default:0" json:"amount"`
	Currency      string         `gorm:"type:varchar(3);default:'gbp'" json:"currency"`
	Details       datatypes.JSON `gorm:"type:jsonb" json:"details,omitempty"`
	Status        string         `gorm:"type:varchar(50)" json:"status"`
}

func (PaymentAuditLog) TableName() string {
	return "payment_audit_logs"
}

type PaymentTransactionV2 struct {
	BaseModel

	PostingDate time.Time `json:"posting_date"`

	ContractID string   `gorm:"not null" json:"contract_id"`
	Contract   Contract `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"contract"`

	StripeEventID string `gorm:"not null" json:"stripe_event_id"`

	FromUserID string `gorm:"index;not null" json:"from_user_id"`
	ToUserID   string `gorm:"index;not null" json:"to_user_id"`

	FromUser User `gorm:"foreignKey:FromUserID;constraint:OnDelete:CASCADE" json:"from_user"`
	ToUser   User `gorm:"foreignKey:ToUserID;constraint:OnDelete:CASCADE" json:"to_user"`

	Amount    decimal.Decimal `gorm:"default:0" json:"amount"`
	AppFee    decimal.Decimal `gorm:"default:0" json:"app_fee"`
	NetAmount decimal.Decimal `gorm:"default:0" json:"net_amount"`

	ClientCommPct     float64 `gorm:"default:0" json:"client_comm_pct"`
	FreelancerCommPct float64 `gorm:"default:0" json:"freelancer_comm_pct"`

	DisountAmount decimal.Decimal `gorm:"default:0" json:"discount_amount"`
	Status        string          `gorm:"default:created"`
}

func (PaymentTransactionV2) TableName() string {
	return "payment_transactionv2"
}

type EscrowStatus string

const (
	EscrowStatusHeld     EscrowStatus = "held"
	EscrowStatusReleased EscrowStatus = "released"
	EscrowStatusRefunded EscrowStatus = "refunded"
)

type EscrowTransaction struct {
	BaseModel

	UserID string `gorm:"index;not null" json:"user_id"`
	User   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" `

	ContractID string
	Contract   Contract `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE"`

	TransactionID string

	Amount     decimal.Decimal
	Status     EscrowStatus
	ReleasedAt *time.Time
}

type UserAccountDetails struct {
	BaseModel

	UserID        string `gorm:"index;not null" json:"user_id"`
	User          User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user"`
	AccountNo     string `gorm:"not null" json:"account_no"`
	CountryCode   string `gorm:"size:2" json:"country_code"`
	Currency      string `gorm:"size:3;not null" json:"currency"`
	RoutingNumber string `jsom:"routing_no"`
}

// type UserBankAccount struct {
// 	BaseModel

// 	UserID                 string `gorm:"index;not null" json:"user_id"`
// 	User                   User   `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
// 	StripeConnectAccountID string `gorm:"type:varchar(100);not null" json:"stripe_connect_account_id"`
// 	StripeBankAccountID    string `gorm:"type:varchar(100);not null" json:"stripe_bank_account_id"`
// 	AccountHolderName      string `gorm:"type:varchar(255);not null" json:"account_holder_name"`
// 	SortCode               string `gorm:"type:varchar(10)" json:"sort_code"`
// 	AccountNumber          string `gorm:"type:varchar(20)" json:"account_number"`
// 	AccountNumberLast4     string `gorm:"type:varchar(4)" json:"account_number_last4"`
// 	BankName               string `gorm:"type:varchar(100)" json:"bank_name"`
// 	BankLogoURL            string `gorm:"type:varchar(500)" json:"bank_logo_url"`
// 	Currency               string `gorm:"type:varchar(3);default:'gbp'" json:"currency"`
// 	IsDefault              bool   `gorm:"default:false" json:"is_default"`
// }
