package job

import (
	"gorm.io/gorm"
	"strings"
	"tasksy/models"
	"time"
)


type TypeCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type TypeMedia struct {
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name"`
	FileSize  int64  `json:"file_size"`
}
type TypeUser struct {
	ID           string `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Email        string `json:"email"`
	PhoneNumber  string `json:"phone_number"`
	ProfilePhoto string `json:"profile_photo,omitempty"`
}

type JobPostValue struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Budget      float64      `json:"budget"`
	OpenBudget  bool         `json:"open_budget"`
	Address     string       `json:"address"`
	Status      string       `json:"status"`
	CreatedBy   TypeUser     `json:"created_by"`
	Category    TypeCategory `json:"category"`
	Media       []TypeMedia  `json:"media"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Service struct {
	db gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: *db}
}

func (s *Service) GetJobFeed(category, searchQuery string, page, limit int) ([]JobPostValue, int64, error) {
	var jobResults []models.JobPost

	var total int64
	jobQuery := s.db.Model(&models.JobPost{}).Where("status = ?", models.JobStatusOpen)

	if category != "" {
		jobQuery = jobQuery.Where("category_id = ?", category)
	}

	if searchQuery != "" {
		formattedSearch := "%" + strings.TrimSpace(searchQuery) + "%"
		jobQuery = jobQuery.Where("title ILIKE ? OR description ILIKE ?", formattedSearch, formattedSearch)
	}

	if err := jobQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := jobQuery.
		Preload("CreatedBy").
		Preload("Category").
		Preload("JobMedia").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&jobResults).Error; err != nil {
		return nil, 0, err
	}

	jobsArray := make([]JobPostValue, 0, len(jobResults))
	for _, post := range jobResults {
		mediaList := make([]TypeMedia, 0, len(post.JobMedia))
		for _, m := range post.JobMedia {
			mediaList = append(mediaList, TypeMedia{
				URL:       m.URL,
				MediaType: m.MediaType,
				FileName:  m.FileName,
				FileSize:  m.FileSize,
			})
		}

		creator := TypeUser{
			ID:           post.CreatedBy.ID,
			FirstName:    post.CreatedBy.FirstName,
			LastName:     post.CreatedBy.LastName,
			Email:        post.CreatedBy.Email,
			PhoneNumber:  post.CreatedBy.PhoneNumber,
			ProfilePhoto: post.CreatedBy.ProfilePhoto,
		}

		cat := TypeCategory{
			ID:   post.Category.ID,
			Name: post.Category.Name,
		}

		jobsArray = append(jobsArray, JobPostValue{
			ID:          post.ID,
			Title:       post.Title,
			Description: post.Description,
			Budget:      post.Budget,
			OpenBudget:  post.OpenBudget,
			Address:     post.Address,
			Status:      post.Status,
			CreatedBy:   creator,
			Category:    cat,
			Media:       mediaList,
			CreatedAt:   post.CreatedAt,
			UpdatedAt:   post.UpdatedAt,
		})
	}

	return jobsArray, total, nil
}
