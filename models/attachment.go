package models

type File struct {
	BaseModel

	URL        string `json:"url"`
	FileName   string `json:"file_name"`
	FileSize   int64  `json:"file_size"`
	FileType   string `json:"media_type"`
	ObjectKey  string `json:"object_key"`
	EntityID   string `gorm:"index" json:"entity_id"`
	EntityType string `gorm:"index" json:"entity_type"`
}

func (File) TableName() string {
	return "files"
}
