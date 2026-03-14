package models

type NotificationPreference struct {
	BaseModel
	UserID                 string                           `gorm:"not null;uniqueIndex" json:"user_id"`
	EnableProposalSent     bool                             `gorm:"default:true" json:"enable_proposal_sent"`
	EnableProposalReceived bool                             `gorm:"default:true" json:"enable_proposal_received"`
	EnableProposalDecision bool                             `gorm:"default:true" json:"enable_proposal_decision"`
	EnableNewJobs          bool                             `gorm:"default:true" json:"enable_new_jobs"`
	JobRadiusKM            float64                          `gorm:"default:50" json:"job_radius_km"`
	City                   string                           `gorm:"type:varchar(120)" json:"city"`
	Latitude               *float64                         `gorm:"type:decimal(11,8)" json:"latitude,omitempty"`
	Longitude              *float64                         `gorm:"type:decimal(11,8)" json:"longitude,omitempty"`
	Categories             []NotificationPreferenceCategory `gorm:"foreignKey:PreferenceID;constraint:OnDelete:CASCADE" json:"categories"`
}

type NotificationPreferenceCategory struct {
	BaseModel
	PreferenceID string   `gorm:"not null;index" json:"preference_id"`
	CategoryID   string   `gorm:"not null;index" json:"category_id"`
	Category     Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE" json:"category,omitempty"`
}

func (NotificationPreference) TableName() string {
	return "notification_preferences"
}

func (NotificationPreferenceCategory) TableName() string {
	return "notification_preference_categories"
}
