package models

import "time"


const (
	DisputeStatusOpen     = "open"
	DisputeStatusResolved = "resolved"
	DisputeStatusClosed   = "closed"
)

const (
	JobStatusDraft      = "draft"
	JobStatusProcessing = "processing"
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
	CreatedBy   User     `gorm:"foreignKey:CreatedByID;constraint:OnDelete:CASCADE" json:"created_by"`
	Category    Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:SET NULL" json:"category"`
	CreatedByID string   `gorm:"type:string;not null;index" json:"created_by_id"`
	CategoryID  string   `gorm:"index" json:"category_id"`
	Title       string   `gorm:"type:varchar(255);not null" json:"title"`
	Description string   `gorm:"type:text;not null" json:"description"`
	Budget      float64  `gorm:"type:decimal(10,2)" json:"budget"`
	OpenBudget  bool     `gorm:"default:false" json:"open_budget"`
	Address     string   `gorm:"type:varchar(500)" json:"address"`
	Status          string           `gorm:"type:varchar(50);default:'open';index" json:"status"`
	JobMedia        []JobMedia       `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job_media,omitempty"`
	Proposals       []Proposal       `gorm:"foreignKey:JobPostID;constraint:OnDelete:CASCADE" json:"-"`
	JobPostLocation *JobPostLocation `gorm:"foreignKey:JobPostID" json:"location,omitempty"`
}

type JobMedia struct {
	BaseModel
	Thumbnail string  `gorm:"type:varchar(500)" json:"thumbnail"`
	ObjectKey string  `gorm:"type:varchar(200)" json:"s3_object_key"`
	JobID     string  `gorm:"not null;index" json:"job_id"`
	Job       JobPost `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"job,omitempty"`
	URL       string  `gorm:"type:varchar(500);" json:"url"`
	MediaType string  `gorm:"type:varchar(50);" json:"media_type"`
	FileName  string  `gorm:"type:varchar(255)" json:"file_name"`
	FileSize  int64   `gorm:"type:bigint" json:"file_size"`
}

type JobPostVideo struct {
	BaseModel

	JobMediaID string `gorm:"type:string;not null;uniqueIndex;constraint:OnDelete:CASCADE" json:"job_media_id"`
	JobMedia   JobMedia

	VideoURL     string `gorm:"type:varchar(500);not null" json:"video_url"`
	ThumbnailURL string `gorm:"type:varchar(500);not null" json:"thumbnail_url"`

	FileName string `gorm:"type:varchar(255);not null" json:"file_name"`
	FileSize int64  `gorm:"type:bigint;not null" json:"file_size"`

	Width  int `gorm:"not null" json:"width"`
	Height int `gorm:"not null" json:"height"`

	S3ObjectKey string `gorm:"type:varchar(255);not null" json:"s3_object_key"`
}

type JobPostLocation struct {
	JobPostID  string  `gorm:"type:string;not null" json:"job_post_id"`
	Latitude   float64 `gorm:"type:decimal(11,8);not null" json:"latitude"`
	Longitude  float64 `gorm:"type:decimal(11,8);not null" json:"longitude"`
	PostalCode string  `json:"pincode"`
	Street     string  `json:"street"`
	City       string  `gorm:"not null" json:"city"`
	State      string  `gorm:"not null" json:"state"`
	Country    string  `gorm:"not null" json:"country"`
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

type Contract struct {
	BaseModel
	JobPostID           string     `gorm:"not null;index" json:"job_post_id"`
	JobPost             JobPost    `gorm:"foreignKey:JobPostID;constraint:OnDelete:CASCADE" json:"job_post"`
	ProposalID          string     `gorm:"not null;uniqueIndex" json:"proposal_id"`
	Proposal            Proposal   `gorm:"foreignKey:ProposalID;constraint:OnDelete:CASCADE" json:"proposal"`
	ClientID            string     `gorm:"not null;index" json:"client_id"`
	Client              User       `gorm:"foreignKey:ClientID;constraint:OnDelete:CASCADE" json:"client"`
	FreelancerID        string     `gorm:"not null;index" json:"freelancer_id"`
	Freelancer          User       `gorm:"foreignKey:FreelancerID;constraint:OnDelete:CASCADE" json:"freelancer"`
	Title               string     `gorm:"type:varchar(255);not null" json:"title"`
	Description         string     `gorm:"type:text" json:"description"`
	TotalAmount         float64    `gorm:"type:decimal(10,2);not null" json:"total_amount"`
	StartDate           time.Time  `gorm:"not null" json:"start_date"`
	EndDate             time.Time  `gorm:"not null" json:"end_date"`
	Status              string     `gorm:"type:varchar(50);default:'pending';index" json:"status"`
	Terms               string     `gorm:"type:text" json:"terms"`
	CommissionRate      float64    `gorm:"type:decimal(5,2);default:0" json:"commission_rate"`
	ClientCompleted     bool       `gorm:"default:false" json:"client_completed"`
	FreelancerCompleted bool       `gorm:"default:false" json:"freelancer_completed"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`

	// Escrow fields
	EscrowPaymentIntentID string `gorm:"type:varchar(100)" json:"escrow_payment_intent_id,omitempty"`
	EscrowStatus          string `gorm:"type:varchar(20);default:'pending'" json:"escrow_status"`
	EscrowAmount          int64  `gorm:"default:0" json:"escrow_amount"`

	// Payments []Payment `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"payments,omitempty"`
	Reviews  []Review  `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"reviews,omitempty"`
	Disputes []Dispute `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"disputes,omitempty"`
}

func (Contract) TableName() string {
	return "contracts"
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

type Dispute struct {
	BaseModel
	ContractID  string   `gorm:"not null;index" json:"contract_id"`
	Contract    Contract `gorm:"foreignKey:ContractID;constraint:OnDelete:CASCADE" json:"contract,omitempty"`
	FiledByID   string   `gorm:"not null;index" json:"filed_by_id"`
	FiledBy     User     `gorm:"foreignKey:FiledByID;constraint:OnDelete:CASCADE" json:"filed_by"`
	// FiledByRole stores whether the filer is "client" or "freelancer" on the contract
	FiledByRole string `gorm:"type:varchar(20);not null;default:'client'" json:"filed_by_role"`
	Reason      string `gorm:"type:varchar(255);not null" json:"reason"`
	Description string `gorm:"type:text;not null" json:"description"`
	Status      string `gorm:"type:varchar(50);default:'open';index" json:"status"`
	// Resolution fields (populated by admin)
	ResolvedByID *string    `gorm:"type:string;index" json:"resolved_by_id,omitempty"`
	ResolvedBy   *User      `gorm:"foreignKey:ResolvedByID" json:"resolved_by,omitempty"`
	Resolution   string     `gorm:"type:text" json:"resolution,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	// Attachments are stored in the files table via EntityID=dispute.ID, EntityType="disputes".
	// No FK constraint is created because the files table is shared across multiple entity types.
	Attachments []File `gorm:"foreignKey:EntityID;references:ID" json:"attachments,omitempty"`
}

func (Dispute) TableName() string {
	return "disputes"
}
