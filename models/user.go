package models

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Permission struct {
	BaseModel
	Slug        string `gorm:"uniqueIndex;not null" json:"slug"`
	Description string `json:"description"`
}

type Role struct {
	Name        string       `gorm:"primaryKey;not null;uniqueIndex"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions"`
}

type User struct {
	BaseModel
	FirstName       string `gorm:"column:first_name" json:"first_name"`
	LastName        string `gorm:"column:last_name" json:"last_name"`
	Email           string `gorm:"column:email" json:"email"`
	Password        string
	Disabled        bool   `gorm:"default:false;column:disabled" json:"disabled"`
	PhoneNumber     string `gorm:"column:phone_number" json:"phone_number"`
	PhoneVerified   bool   `gorm:"default:false;column:phone_verified" json:"phone_verified"`
	EmailVerified   bool   `gorm:"default:false;column:email_verified" json:"email_verified"`
	ProfilePhoto    string `gorm:"size:255;column:profile_photo" json:"profile_photo"`
	IdentityVerfied bool   `gorm:"default:false;column:identity_verified" json:"identity_verified"`
	GoogleID        string
	Roles           []Role `gorm:"many2many:user_roles;" json:"roles"`
}

func (u User) Can(slug string, db *gorm.DB) bool {
	var count int64
	db.Table("users").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Joins("JOIN role_permissions ON role_permissions.role_id = roles.id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("users.id = ? AND permissions.slug = ?", u.ID, slug).
		Count(&count)
	return count > 0
}

type Portfolio struct {
	BaseModel
	UserID      string `gorm:"index;not null" json:"user_id"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	ProjectURL  string `json:"project_url"`
	Media       []File `gorm:"polymorphic:Entity;polymorphicValue:portfolios" json:"media"`
}

type Certification struct {
	BaseModel
	UserID         string     `gorm:"index;not null" json:"user_id"`
	Name           string     `json:"name" binding:"required"`
	IssuingOrg     string     `json:"issuing_organization" binding:"required"`
	IssueDate      time.Time  `json:"issue_date" binding:"required"`
	ExpirationDate *time.Time `json:"expiration_date"` // Pointer allows null (no expiry)`
	ImageURL       string     `json:"image_url" binding:"required"`
}

type ReferralReward struct {
	BaseModel
	ReferrerID       string `gorm:"index;not null"`    // Who gets the money
	RefereeID        string `gorm:"index;not null"`    // The new user who joined
	ContractID       string `gorm:"index"`             // The job that triggered the reward
	Amount           int64  `gorm:"not null"`          // Amount in cents
	Status           string `gorm:"default:'pending'"` // pending, paid, cancelled
	StripeTransferID string `gorm:"index"`             // Proof of payment via Stripe
}

type ReferralCode struct {
	BaseModel
	Code    string `gorm:"type:varchar(20);uniqueIndex;not null"`
	OwnerID string `gorm:"index;not null"` // The User who owns the code
	Owner   User   `gorm:"foreignKey:OwnerID"`

	Type         string `gorm:"type:varchar(20);default:'standard'"` // standard, influencer, seasonal
	RewardAmount int64  `gorm:"default:500"`                         // Amount in cents (£5)
	MaxUses      int    `gorm:"default:-1"`                          // -1 for unlimited
	CurrentUses  int    `gorm:"default:0"`
	IsActive     bool   `gorm:"default:true"`
	ExpiresAt    *time.Time
}
type ReferralUsage struct {
	BaseModel
	ReferralCodeID string       `gorm:"index;not null" json:"referral_code_id"`
	ReferralCode   ReferralCode `gorm:"foreignKey:ReferralCodeID"`

	ReferrerID string `gorm:"index;not null" json:"referrer_id"`
	RefereeID  string `gorm:"uniqueIndex;not null" json:"referee_id"`

	RewardAmount int64  `json:"reward_amount"`
	Status       string `gorm:"type:varchar(20);default:'pending'"`

	IsQualified bool       `gorm:"default:false"`
	QualifiedAt *time.Time `json:"qualified_at,omitempty"`
}

// 1. Use AfterCreate, not AfterSave. We only increment when a NEW usage is created.
func (u *ReferralUsage) AfterCreate(tx *gorm.DB) (err error) {
	// 2. Atomic Increment: Much faster and safer than Count(*)
	// We update the 'ReferralCode' table where ID matches the Usage's foreign key
	err = tx.Model(&ReferralCode{}).
		Where("id = ?", u.ReferralCodeID).
		UpdateColumn("current_uses", gorm.Expr("current_uses + ?", 1)).
		Error

	if err != nil {
		return err // This will roll back the transaction automatically
	}
	return nil
}

func (ReferralUsage) TableName() string {
	return "referral_usages"
}
