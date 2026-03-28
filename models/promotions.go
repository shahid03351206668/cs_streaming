package models

import "time"

type PromotionalOffer struct {
	BaseModel
	Name        string `gorm:"type:varchar(100);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`

	// Eligibility conditions (both can be set; user must meet all non-zero conditions)
	MinJobsCompleted int `gorm:"default:0" json:"min_jobs_completed"`
	MinJobsPosted    int `gorm:"default:0" json:"min_jobs_posted"`

	// Discount applied on the platform fee
	DiscountPercentage float64 `gorm:"type:decimal(5,2);default:0" json:"discount_percentage"`
	DiscountAmount     int64   `gorm:"default:0" json:"discount_amount"`
	MaxDiscountAmount  int64   `gorm:"default:0" json:"max_discount_amount"` // 0 = no cap

	IsActive  bool       `gorm:"default:true" json:"is_active"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (PromotionalOffer) TableName() string {
	return "promotional_offers"
}
