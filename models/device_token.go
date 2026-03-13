package models

import "time"

// DeviceToken stores an FCM device token for a user.
// A user may have multiple tokens (multiple devices), but each token is globally unique.
type DeviceToken struct {
	BaseModel
	UserID     string    `gorm:"not null;index" json:"user_id"`
	Token      string    `gorm:"not null;uniqueIndex" json:"token"`
	Platform   string    `gorm:"type:varchar(20)" json:"platform,omitempty"`
	LastSeenAt time.Time `gorm:"not null" json:"last_seen_at"`
}

func (DeviceToken) TableName() string {
	return "device_tokens"
}
