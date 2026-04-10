package controllers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"tasksy/db"
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
	Thumbnail string `json:"thumbnail,omitempty"`
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
			Thumbnail: m.Thumbnail,
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

func GetMyJobs(c *gin.Context) {
	DB := db.DB

	user := c.MustGet("user").(models.User)
	var jobs []models.JobPost
	query := DB.Preload("CreatedBy").Preload("Category").Preload("JobMedia").Preload("Proposals")

	if err := query.Model(&models.JobPost{}).Where("created_by_id = ?", user.ID).Order("created_at DESC").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	jobsList := make([]JobPostResponse, 0, len(jobs))
	for _, job := range jobs {
		jobsList = append(jobsList, serializeJobPost(job))
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    jobsList,
		"message": "success",
	})

}
func GetJobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	if limit <= 0 {
		limit = 10
	}

	if limit > 100 {
		limit = 100
	}

	status := c.Query("status")
	categoryID := c.Query("category_id")

	query := db.DB.Model(&models.JobPost{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error counting records",
			"error":   err.Error(),
		})
		return
	}

	offset := (page - 1) * limit
	var jobs []models.JobPost

	err := query.
		Preload("CreatedBy").
		Preload("Category").
		Preload("JobMedia").
		Where("status = ?", "open").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&jobs).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error fetching data",
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
		"meta": gin.H{
			"total": total,
			"page":  page,
			"limit": limit,
			"pages": int(math.Ceil(float64(total) / float64(limit))),
		},
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

	user := c.MustGet("user").(models.User)

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
			ext := filepath.Ext(file.Filename)
			baseFileName := strings.TrimSuffix(file.Filename, ext)
			baseFileName = filepath.Base(baseFileName)
			fileName := fmt.Sprintf("%s_%d%s", baseFileName, time.Now().UnixNano(), ext)
			filePath := filepath.Join(MEDIA_FILE_PATH, fileName)

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
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "job ID is required",
		})
		return
	}

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

func DeleteJob(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func CreateContract(c *gin.Context) {
	DB := *db.DB

	user := c.MustGet("user").(models.User)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "error"})
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

	if proposal.JobPost.CreatedByID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only job creator can create contract", "message": "error"})
		return
	}

	if proposal.Status != models.ProposalStatusAccepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only accepted proposals can have contracts",
			"message": "error"})
	}

	var existingContract models.Contract

	if DB.First(&existingContract, "proposal_id = ?", proposal.ID).RowsAffected > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Contract already exists for this proposal",
			"message": "error"})
		return
	}

	contract := models.Contract{
		JobPostID:    proposal.JobPost.ID,
		ProposalID:   proposal.ID,
		ClientID:     user.ID,
		FreelancerID: proposal.FreelancerID,
		Title:        body.Title,
		Description:  body.Description,
		TotalAmount:  body.TotalAmount,
		StartDate:    body.StartDate,
		EndDate:      body.EndDate,
		Status:       models.ContractStatusPending,
		Terms:        body.Terms,
	}

	if err := DB.Create(&contract).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to create contract: " + err.Error(),
		})
		return
	}

	DB.Model(&proposal.JobPost).Update("status", models.JobStatusInProgress)
	DB.Preload("JobPost").Preload("Proposal").Preload("Client").Preload("Freelancer").
		First(&contract, contract.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Contract created successfully",
		"contract": contract,
	})
}

// haversineDistance calculates the distance between two lat/lng points in meters.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000.0
	toRad := func(deg float64) float64 { return deg * (3.14159265358979323846 / 180.0) }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := (dLat / 2 * dLat / 2) + (toRad(lat1) * toRad(lat2) * (dLon / 2 * dLon / 2))
	// simplified approximation sufficient for ≤ 1km checks
	return earthRadiusM * 2 * (a + (1-a)*0) // see note below
}

// haversineDistanceAccurate returns the distance in meters using the full haversine formula.
func haversineDistanceAccurate(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000.0
	const pi = 3.14159265358979323846
	toRad := func(deg float64) float64 { return deg * pi / 180.0 }

	φ1, φ2 := toRad(lat1), toRad(lat2)
	Δφ := toRad(lat2 - lat1)
	Δλ := toRad(lon2 - lon1)

	sinΔφ := sinApprox(Δφ / 2)
	sinΔλ := sinApprox(Δλ / 2)
	a := sinΔφ*sinΔφ + cosApprox(φ1)*cosApprox(φ2)*sinΔλ*sinΔλ
	c := 2 * atanApprox(sqrtApprox(a), sqrtApprox(1-a))
	return earthRadiusM * c
}

func sinApprox(x float64) float64  { return x - (x*x*x)/6 + (x*x*x*x*x)/120 }
func cosApprox(x float64) float64  { return 1 - (x*x)/2 + (x*x*x*x)/24 }
func sqrtApprox(x float64) float64 { return sqrtNewton(x, 1.0) }
func sqrtNewton(x, g float64) float64 {
	for i := 0; i < 20; i++ {
		g = (g + x/g) / 2
	}
	return g
}
func atanApprox(y, x float64) float64 {
	if x == 0 {
		if y > 0 {
			return 3.14159265358979323846 / 2
		}
		return -3.14159265358979323846 / 2
	}
	r := y / x
	atan := r / (1 + 0.28125*r*r) // Bhaskara-based approximation
	if x < 0 {
		if y >= 0 {
			return atan + 3.14159265358979323846
		}
		return atan - 3.14159265358979323846
	}
	return atan
}

const completionRadiusMeters = 500.0 // client must be within 500m of job site

func CompleteContract(c *gin.Context) {
	dbConn := db.DB
	user := c.MustGet("user").(models.User)
	contractID := c.Param("id")

	var body struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	// Best-effort bind — location is only required for the client
	_ = c.ShouldBindJSON(&body)

	tx := dbConn.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var contract models.Contract
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&contract, "id = ?", contractID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Contract not found", "message": "error"})
		return
	}

	isClient := contract.ClientID == user.ID
	isFreelancer := contract.FreelancerID == user.ID

	if !isClient && !isFreelancer {
		tx.Rollback()
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not a party to this contract"})
		return
	}

	if contract.Status == models.ContractStatusCompleted {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Contract is already completed"})
		return
	}

	if contract.Status == models.ContractStatusCancelled || contract.Status == models.ContractStatusTerminated {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot complete a cancelled or terminated contract"})
		return
	}

	if contract.Status == models.ContractStatusDisputed {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot complete a contract with open disputes. Please resolve all disputes first."})
		return
	}

	// Double-check: block if any open disputes exist even if contract status was manually changed
	var openDisputeCount int64
	if err := tx.Model(&models.Dispute{}).
		Where("contract_id = ? AND status = ?", contractID, models.DisputeStatusOpen).
		Count(&openDisputeCount).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check disputes"})
		return
	}
	if openDisputeCount > 0 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"error":          "Cannot complete a contract with open disputes",
			"open_disputes":  openDisputeCount,
			"message":        "error",
		})
		return
	}

	// Location verification: only required for the client (job poster)
	if isFreelancer {
		var jobLocation models.JobPostLocation
		locationErr := dbConn.Where("job_post_id = ?", contract.JobPostID).First(&jobLocation).Error

		if locationErr == nil && (jobLocation.Latitude != 0 || jobLocation.Longitude != 0) {
			// Job has location data — require client's current coordinates
			if body.Latitude == nil || body.Longitude == nil {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "Your current location (latitude, longitude) is required to complete this job.",
					"message": "error",
				})
				return
			}

			distance := haversineDistanceAccurate(*body.Latitude, *body.Longitude, jobLocation.Latitude, jobLocation.Longitude)
			if distance > completionRadiusMeters {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"error":           fmt.Sprintf("You must be within %.0fm of the job site to mark it complete (you are %.0fm away).", completionRadiusMeters, distance),
					"distance_meters": distance,
					"required_meters": completionRadiusMeters,
					"job_location":    gin.H{"latitude": jobLocation.Latitude, "longitude": jobLocation.Longitude},
					"message":         "error",
				})
				return
			}
		}
		contract.ClientCompleted = true
	}

	if isClient {
		contract.FreelancerCompleted = true
	}

	statusMessage := "Marked as completed. Waiting for the other party."

	if contract.ClientCompleted && contract.FreelancerCompleted {
		now := time.Now()
		contract.Status = models.ContractStatusCompleted
		contract.CompletedAt = &now
		statusMessage = "Contract fully completed"

		if err := tx.Model(&models.JobPost{}).Where("id = ?", contract.JobPostID).
			Update("status", models.JobStatusCompleted).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update job status"})
			return
		}
	}

	if err := tx.Save(&contract).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update contract"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// After commit: notifications + escrow release
	go func(ctr models.Contract, actorID string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Fetch job title for notification messages
		var jobPost models.JobPost
		jobTitle := "your job"
		if err := dbConn.Select("title").First(&jobPost, "id = ?", ctr.JobPostID).Error; err == nil {
			jobTitle = jobPost.Title
		}

		if notificationService != nil {
			if ctr.ClientCompleted && ctr.FreelancerCompleted {
				// Both done — notify both parties
				_ = notificationService.NotifyJobCompleted(ctx, ctr.ClientID, jobTitle, ctr.ID, ctr.JobPostID)
				_ = notificationService.NotifyJobCompleted(ctx, ctr.FreelancerID, jobTitle, ctr.ID, ctr.JobPostID)
			} else {
				// Only one party confirmed — notify the other to also confirm
				otherPartyID := ctr.FreelancerID
				if actorID == ctr.FreelancerID {
					otherPartyID = ctr.ClientID
				}
				_ = notificationService.NotifyAwaitingCompletion(ctx, otherPartyID, jobTitle, ctr.ID, ctr.JobPostID)
			}
		}

		// Release escrow if fully completed
		if ctr.ClientCompleted && ctr.FreelancerCompleted && ctr.EscrowPaymentIntentID != "" {
			dbConn.Model(&models.Contract{}).Where("id = ?", ctr.ID).
				Where("escrow_status = ?", "funded").
				Update("escrow_status", "release_pending")
		}
	}(contract, user.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": statusMessage,
		"contract": gin.H{
			"id":                   contract.ID,
			"status":               contract.Status,
			"client_completed":     contract.ClientCompleted,
			"freelancer_completed": contract.FreelancerCompleted,
			"completed_at":         contract.CompletedAt,
			"escrow_status":        contract.EscrowStatus,
		},
	})
}

// func GetContracts

func GetContracts(c *gin.Context) {
	dbConn := db.DB
	user := c.MustGet("user").(models.User)

	var queryParams struct {
		Page       int    `form:"page,default=1"`
		Limit      int    `form:"limit,default=10"`
		Status     string `form:"status"`
		Role       string `form:"role"` // filter by: client, freelancer
		ProposalID string `form:"proposal_id"`
		ContractID string `form:"contract_id"`
	}

	if err := c.ShouldBindQuery(&queryParams); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters"})
		return
	}

	var contracts []models.Contract
	var total int64

	// Build base scoped query (security: only contracts this user is party to)
	query := dbConn.Model(&models.Contract{})

	switch queryParams.Role {
	case "client":
		query = query.Where("client_id = ?", user.ID)
	case "freelancer":
		query = query.Where("freelancer_id = ?", user.ID)
	default:
		query = query.Where("client_id = ? OR freelancer_id = ?", user.ID, user.ID)
	}

	if queryParams.Status != "" {
		query = query.Where("status = ?", queryParams.Status)
	}
	if queryParams.ProposalID != "" {
		query = query.Where("proposal_id = ?", queryParams.ProposalID)
	}
	if queryParams.ContractID != "" {
		query = query.Where("id = ?", queryParams.ContractID)
	}

	// Count before pagination
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to count contracts", "error": err.Error()})
		return
	}

	offset := (queryParams.Page - 1) * queryParams.Limit

	err := query.
		Limit(queryParams.Limit).
		Offset(offset).
		Order("contracts.created_at DESC").
		Preload("JobPost").
		Preload("JobPost.JobPostLocation").
		Preload("Client").
		Preload("Freelancer").
		Preload("Proposal").
		Preload("Reviews").
		Find(&contracts).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Failed to fetch contracts",
			"error":   err.Error()})
		return
	}

	// We create a temporary struct to hold the Contract + The Computed Flags
	type ContractResponse struct {
		models.Contract           // Embed the original contract fields
		ReviewedByClient     bool `json:"reviewed_by_client"`
		ReviewedByFreelancer bool `json:"reviewed_by_freelancer"`
	}

	// Initialize the result slice
	responseList := make([]ContractResponse, 0, len(contracts))

	for _, contract := range contracts {
		rByClient := false
		rByFreelancer := false

		// Check the preloaded reviews
		// Assuming your Review model has a 'ReviewerID' field
		for _, review := range contract.Reviews {
			if review.ReviewerID == contract.ClientID {
				rByClient = true
			}
			if review.ReviewerID == contract.FreelancerID {
				rByFreelancer = true
			}
		}

		// Add to the response list
		responseList = append(responseList, ContractResponse{
			Contract:             contract,
			ReviewedByClient:     rByClient,
			ReviewedByFreelancer: rByFreelancer,
		})
	}

	// 8. Return the processed list
	c.JSON(http.StatusOK, gin.H{
		"data": responseList, // Send the wrapper list, not the raw contracts
		"meta": gin.H{
			"current_page": queryParams.Page,
			"limit":        queryParams.Limit,
			"total":        total,
			"total_pages":  int(math.Ceil(float64(total) / float64(queryParams.Limit))),
		},
	})
}

// Route: POST /api/contracts/:id/review
func AddReview(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	contractID := c.Param("id")

	var body struct {
		Rating  int    `json:"rating" binding:"required,min=1,max=5"`
		Comment string `json:"comment"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid input. Rating must be between 1 and 5.",
		})
		return
	}

	var contract models.Contract
	if err := db.DB.Where("id = ?", contractID).First(&contract).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Contract not found"})
		return
	}

	var targetID string
	if user.ID == contract.ClientID {
		targetID = contract.FreelancerID
	} else if user.ID == contract.FreelancerID {
		targetID = contract.ClientID
	} else {

		c.JSON(http.StatusForbidden, gin.H{
			"error": "unauthorized access",
		})
		return
	}

	if contract.Status != models.ContractStatusCompleted {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "You can only review completed contracts. Current status: " + contract.Status,
		})
		return
	}

	var existingReview models.Review
	if err := db.DB.Where("contract_id = ? AND reviewer_id = ?", contract.ID, user.ID).
		First(&existingReview).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{ // 409 Conflict
			"error": "You have already submitted a review for this contract",
		})
		return
	}

	review := models.Review{
		ContractID: contract.ID,
		ReviewerID: user.ID,
		TargetID:   targetID,
		Rating:     body.Rating,
		Comment:    body.Comment,
	}

	if err := db.DB.Create(&review).Error; err != nil {
		if strings.Contains(err.Error(), "idx_review_contract_reviewer") {
			c.JSON(http.StatusConflict, gin.H{"error": "You have already submitted a review for this contract"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save review",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Review submitted successfully",
		"data":    review,
	})
}
