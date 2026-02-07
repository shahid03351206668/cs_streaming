package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseModel struct {
	ID        string         `gorm:"type:string;primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"deleted_at"`
}

func (model *BaseModel) BeforeCreate(tx *gorm.DB) (err error) {
	if model.ID == "" {
		model.ID = uuid.New().String()
	}
	return nil
}

type SystemSettings struct {
	ID                             string  `gorm:"type:string;primaryKey" json:"id"`
	ClientCommissionPercentage     float64 `gorm:"type:decimal(5,2);default:0" json:"client_commission_percentage"`
	FreelancerCommissionPercentage float64 `gorm:"type:decimal(5,2);default:0" json:"freelancer_commission_percentage"`
	ApplicationFeeAmount           int64   `gorm:"default:0" json:"application_fee_amount"`
}

func (SystemSettings) TableName() string {
	return "system_settings"
}

func (s *SystemSettings) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == "" {
		s.ID = "system_settings"
	}

	return nil
}
