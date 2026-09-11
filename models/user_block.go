package models

type UserBlock struct {
	BaseModel
	BlockerID string `gorm:"type:string;not null;index;uniqueIndex:idx_user_block_unique" json:"blocker_id"`
	Blocker   User   `gorm:"foreignKey:BlockerID;constraint:OnDelete:CASCADE" json:"-"`
	BlockedID string `gorm:"type:string;not null;index;uniqueIndex:idx_user_block_unique" json:"blocked_id"`
	Blocked   User   `gorm:"foreignKey:BlockedID;constraint:OnDelete:CASCADE" json:"blocked,omitempty"`
}

func (UserBlock) TableName() string {
	return "user_blocks"
}
