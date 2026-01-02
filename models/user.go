package models

import "gorm.io/gorm"

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
