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
	AppFeePercentage               float64 `gorm:"type:decimal(5,2);default:0" json:"app_fee_percentage"`
	ReferralDiscountPercentage     float64 `gorm:"type:decimal(5,2);default:0" json:"referral_discount_percentage"`
	ReferralRewardAmount           int64   `gorm:"default:0" json:"referral_reward_amount"`
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

type EmailAccount struct {
	BaseModel

	Name      string `gorm:"type:varchar(100);not null" json:"name"`
	Host      string `gorm:"type:varchar(255);not null" json:"host"`
	Port      int    `gorm:"not null;default:587" json:"port"`
	Email     string `gorm:"type:varchar(255);not null" json:"email"`
	Password  string `gorm:"type:varchar(255);not null" json:"-"`
	FromName  string `gorm:"type:varchar(100);not null" json:"from_name"`
	IsActive  bool   `gorm:"default:true" json:"is_active"`
	IsDefault bool   `gorm:"default:false" json:"is_default"`
}

func (EmailAccount) TableName() string {
	return "email_accounts"
}

type EmailTemplate struct {
	BaseModel
	Name    string `gorm:"type:varchar(150);not null" json:"name"`
	Subject string `gorm:"type:varchar(255);not null" json:"subject"`
	Body    string `gorm:"type:text;not null" json:"body"`
}

func (EmailTemplate) TableName() string {
	return "email_templates"
}
