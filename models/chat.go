package models

import "time"

const (
	MessageTypeText   = "text"
	MessageTypeAudio  = "audio"
	MessageTypeImage  = "image"
	MessageTypeFile   = "file"
	MessageTypeSystem = "system"
)

type ChatConversation struct {
	BaseModel

	JobPostID   string        `gorm:"index" json:"jobpost_id"`
	JobPost     *JobPost      `gorm:"foreignKey:JobPostID" json:"jobpost,omitempty"`
	LastMessage string        `gorm:"type:text" json:"last_message"`
	LastSentAt  time.Time     `json:"last_sent_at"`
	Messages    []ChatMessage `gorm:"foreignKey:ConversationID" json:"-"`

	Participants []ChatParticipant `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE" json:"participants"`
}

type ChatParticipant struct {
	BaseModel
	ConversationID string    `gorm:"not null;index:idx_chat_user" json:"conversation_id"`
	UserID         string    `gorm:"not null;index:idx_chat_user" json:"user_id"`
	User           User      `gorm:"foreignKey:UserID" json:"user"`
	JoinedAt       time.Time `json:"joined_at"`
	UnreadCount    int       `gorm:"default:0" json:"unread_count"`
	IsAdmin        bool      `gorm:"default:false" json:"is_admin"`
}

type ChatMessage struct {
	BaseModel

	ConversationID string `gorm:"not null;index" json:"conversation_id"`
	SenderID       string `gorm:"not null;index" json:"sender_id"`
	Sender         User   `gorm:"foreignKey:SenderID" json:"sender,omitempty"`

	Content string `gorm:"type:text;not null;default:''" json:"content"`
	IsRead  bool   `gorm:"default:false" json:"is_read"`

	Type        string           `gorm:"type:varchar(20);default:'text'" json:"type"`
	Attachments []ChatAttachment `gorm:"foreignKey:MessageID;constraint:OnDelete:CASCADE" json:"attachments"`
}
type ChatAttachment struct {
	BaseModel
	MessageID string `gorm:"not null;index" json:"message_id"`
	URL       string `gorm:"type:varchar(500);not null" json:"url"`      // S3/Cloudinary URL
	FileType  string `gorm:"type:varchar(20);not null" json:"file_type"` // e.g., 'image', 'audio', 'pdf'
	MimeType  string `gorm:"type:varchar(100)" json:"mime_type"`         // e.g., 'image/jpeg', 'audio/mpeg'
	FileName  string `gorm:"type:varchar(255)" json:"file_name"`         // Original filename
	FileSize  int64  `gorm:"type:bigint" json:"file_size"`               // Size in bytes
	Duration  int    `gorm:"default:0" json:"duration"`                  // Seconds (Only for Audio/Video)
}

func (ChatConversation) TableName() string {
	return "chat_conversation"
}

func (ChatMessage) TableName() string {
	return "chat_message"
}

func (ChatParticipant) TableName() string {
	return "chat_participants"
}

func (ChatAttachment) TableName() string {
	return "chat_attachments"
}
