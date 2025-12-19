package models

import "time"

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

	CreatedBy User     `gorm:"foreignKey:CreatedByID;constraint:OnDelete:CASCADE" json:"created_by"`
	Category  Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"category"`

	CreatedByID string  `gorm:"type:string;not null;index" json:"created_by_id"`
	CategoryID  string  `gorm:"index" json:"category_id"`
	Title       string  `gorm:"type:varchar(255);not null" json:"title"`
	Description string  `gorm:"type:text;not null" json:"description"`
	Budget      float64 `gorm:"type:decimal(10,2)" json:"budget"`
	OpenBudget  bool    `gorm:"default:false" json:"open_budget"`
	Address     string  `gorm:"type:varchar(500)" json:"address"`
	Status      string  `gorm:"type:varchar(50);default:'open';index" json:"status"`

	JobMedia  []JobMedia `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job_media,omitempty"`
	Proposals []Proposal `gorm:"foreignKey:JobPostID;constraint:OnDelete:CASCADE" json:"-"`
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

const (
	ContractStatusPending    = "pending"
	ContractStatusActive     = "active"
	ContractStatusCompleted  = "completed"
	ContractStatusCancelled  = "cancelled"
	ContractStatusDisputed   = "disputed"
	ContractStatusTerminated = "terminated"
)

const (
	PaymentStatusPending    = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusCompleted  = "completed"
	PaymentStatusFailed     = "failed"
	PaymentStatusRefunded   = "refunded"
)

type Contract struct {
	BaseModel
	JobPostID    string    `gorm:"not null;index" json:"job_post_id"`
	JobPost      JobPost   `gorm:"foreignKey:JobPostID;constraint:OnDelete:CASCADE" json:"job_post"`
	ProposalID   string    `gorm:"not null;uniqueIndex" json:"proposal_id"`
	Proposal     Proposal  `gorm:"foreignKey:ProposalID;constraint:OnDelete:CASCADE" json:"proposal"`
	ClientID     string    `gorm:"not null;index" json:"client_id"`
	Client       User      `gorm:"foreignKey:ClientID;constraint:OnDelete:CASCADE" json:"client"`
	FreelancerID string    `gorm:"not null;index" json:"freelancer_id"`
	Freelancer   User      `gorm:"foreignKey:FreelancerID;constraint:OnDelete:CASCADE" json:"freelancer"`
	Title        string    `gorm:"type:varchar(255);not null" json:"title"`
	Description  string    `gorm:"type:text" json:"description"`
	TotalAmount  float64   `gorm:"type:decimal(10,2);not null" json:"total_amount"`
	StartDate    time.Time `gorm:"not null" json:"start_date"`
	EndDate      time.Time `gorm:"not null" json:"end_date"`
	Status       string    `gorm:"type:varchar(50);default:'pending';index" json:"status"`
	Terms        string    `gorm:"type:text" json:"terms"`

	ClientCompleted     bool       `gorm:"default:false" json:"client_completed"`
	FreelancerCompleted bool       `gorm:"default:false" json:"freelancer_completed"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`

	Payments []Payment `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"payments,omitempty"`
}

type Payment struct {
	BaseModel
	ContractID       string     `gorm:"not null;index" json:"contract_id"`
	Contract         Contract   `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"contract,omitempty"`
	Amount           float64    `gorm:"type:decimal(10,2);not null" json:"amount"`
	Status           string     `gorm:"type:varchar(50);default:'pending';index" json:"status"`
	PaymentMethod    string     `gorm:"type:varchar(50)" json:"payment_method"`
	TransactionID    string     `gorm:"type:varchar(255);uniqueIndex" json:"transaction_id"`
	ProcessedAt      *time.Time `json:"processed_at,omitempty"`
	PlatformFee      float64    `gorm:"type:decimal(10,2);default:0" json:"platform_fee"`
	FreelancerAmount float64    `gorm:"type:decimal(10,2)" json:"freelancer_amount"`
}

func (Contract) TableName() string {
	return "contracts"
}

func (Payment) TableName() string {
	return "payments"
}

type Review struct {
	BaseModel
	ContractID string   `gorm:"not null;index:idx_review_contract_reviewer" json:"contract_id"`
	Contract   Contract `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"-"`
	// author
	ReviewerID string `gorm:"not null;index:idx_review_contract_reviewer" json:"reviewer_id"`
	Reviewer   User   `gorm:"foreignKey:ReviewerID;constraint:OnDelete:CASCADE" json:"reviewer"`
	// the user who get the review
	TargetID   string `gorm:"not null;index" json:"target_id"`
	TargetUser User   `gorm:"foreignKey:TargetID;constraint:OnDelete:CASCADE" json:"target_user"`
	Rating     int    `gorm:"not null;check:rating >= 1 AND rating <= 5" json:"rating"`
	Comment    string `gorm:"type:text" json:"comment"`
}

func (Review) TableName() string {
	return "reviews"
}
