package models

import "time"

type AccountType string

const (
	AccountTypeUserWallet AccountType = "user_wallet"
	AccountTypeEscrow     AccountType = "escrow"
	AccountTypeRevenue    AccountType = "revenue"
	AccountTypeMarketing  AccountType = "marketing"
	AccountTypeExternal   AccountType = "external"
)

type Account struct {
	BaseModel
	UserID *string     `gorm:"index" json:"user_id,omitempty"`
	User   *User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Type   AccountType `gorm:"type:varchar(30);not null;index" json:"type"`
	Name   string      `gorm:"type:varchar(100);not null" json:"name"`
}

func (Account) TableName() string {
	return "accounts"
}

type LedgerTransactionType string

const (
	LedgerTxEscrowFund       LedgerTransactionType = "escrow_fund"
	LedgerTxEscrowRelease    LedgerTransactionType = "escrow_release"
	LedgerTxEscrowRefund     LedgerTransactionType = "escrow_refund"
	LedgerTxPlatformFee      LedgerTransactionType = "platform_fee"
	LedgerTxReferralReward   LedgerTransactionType = "referral_reward"
	LedgerTxReferralDiscount LedgerTransactionType = "referral_discount"
	LedgerTxPayout           LedgerTransactionType = "payout"
)

type LedgerTransaction struct {
	BaseModel
	Type        LedgerTransactionType `gorm:"type:varchar(50);not null;index" json:"type"`
	ReferenceID string                `gorm:"type:varchar(100);index" json:"reference_id"`
	Status      string                `gorm:"type:varchar(20);not null;default:'posted'" json:"status"`
	Description string                `gorm:"type:varchar(500)" json:"description"`
	PostingDate time.Time             `gorm:"not null;index" json:"posting_date"`
	Entries     []GLEntry             `gorm:"foreignKey:TransactionID" json:"entries,omitempty"`
}

func (LedgerTransaction) TableName() string {
	return "ledger_transactions"
}

type GLEntry struct {
	BaseModel
	TransactionID string             `gorm:"type:varchar(100);not null;index" json:"transaction_id"`
	Transaction   *LedgerTransaction `gorm:"foreignKey:TransactionID" json:"-"`
	AccountID     string             `gorm:"type:varchar(100);not null;index" json:"account_id"`
	Account       *Account           `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	Amount        int64              `gorm:"not null" json:"amount"`
	Category      string             `gorm:"type:varchar(50)" json:"category"`
}

func (GLEntry) TableName() string {
	return "gl_entries"
}
