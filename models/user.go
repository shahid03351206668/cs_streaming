package models

type Role struct {
	Name string `gorm:"primaryKey;not null;uniqueIndex"`
}

type User struct {
	BaseModel
	FirstName     string `gorm:"column:first_name" json:"first_name"`
	LastName      string `gorm:"column:last_name" json:"last_name"`
	Email         string `gorm:"column:email" json:"email"`
	Password      string
	PhoneNumber   string `gorm:"column:phone_number" json:"phone_number"`
	PhoneVerified bool   `gorm:"default:false;column:phone_verified" json:"phone_verified"`
	EmailVerified bool   `gorm:"default:false;column:email_verified" json:"email_verified"`
	ProfilePhoto  string `gorm:"size:255;column:profile_photo" json:"profile_photo"`
	GoogleID      string
	Disabled      bool `gorm:"default:false;column:disabled" json:"disabled"`
}

type UserRoles struct {
	UserID string `gorm:"primaryKey"`
	RoleID string `gorm:"primaryKey"`
	User   User
	Role   Role
}
