package controllers

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const ALLOWED_JOB_POST_MEDIA int = 6
const MEDIA_FILE_PATH string = "media/"

func getMediaType(file *multipart.FileHeader) string {
	ext := strings.ToLower(filepath.Ext(file.Filename))

	imageExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true}
	videoExts := map[string]bool{".mp4": true, ".mov": true, ".avi": true}
	docExts := map[string]bool{".pdf": true, ".doc": true, ".docx": true}

	switch {
	case imageExts[ext]:
		return "image"
	case videoExts[ext]:
		return "video"
	case docExts[ext]:
		return "document"
	default:
		return "other"
	}
}

type JobMediaResponse struct {
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name"`
	FileSize  int64  `json:"file_size"`
}

type UserResponse struct {
	ID           string `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Email        string `json:"email"`
	PhoneNumber  string `json:"phone_number"`
	ProfilePhoto string `json:"profile_photo,omitempty"`
}

type CategoryResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type JobPostResponse struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Budget      float64            `json:"budget"`
	OpenBudget  bool               `json:"open_budget"`
	Address     string             `json:"address"`
	Status      string             `json:"status"`
	CreatedBy   UserResponse       `json:"created_by"`
	Category    CategoryResponse   `json:"category"`
	Media       []JobMediaResponse `json:"media"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

func serializeJobPost(job models.JobPost) JobPostResponse {
	media := make([]JobMediaResponse, 0, len(job.JobMedia))
	for _, m := range job.JobMedia {
		media = append(media, JobMediaResponse{
			URL:       m.URL,
			MediaType: m.MediaType,
			FileName:  m.FileName,
			FileSize:  m.FileSize,
		})
	}

	user := UserResponse{
		ID:           fmt.Sprintf("%v", job.CreatedBy.ID),
		FirstName:    job.CreatedBy.FirstName,
		LastName:     job.CreatedBy.LastName,
		Email:        job.CreatedBy.Email,
		PhoneNumber:  job.CreatedBy.PhoneNumber,
		ProfilePhoto: job.CreatedBy.ProfilePhoto,
	}

	// Serialize category
	category := CategoryResponse{
		ID:   fmt.Sprintf("%v", job.Category.ID),
		Name: job.Category.Name,
	}

	return JobPostResponse{
		ID:          fmt.Sprintf("%v", job.ID),
		Title:       job.Title,
		Description: job.Description,
		Budget:      job.Budget,
		OpenBudget:  job.OpenBudget,
		Address:     job.Address,
		Status:      job.Status,
		CreatedBy:   user,
		Category:    category,
		Media:       media,
		CreatedAt:   job.CreatedAt,
		UpdatedAt:   job.UpdatedAt,
	}
}

func GetJobDetail(c *gin.Context) {
	var job models.JobPost

	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "id is missing in the query parmas",
			"message": "error",
		})
		return
	}

	result := db.DB.
		Preload("CreatedBy").
		Preload("Category").
		Preload("JobMedia").
		Where("id = ?", id).First(&job)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   result.Error.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    serializeJobPost(job),
	})
}

func GetJobs(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")
	status := c.Query("status")
	categoryID := c.Query("category_id")

	var jobs []models.JobPost
	query := db.DB.Preload("CreatedBy").Preload("Category").Preload("JobMedia")

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	var total int64
	query.Model(&models.JobPost{}).Count(&total)

	offset := 0
	if page != "1" {
		offset = (10 * (int(page[0]) - '0')) - 10
	}

	if err := query.Offset(offset).Limit(10).Order("created_at DESC").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	serializedJobs := make([]JobPostResponse, 0, len(jobs))
	for _, job := range jobs {
		serializedJobs = append(serializedJobs, serializeJobPost(job))
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    serializedJobs,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

type CreateJobRequest struct {
	CategoryID  string  `form:"category_id" binding:"required"`
	Title       string  `form:"title" binding:"required,min=3,max=255"`
	Description string  `form:"description" binding:"required,min=10"`
	Budget      float64 `form:"budget" binding:"omitempty,min=0"`
	OpenBudget  bool    `form:"open_budget"`
	Address     string  `form:"address" binding:"omitempty,max=500"`
}

func CreateJob(c *gin.Context) {
	var DB = *db.DB

	user, exists := lib.GetUser(c)

	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "error",
			"error":   "provide a valid authorization token",
		})
		return
	}

	var body CreateJobRequest
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	var category models.Category
	if err := DB.Where("id = ?", body.CategoryID).First(&category).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "invalid category",
		})
		return
	}

	tx := DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	jobPost := models.JobPost{
		CreatedByID: user.ID,
		CategoryID:  category.ID,
		Title:       body.Title,
		Description: body.Description,
		Budget:      body.Budget,
		OpenBudget:  body.OpenBudget,
		Address:     body.Address,
		Status:      "open",
	}

	if err := tx.Create(&jobPost).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	form, err := c.MultipartForm()
	if err == nil && form != nil && form.File["media"] != nil {
		files := form.File["media"]
		if len(files) > ALLOWED_JOB_POST_MEDIA {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "error",
				"error":   fmt.Sprintf("Maximum %d files allowed", ALLOWED_JOB_POST_MEDIA),
			})
			return
		}

		// Ensure media directory exists
		if err := os.MkdirAll(MEDIA_FILE_PATH, 0755); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "error",
				"error":   fmt.Sprintf("failed to create media directory: %v", err),
			})
			return
		}

		for _, file := range files {
			// Generate unique filename with proper extension
			ext := filepath.Ext(file.Filename)
			baseFileName := strings.TrimSuffix(file.Filename, ext)
			baseFileName = filepath.Base(baseFileName)
			fileName := fmt.Sprintf("%s_%d%s", baseFileName, time.Now().UnixNano(), ext)
			filePath := filepath.Join(MEDIA_FILE_PATH, fileName)

			fmt.Println(filePath)

			if err := c.SaveUploadedFile(file, filePath); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "error",
					"error":   fmt.Sprintf("failed to upload file: %v", err),
				})
				return
			}

			jobMedia := models.JobMedia{
				JobID:     jobPost.ID,
				URL:       filePath,
				MediaType: getMediaType(file),
				FileName:  file.Filename,
				FileSize:  file.Size,
			}

			if err := tx.Create(&jobMedia).Error; err != nil {
				tx.Rollback()
				os.Remove(filePath)
				c.JSON(http.StatusInternalServerError, gin.H{
					"message": "error",
					"error":   "failed to save media record",
				})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to commit transaction",
		})
		return
	}

	var createdJob models.JobPost
	if err := DB.Preload("CreatedBy").Preload("Category").Preload("JobMedia").
		Where("id = ?", jobPost.ID).First(&createdJob).Error; err != nil {
		c.JSON(http.StatusCreated, gin.H{
			"message": "created",
			"job_id":  fmt.Sprintf("%v", jobPost.ID),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "created",
		"data":    serializeJobPost(createdJob),
	})
}

func UpdateJob(c *gin.Context) {
	user, exists := lib.GetUser(c)
	id := c.Param("id")

	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "error",
			"error":   "invalid user",
		})
		return
	}

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "job ID is required",
		})
		return
	}

	// Find the job post - FIXED: Correct GORM syntax
	jobPost := models.JobPost{}
	if err := db.DB.Where("id = ?", id).First(&jobPost).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "job post not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch job post: " + err.Error(),
		})
		return
	}

	// Check if user is the creator of the job post
	if jobPost.CreatedByID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "you are not authorized to update this job post",
		})
		return
	}

	var body struct {
		CategoryID  string  `form:"category_id"`
		Title       string  `form:"title"`
		Description string  `form:"description"`
		Budget      float64 `form:"budget"`
		OpenBudget  bool    `form:"open_budget"`
		Address     string  `form:"address"`
		Status      string  `form:"status"`
	}

	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	// Validate status if provided
	if body.Status != "" {
		validStatuses := []string{
			models.JobStatusDraft,
			models.JobStatusOpen,
			models.JobStatusInProgress,
			models.JobStatusCompleted,
			models.JobStatusCancelled,
			models.JobStatusClosed,
			models.JobStatusOnHold,
		}
		isValidStatus := false
		for _, status := range validStatuses {
			if body.Status == status {
				isValidStatus = true
				break
			}
		}
		if !isValidStatus {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "error",
				"error":   "invalid status value",
			})
			return
		}
	}

	// Validate category if provided
	if body.CategoryID != "" {
		category := models.Category{}
		if err := db.DB.Where("id = ?", body.CategoryID).First(&category).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   "category not found",
				})
				return
			}
		}
		// Check if category is disabled
		if category.Disable {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "error",
				"error":   "selected category is disabled",
			})
			return
		}
	}

	// Prepare updates map
	updates := make(map[string]interface{})

	if body.Title != "" {
		updates["title"] = body.Title
	}

	if body.Description != "" {
		updates["description"] = body.Description
	}

	if body.Budget != 0 {
		updates["budget"] = body.Budget
	}

	// Always update OpenBudget if it's part of the request
	updates["open_budget"] = body.OpenBudget

	if body.Address != "" {
		updates["address"] = body.Address
	}

	if body.Status != "" {
		// Prevent changing status of completed jobs
		if jobPost.Status == models.JobStatusCompleted && body.Status != models.JobStatusCompleted {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "error",
				"error":   "cannot change status of a completed job",
			})
			return
		}

		// Uncomment when Contract model is implemented
		// if body.Status == models.JobStatusCancelled {
		// 	var activeContracts int64
		// 	db.DB.Model(&models.Contract{}).
		// 		Where("job_post_id = ? AND status = ?", id, models.ContractStatusActive).
		// 		Count(&activeContracts)

		// 	if activeContracts > 0 {
		// 		c.JSON(http.StatusBadRequest, gin.H{
		// 			"message": "error",
		// 			"error":   "cannot cancel job with active contracts",
		// 		})
		// 		return
		// 	}
		// }

		updates["status"] = body.Status
	}

	if body.CategoryID != "" {
		updates["category_id"] = body.CategoryID
	}

	// Check if there are any updates to apply
	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "no valid fields to update",
		})
		return
	}

	// Update the job post
	if err := db.DB.Model(&jobPost).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to update job post: " + err.Error(),
		})
		return
	}

	// jobResponse =
	if err := db.DB.Preload("Category").
		Preload("CreatedBy").
		Preload("JobMedia").
		Where("id = ?", id).
		First(&jobPost).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to reload job post: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    serializeJobPost(jobPost),
	})
}

func CreateContract(c *gin.Context) {
	user, exists := lib.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"message": "error",
			"error":   "invalid user",
		})
		return
	}
	var body struct {
		ProposalID  string    `json:"proposal_id" binding:"required"`
		Title       string    `json:"title" binding:"required"`
		Description string    `json:"description"`
		TotalAmount float64   `json:"total_amount" binding:"required,gt=0"`
		StartDate   time.Time `json:"start_date" binding:"required"`
		EndDate     time.Time `json:"end_date" binding:"required"`
		Terms       string    `json:"terms"`
	}

	if body.EndDate.Before(body.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "End date must be after start date"})
		return
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var proposal models.Proposal
	if err := db.DB.Preload("JobPost").Where("id = ?", body.ProposalID).First(&proposal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "proposal not found",
			"message": "error",
		})
		return
	}

	if proposal.JobPost.CreatedBy.ID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only job creator can create contract", "message": "error"})
		return
	}

	if proposal.Status != models.ProposalStatusAccepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only accepted proposals can have contracts",
			"message": "error"})
	}

	var existingContract models.Contract

	if err := db.DB.Where(&existingContract, "proposal_id = ?", proposal.ID).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Contract already exists for this proposal",
			"message": "error"})
		return
	}

	contract := models.Contract{
		JobPostID:    proposal.JobPost.ID,
		ProposalID:   proposal.ID,
		ClientID:     proposal.JobPost.CreatedBy.ID,
		FreelancerID: proposal.FreelancerID,
		Title:        body.Title,
		Description:  body.Description,
		TotalAmount:  body.TotalAmount,
		StartDate:    body.StartDate,
		EndDate:      body.EndDate,
		Status:       models.ContractStatusPending,
		Terms:        body.Terms,
	}
	if err := db.DB.Create(&contract).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to create contract: " + err.Error(),
		})

		return
	}

	db.DB.Model(&proposal.JobPost).Update("status", models.JobStatusInProgress)
	db.DB.Preload("JobPost").Preload("Proposal").Preload("Client").Preload("Freelancer").
		First(&contract, contract.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Contract created successfully",
		"contract": contract,
	})

}

func GetContracts(c *gin.Context) {
	var job_id string = c.Query("job_id")

	if job_id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "job_id query param is required",
		})
		return
	}

	var contract models.Contract
	if err := db.DB.Where(&contract, "job_post_id = ?", job_id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    contract,
	})
}
