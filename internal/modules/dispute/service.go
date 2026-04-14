package dispute

import (
	"context"
	"errors"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"tasksy/internal/modules/notifications"
	"tasksy/models"
	aws_services "tasksy/pkg"
	"tasksy/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type UserProfileObject struct {
	ID          string `json:"id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phone_no"`
	Photo       string `json:"photo"`
}

type JobPostObject struct {
	ID          string          `json:"id"`
	Category    models.Category `json:"category"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Budget      float64         `json:"budget"`
	OpenBudget  bool            `json:"open_budget"`
	Address     string          `json:"address"`
	Status      string          `json:"status"`
}

type ContractObject struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type DisputeVal struct {
	ID          string             `json:"id"`
	JobPost     JobPostObject      `json:"job_post"`
	ContractID  string             `json:"contract_id"`
	FiledByRole string             `json:"filed_by_role"`
	Reason      string             `json:"reason"`
	Description string             `json:"description"`
	Status      string             `json:"status"`
	Resolution  string             `json:"resolution,omitempty"`
	ResolvedAt  *time.Time         `json:"resolved_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	FiledBy     UserProfileObject  `json:"filed_by"`
	ResolvedBy  *UserProfileObject `json:"resolved_by,omitempty"`
	Contract    ContractObject     `json:"contract"`
	Attachments []models.File      `json:"attachments"`
}

func toDisputeVal(d models.Dispute) DisputeVal {
	filer := UserProfileObject{
		ID:          d.FiledBy.ID,
		FirstName:   d.FiledBy.FirstName,
		LastName:    d.FiledBy.LastName,
		Email:       d.FiledBy.Email,
		PhoneNumber: d.FiledBy.PhoneNumber,
		Photo:       d.FiledBy.ProfilePhoto,
	}

	contract := ContractObject{
		ID:     d.ContractID,
		Title:  d.Contract.Title,
		Status: d.Contract.Status,
	}
	var jobPost JobPostObject

	if d.Contract.JobPost.ID != "" {
		jobPost = JobPostObject{
			ID:          d.Contract.JobPost.ID,
			Category:    d.Contract.JobPost.Category,
			Title:       d.Contract.JobPost.Title,
			Description: d.Contract.JobPost.Description,
			Budget:      d.Contract.JobPost.Budget,
			OpenBudget:  d.Contract.JobPost.OpenBudget,
			Address:     d.Contract.JobPost.Address,
			Status:      d.Contract.JobPost.Status,
		}
	}

	attachments := d.Attachments
	if attachments == nil {
		attachments = []models.File{}
	}

	val := DisputeVal{
		ID:          d.ID,
		ContractID:  d.ContractID,
		FiledByRole: d.FiledByRole,
		Reason:      d.Reason,
		Description: d.Description,
		JobPost:     jobPost,
		Status:      d.Status,
		Resolution:  d.Resolution,
		ResolvedAt:  d.ResolvedAt,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
		FiledBy:     filer,
		Contract:    contract,
		Attachments: attachments,
	}

	if d.ResolvedBy != nil {
		rv := UserProfileObject{
			ID:          d.ResolvedBy.ID,
			FirstName:   d.ResolvedBy.FirstName,
			LastName:    d.ResolvedBy.LastName,
			Email:       d.ResolvedBy.Email,
			PhoneNumber: d.ResolvedBy.PhoneNumber,
			Photo:       d.ResolvedBy.ProfilePhoto,
		}
		val.ResolvedBy = &rv
	}

	return val
}

const entityType = "disputes"

type Service struct {
	db       *gorm.DB
	notif    *notifications.Service
	s3Client *aws_services.S3Client
}

func NewService(db *gorm.DB, notif *notifications.Service, s3Client *aws_services.S3Client) *Service {
	return &Service{db: db, notif: notif, s3Client: s3Client}
}

func (s *Service) CreateDispute(
	contractID, filedByID, reason, description string,
	files []*multipart.FileHeader,
) (*DisputeVal, error) {
	var contract models.Contract
	if err := s.db.First(&contract, "id = ?", contractID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("contract not found")
		}
		return nil, err
	}

	if contract.ClientID != filedByID && contract.FreelancerID != filedByID {
		return nil, errors.New("you are not a party to this contract")
	}

	filedByRole := "client"
	if filedByID == contract.FreelancerID {
		filedByRole = "freelancer"
	}

	var existing int64
	s.db.Model(&models.Dispute{}).
		Where("contract_id = ? AND filed_by_id = ? AND status = ?", contractID, filedByID, models.DisputeStatusOpen).
		Count(&existing)
	if existing > 0 {
		return nil, errors.New("you already have an open dispute for this contract")
	}

	dispute := models.Dispute{
		ContractID:  contractID,
		FiledByID:   filedByID,
		FiledByRole: filedByRole,
		Reason:      reason,
		Description: description,
		Status:      models.DisputeStatusOpen,
	}

	tx := s.db.Begin()
	if err := tx.Create(&dispute).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Model(&models.Contract{}).Where("id = ?", contractID).
		Update("status", models.ContractStatusDisputed).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	if len(files) > 0 && s.s3Client != nil {
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				logger.Log.Warn("dispute attachment open failed", zap.String("file", fh.Filename), zap.Error(err))
				continue
			}
			contentType := fh.Header.Get("Content-Type")
			if contentType == "" {
				contentType = mimeFromName(fh.Filename)
			}
			url, objectKey, err := s.s3Client.UploadFile(f, fh.Filename, contentType, "", "")
			f.Close()
			if err != nil {
				logger.Log.Warn("dispute attachment upload failed", zap.String("file", fh.Filename), zap.Error(err))
				continue
			}
			att := models.File{
				URL:        url,
				FileName:   fh.Filename,
				FileSize:   fh.Size,
				FileType:   contentType,
				ObjectKey:  objectKey,
				EntityID:   dispute.ID,
				EntityType: entityType,
			}
			if err := s.db.Create(&att).Error; err != nil {
				logger.Log.Warn("dispute attachment db save failed", zap.String("file", fh.Filename), zap.Error(err))
			}
		}
	}

	s.db.
		Preload("FiledBy").
		Preload("Contract").
		Preload("Contract.JobPost").
		Preload("Contract.JobPost.Category").
		Preload("Attachments", "entity_type = ?", entityType).
		First(&dispute, "id = ?", dispute.ID)

	go func() {
		if s.notif == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		otherPartyID := contract.FreelancerID
		if filedByID == contract.FreelancerID {
			otherPartyID = contract.ClientID
		}
		if err := s.notif.NotifyDisputeCreated(ctx, otherPartyID, contract.Title, dispute.ID, contractID); err != nil {
			logger.Log.Warn("failed to notify dispute created",
				zap.String("dispute_id", dispute.ID), zap.Error(err))
		}
	}()

	val := toDisputeVal(dispute)
	return &val, nil
}

func (s *Service) ListMyDisputes(userID string, page, limit int, status string) ([]DisputeVal, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}

	query := s.db.Model(&models.Dispute{}).
		Joins("JOIN contracts ON contracts.id = disputes.contract_id").
		Where("disputes.filed_by_id = ? OR contracts.client_id = ? OR contracts.freelancer_id = ?",
			userID, userID, userID)
	if status != "" {
		query = query.Where("disputes.status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var disputes []models.Dispute
	offset := (page - 1) * limit
	if err := query.
		Preload("FiledBy").
		Preload("Contract").
		Preload("Contract.JobPost").
		Preload("Contract.JobPost.Category").
		Preload("Attachments", "entity_type = ?", entityType).
		Order("disputes.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&disputes).Error; err != nil {
		return nil, 0, err
	}

	result := make([]DisputeVal, 0, len(disputes))
	for _, d := range disputes {
		result = append(result, toDisputeVal(d))
	}
	return result, total, nil
}

// GetDispute returns a single dispute, restricted to parties of the contract.
func (s *Service) GetDispute(id, userID string) (*DisputeVal, error) {
	var dispute models.Dispute
	err := s.db.
		Preload("FiledBy").
		Preload("Contract").
		Preload("Contract.JobPost").
		Preload("Contract.JobPost.Category").
		Preload("ResolvedBy").
		Preload("Attachments", "entity_type = ?", entityType).
		First(&dispute, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("dispute not found")
		}
		return nil, err
	}

	c := dispute.Contract
	if c.ClientID != userID && c.FreelancerID != userID && dispute.FiledByID != userID {
		return nil, errors.New("access denied")
	}

	val := toDisputeVal(dispute)
	return &val, nil
}

// AdminGetDispute returns a single dispute by ID without access restriction.
func (s *Service) AdminGetDispute(id string) (*models.Dispute, error) {
	var dispute models.Dispute
	err := s.db.
		Preload("FiledBy").
		Preload("Contract").
		Preload("ResolvedBy").
		Preload("Attachments", "entity_type = ?", entityType).
		First(&dispute, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("dispute not found")
		}
		return nil, err
	}
	return &dispute, nil
}

// AdminListDisputes returns all disputes with optional status/contract filter.
func (s *Service) AdminListDisputes(page, limit int, status, contractID string) ([]models.Dispute, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	query := s.db.Model(&models.Dispute{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if contractID != "" {
		query = query.Where("contract_id = ?", contractID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var disputes []models.Dispute
	offset := (page - 1) * limit
	err := query.
		Preload("FiledBy").
		Preload("Contract").
		Preload("ResolvedBy").
		Preload("Attachments", "entity_type = ?", entityType).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&disputes).Error

	return disputes, total, err
}

// AdminResolveDispute marks a dispute as resolved and re-activates the contract if no
// other open disputes remain.
func (s *Service) AdminResolveDispute(id, resolvedByID, resolution string) (*models.Dispute, error) {
	var dispute models.Dispute
	if err := s.db.Preload("Contract").First(&dispute, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("dispute not found")
		}
		return nil, err
	}
	if dispute.Status == models.DisputeStatusResolved {
		return nil, errors.New("dispute is already resolved")
	}

	now := time.Now()
	tx := s.db.Begin()
	updates := map[string]any{
		"status":         models.DisputeStatusResolved,
		"resolved_by_id": resolvedByID,
		"resolution":     resolution,
		"resolved_at":    now,
	}
	if err := tx.Model(&dispute).Updates(updates).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	var openCount int64
	tx.Model(&models.Dispute{}).
		Where("contract_id = ? AND status = ? AND id != ?", dispute.ContractID, models.DisputeStatusOpen, id).
		Count(&openCount)
	if openCount == 0 {
		if err := tx.Model(&models.Contract{}).Where("id = ?", dispute.ContractID).
			Update("status", models.ContractStatusActive).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.db.
		Preload("FiledBy").
		Preload("Contract").
		Preload("ResolvedBy").
		Preload("Attachments", "entity_type = ?", entityType).
		First(&dispute, "id = ?", id)

	go func() {
		if s.notif == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c := dispute.Contract
		_ = s.notif.NotifyDisputeResolved(ctx, c.ClientID, c.Title, dispute.ID, c.ID)
		_ = s.notif.NotifyDisputeResolved(ctx, c.FreelancerID, c.Title, dispute.ID, c.ID)
	}()

	return &dispute, nil
}

// mimeFromName returns a basic MIME type derived from a file's extension.
func mimeFromName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}
