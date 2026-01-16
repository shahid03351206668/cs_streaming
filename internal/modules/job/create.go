package job

import (
	"fmt"
	"mime/multipart"
	"slices"
	"time"

	"tasksy/models"
	aws_services "tasksy/pkg"
	"tasksy/pkg/logger"
	"tasksy/pkg/worker"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
	Thumbnail string `json:"thumbnail,omitempty"`
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

type JobPostData struct {
	CategoryID  string  `form:"category_id"`
	Title       string  `form:"title"`
	Description string  `form:"description"`
	Budget      float64 `form:"budget"`
	OpenBudget  bool    `form:"open_budget"`
	Address     string  `form:"address"`
}

type Service struct {
	db          gorm.DB
	s3Client    *aws_services.S3Client
	queueClient *asynq.Client
}

func NewService(db *gorm.DB, s3Client *aws_services.S3Client, queueClient *asynq.Client) Service {
	return Service{db: *db, s3Client: s3Client, queueClient: queueClient}
}

func (s *Service) CreateJobPost(user models.User, data JobPostData, media []*multipart.FileHeader) (*models.JobPost, error) {
	fmt.Println("test create job")
	jobPost := models.JobPost{
		CreatedByID: user.ID,
		CategoryID:  data.CategoryID,
		Title:       data.Title,
		Description: data.Description,
		Budget:      data.Budget,
		OpenBudget:  data.OpenBudget,
		Address:     data.Address,
		Status:      models.JobStatusProcessing,
	}

	tx := s.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&jobPost).Error; err != nil {
		logger.Log.Error("Failed to create job post", zap.Error(err))
		return nil, err
	}

	fmt.Println("media files")
	fmt.Println(media)
	for _, f := range media {
		fileType := f.Header.Get("Content-Type")
		fmt.Println("fileType", fileType)

		if slices.Contains([]string{"video/mp4", "video/mov", "video/quicktime"}, fileType) {
			rawKey := fmt.Sprintf("raw/%s/%s", jobPost.ID, f.Filename)
			file, err := f.Open()

			if err != nil {
				file.Close()
				tx.Rollback()
				logger.Log.Error("faild to open file", zap.Error(err))
				return nil, err
			}

			_, key, err := s.s3Client.UploadFileToBucket(file, rawKey, fileType, "tasksy-raw-media")
			if err != nil {
				file.Close()
				tx.Rollback()
				logger.Log.Error("faild to open file", zap.Error(err))

				return nil, err
			}

			task, _ := worker.NewVideoTranscodeTask(jobPost.ID, key, f.Filename, fileType, f.Size)
			if _, err := s.queueClient.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(10*time.Minute)); err != nil {
				return nil, err
			}

		} else {
			file, err := f.Open()
			fileUrl, key, err := s.s3Client.UploadFileToBucket(file, f.Filename, fileType, "tasksy-storage")
			if err != nil {
				file.Close()
				tx.Rollback()
				logger.Log.Error("faild to open file", zap.Error(err))
				return nil, err
			}

			if err := tx.Create(&models.JobMedia{
				JobID:     jobPost.ID,
				URL:       fileUrl,
				ObjectKey: key,
				FileName:  f.Filename,
				FileSize:  f.Size,
				MediaType: fileType,
			}).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &jobPost, nil
}
