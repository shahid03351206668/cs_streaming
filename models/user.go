package models

type Role struct {
	Name string `gorm:"primaryKey;not null;uniqueIndex"`
}

type User struct {
	BaseModel
	FirstName     string `gorm:"column:first_name" json:"first_name"`
	LastName      string `gorm:"column:last_name" json:"last_name"`
	Email         string `gorm:"uniqueIndex"`
	Password      string
	PhoneNumber   string `gorm:"uniqueIndex;column:phone_number" json:"phone_number"`
	PhoneVerified bool   `gorm:"default:false;column:phone_verfied" json:"phone_verfied"`
	EmailVerfied  bool   `gorm:"default:false;column:email_verfied" json:"email_verfied"`
	ProfilePhoto  string `gorm:"size:255;column:profile_photo" json:"profile_photo"`
	GoogleID      string
}

type UserRoles struct {
	UserID string `gorm:"primaryKey"`
	RoleID string `gorm:"primaryKey"`
	User   User
	Role   Role
}
