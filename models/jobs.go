package models

type Category struct {
	BaseModel
	Name    string `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Disable bool   `gorm:"default:false" json:"disable"`
}

type JobPost struct {
	BaseModel
	CreatedByID string `gorm:"not null;index" json:"created_by_id"`
	CreatedBy   User   `gorm:"foreignKey:CreatedByID;constraint:OnDelete:CASCADE" json:"created_by"`

	CategoryID string   `gorm:"index" json:"category_id"`
	Category   Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"category"`

	Title       string  `gorm:"type:varchar(255);not null" json:"title"`
	Description string  `gorm:"type:text;not null" json:"description"`
	Budget      float64 `gorm:"type:decimal(10,2)" json:"budget"`
	OpenBudget  bool    `gorm:"default:false" json:"open_budget"`
	Address     string  `gorm:"type:varchar(500)" json:"address"`
	Status      string  `gorm:"type:varchar(50);default:'open';index" json:"status"`
	// open, closed, in_progress, completed
	JobMedia []JobMedia `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job_media,omitempty"`
}

type JobMedia struct {
	BaseModel
	JobID     string  `gorm:"not null;index" json:"job_id"`
	Job       JobPost `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job,omitempty"`
	URL       string  `gorm:"type:varchar(500);not null" json:"url"`
	MediaType string  `gorm:"type:varchar(50);not null" json:"media_type"`
	FileName  string  `gorm:"type:varchar(255)" json:"file_name"`
	FileSize  int64   `gorm:"type:bigint" json:"file_size"`
}

func (Category) TableName() string {
	return "categories"
}

func (JobPost) TableName() string {
	return "job_posts"
}

func (JobMedia) TableName() string {
	return "job_media"
}
