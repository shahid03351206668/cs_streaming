package models

import "gorm.io/datatypes"

type Notification struct {
	BaseModel
	UserID string         `gorm:"type:string;not null;index" json:"user_id"`
	Title  string         `gorm:"type:varchar(255);not null" json:"title"`
	Body   string         `gorm:"type:text;not null" json:"body"`
	Type   string         `gorm:"type:varchar(50);index" json:"type"`
	Screen string         `gorm:"type:varchar(100)" json:"screen"`
	Data   datatypes.JSON `gorm:"type:jsonb" json:"data,omitempty"`
	Read   bool           `gorm:"default:false;index" json:"read"`
}

func (Notification) TableName() string {
	return "notifications"
}
