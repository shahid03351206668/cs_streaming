package models

import (
	"time"
)

type Role struct {
	Name string `gorm:"primaryKey;not null;uniqueIndex"`
}

type User struct {
	BaseModel
	FirstName     string
	LastName      string
	Email         string `gorm:"uniqueIndex"`
	Password      string
	PhoneNumber   string `gorm:"uniqueIndex"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	PhoneVerified bool   `gorm:"default:false"`
	EmailVerfied  bool   `gorm:"default:false"`
	ProfilePhoto  string `gorm:"size:255"`
}

type UserRoles struct {
	UserID string `gorm:"primaryKey"`
	RoleID string `gorm:"primaryKey"`

	User User
	Role Role
}
