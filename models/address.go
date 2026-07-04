package models

type UserAddress struct {
	BaseModel
	UserID     string `gorm:"index;not null" json:"user_id"`
	Line1      string `gorm:"type:varchar(255);not null" json:"line1"`
	Line2      string `gorm:"type:varchar(255)" json:"line2"`
	City       string `gorm:"type:varchar(100);not null" json:"city"`
	State      string `gorm:"type:varchar(100)" json:"state"`
	PostalCode string `gorm:"type:varchar(20);not null" json:"postal_code"`
	Country    string `gorm:"type:varchar(2);not null" json:"country"`
	IsDefault  bool   `gorm:"default:false" json:"is_default"`
}

func (UserAddress) TableName() string { return "user_addresses" }
