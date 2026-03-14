package job

import (
	"context"
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
	DistanceKM  *float64     `json:"distance_km,omitempty"`
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

	// job location fields for job job post

	City       string  `form:"city"`
	State      string  `form:"state"`
	Latitude   float64 `form:"latitude"`
	Longitude  float64 `form:"longitude"`
	PostalCode string  `form:"postalcode"`
	Street     string  `form:"street"`
	Country    string  `form:"country"`
}

type Service struct {
	db          gorm.DB
	s3Client    *aws_services.S3Client
	queueClient *asynq.Client
	notifier    JobNotifier
	feedCache   *jobFeedCache
}

type JobNotifier interface {
	NotifyNewJobPostedToInterestedUsers(ctx context.Context, job *models.JobPost, loc *models.JobPostLocation) error
}

func NewService(db *gorm.DB, s3Client *aws_services.S3Client, queueClient *asynq.Client, notifier JobNotifier) Service {
	return Service{db: *db, s3Client: s3Client, queueClient: queueClient, notifier: notifier, feedCache: newJobFeedCache(jobFeedCacheTTL)}
}

func (s *Service) InvalidateJobFeedCache() {
	if s.feedCache == nil {
		return
	}
	s.feedCache.Clear()
}

func (s *Service) CreateJobPost(user models.User, data JobPostData, media []*multipart.FileHeader) (*models.JobPost, error) {
	fmt.Println("test create job")

	var videos []*multipart.FileHeader
	var images []*multipart.FileHeader

	for _, f := range media {
		fileType := f.Header.Get("Content-Type")
		if slices.Contains([]string{"video/mp4", "video/mov", "video/quicktime"}, fileType) {
			videos = append(videos, f)

		} else if slices.Contains([]string{"image/jpeg", "image/webp", "image/png"}, fileType) {
			images = append(images, f)
		}
	}

	initialStatus := models.JobStatusOpen
	if len(videos) > 0 {
		initialStatus = models.JobStatusProcessing
	}

	jobPost := models.JobPost{
		CreatedByID: user.ID,
		CategoryID:  data.CategoryID,
		Title:       data.Title,
		Description: data.Description,
		Budget:      data.Budget,
		OpenBudget:  data.OpenBudget,
		Address:     data.Address,
		Status:      initialStatus,
	}

	type uploadedMedia struct {
		URL       string
		Key       string
		FileName  string
		FileSize  int64
		MediaType string
	}

	var uploadedFiles []uploadedMedia
	for _, f := range images {

		fileType := f.Header.Get("Content-Type")
		file, err := f.Open()

		if err != nil {
			logger.Log.Error("failed to open file", zap.Error(err))
			return nil, err
		}

		fileUrl, key, err := s.s3Client.UploadFileToBucket(file, f.Filename, fileType, "tasksy-storage")
		file.Close()

		if err != nil {
			logger.Log.Error("failed to upload file to s3 bucket", zap.Error(err))
			return nil, err
		}

		uploadedFiles = append(uploadedFiles, uploadedMedia{
			URL:       fileUrl,
			Key:       key,
			FileName:  f.Filename,
			FileSize:  f.Size,
			MediaType: fileType,
		})
	}

	tx := s.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&jobPost).Error; err != nil {
		logger.Log.Error("Failed to create job post", zap.Error(err))
		tx.Rollback()
		return nil, err
	}

	for _, uploaded := range uploadedFiles {
		if err := tx.Create(&models.JobMedia{
			JobID:     jobPost.ID,
			URL:       uploaded.URL,
			ObjectKey: uploaded.Key,
			FileName:  uploaded.FileName,
			FileSize:  uploaded.FileSize,
			MediaType: uploaded.MediaType,
		}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	jobLocation := models.JobPostLocation{
		JobPostID:  jobPost.ID,
		City:       data.City,
		State:      data.State,
		Latitude:   data.Latitude,
		Longitude:  data.Longitude,
		PostalCode: data.PostalCode,
		Street:     data.Street,
		Country:    data.Country,
	}

	if err := tx.Create(&jobLocation).Error; err != nil {
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// New job posts affect the public feed; ensure cached feed pages refresh quickly.
	s.InvalidateJobFeedCache()

	if s.notifier != nil && jobPost.Status == models.JobStatusOpen {
		go func(post models.JobPost, loc models.JobPostLocation) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.notifier.NotifyNewJobPostedToInterestedUsers(ctx, &post, &loc)
		}(jobPost, jobLocation)
	}

	// Upload video files and queue for processing (after transaction commits)
	for _, f := range videos {
		fileType := f.Header.Get("Content-Type")
		rawKey := fmt.Sprintf("raw/%s/%s", jobPost.ID, f.Filename)
		file, err := f.Open()

		if err != nil {
			logger.Log.Error("failed to open file", zap.Error(err))
			return nil, err
		}

		_, key, err := s.s3Client.UploadFileToBucket(file, rawKey, fileType, "tasksy-raw-media")
		file.Close()

		if err != nil {
			logger.Log.Error("failed to upload file to s3 bucket", zap.Error(err))
			return nil, err
		}

		task, _ := worker.NewVideoTranscodeTask(jobPost.ID, key, f.Filename, fileType, f.Size)

		fmt.Println("sending video files into queue")
		if _, err := s.queueClient.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(10*time.Minute)); err != nil {
			logger.Log.Error("error while adding task into queue", zap.Error(err))
			return nil, err
		}
	}

	return &jobPost, nil
}
