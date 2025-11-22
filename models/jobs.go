package models

const (
	JobStatusDraft      = "draft"
	JobStatusOpen       = "open"
	JobStatusInProgress = "in_progress"
	JobStatusCompleted  = "completed"
	JobStatusCancelled  = "cancelled"
	JobStatusClosed     = "closed"
	JobStatusOnHold     = "on_hold"
)

const (
	ProposalStatusPending     = "pending"
	ProposalStatusShortlisted = "shortlisted"
	ProposalStatusAccepted    = "accepted"
	ProposalStatusRejected    = "rejected"
	ProposalStatusWithdrawn   = "withdrawn"
)

type Category struct {
	BaseModel
	Name    string `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Disable bool   `gorm:"default:false" json:"disable"`
}

type JobPost struct {
	BaseModel
	CreatedByID string     `gorm:"not null;index" json:"created_by_id"`
	CreatedBy   User       `gorm:"foreignKey:CreatedByID;constraint:OnDelete:CASCADE" json:"created_by"`
	CategoryID  string     `gorm:"index" json:"category_id"`
	Category    Category   `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"category"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Description string     `gorm:"type:text;not null" json:"description"`
	Budget      float64    `gorm:"type:decimal(10,2)" json:"budget"`
	OpenBudget  bool       `gorm:"default:false" json:"open_budget"`
	Address     string     `gorm:"type:varchar(500)" json:"address"`
	Status      string     `gorm:"type:varchar(50);default:'open';index" json:"status"`
	JobMedia    []JobMedia `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job_media,omitempty"`
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

type Proposal struct {
	BaseModel
	JobPostID    string  `gorm:"not null;index" json:"job_post_id"`
	JobPost      JobPost `gorm:"foreignKey:JobPostID;constraint:OnDelete:CASCADE" json:"job_post"`
	FreelancerID string  `gorm:"not null;index" json:"freelancer_id"`
	Freelancer   User    `gorm:"foreignKey:FreelancerID;constraint:OnDelete:CASCADE" json:"freelancer"`
	CoverLetter  string  `gorm:"type:text;not null" json:"cover_letter"`
	BidAmount    float64 `gorm:"type:decimal(10,2);not null" json:"bid_amount"`
	Duration     int     `gorm:"not null" json:"duration"`
	Status       string  `gorm:"type:varchar(50);default:'pending';index" json:"status"`
	// Status: pending, shortlisted, accepted, rejected, withdrawn
	ProposalAttachments []ProposalAttachment `gorm:"foreignKey:ProposalID;constraint:OnDelete:CASCADE" json:"attachments,omitempty"`
}

type ProposalAttachment struct {
	BaseModel
	ProposalID string   `gorm:"not null;index" json:"proposal_id"`
	Proposal   Proposal `gorm:"foreignKey:ProposalID;constraint:OnDelete:CASCADE" json:"proposal,omitempty"`
	URL        string   `gorm:"type:varchar(500);not null" json:"url"`
	FileName   string   `gorm:"type:varchar(255)" json:"file_name"`
	FileSize   int64    `gorm:"type:bigint" json:"file_size"`
	FileType   string   `gorm:"type:varchar(50)" json:"file_type"`
}
