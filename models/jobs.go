package models

type Category struct {
	BaseModel
	Name string `gorm:"primaryKey"`
}

type JobPost struct {
	BaseModel
	CreatedByID uint `gorm:"not null;index" json:"created_by_id"`
	CreatedBy   User `gorm:"foreignKey:CreatedByID" json:"created_by"`

	CategoryID uint     `gorm:"index" json:"category_id"`
	Category   Category `gorm:"foreignKey:CategoryID" json:"category"`

	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Description string     `gorm:"type:text;not null" json:"description"`
	Budget      float64    `gorm:"type:decimal(10,2)" json:"budget"`
	OpenBudget  bool       `gorm:"default:false" json:"open_budget"`
	Address     string     `gorm:"type:varchar(500)" json:"address"`
	JobMedia    []JobMedia `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job_media,omitempty"`
}

type JobMedia struct {
	BaseModel
	JobID     uint    `gorm:"not null;index" json:"job_id"`
	Job       JobPost `gorm:"foreignKey:JobID" json:"job,omitempty"`
	URL       string  `gorm:"type:varchar(500);not null" json:"url"`
	MediaType string  `gorm:"type:varchar(50);not null" json:"media_type"`
}
