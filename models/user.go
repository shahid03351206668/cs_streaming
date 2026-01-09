package models

import (
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
