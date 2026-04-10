package dispute

import (
	"context"
	"errors"
	"time"

	"tasksy/internal/modules/notifications"
	"tasksy/models"
	"tasksy/pkg/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db    *gorm.DB
	notif *notifications.Service
}

func NewService(db *gorm.DB, notif *notifications.Service) *Service {
	return &Service{db: db, notif: notif}
}

// CreateDispute files a new dispute against a contract. The contract must be
// in "active" or "in_progress" status and the caller must be a party to it.
func (s *Service) CreateDispute(contractID, filedByID, reason, description string) (*models.Dispute, error) {
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

	if contract.Status != models.ContractStatusActive {
		return nil, errors.New("disputes can only be filed on contracts that are in progress")
	}

	// Prevent duplicate open disputes from the same user
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
		Reason:      reason,
		Description: description,
		Status:      models.DisputeStatusOpen,
	}

	tx := s.db.Begin()
	if err := tx.Create(&dispute).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Mark contract as disputed
	if err := tx.Model(&models.Contract{}).Where("id = ?", contractID).
		Update("status", models.ContractStatusDisputed).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Reload with relations
	s.db.Preload("FiledBy").First(&dispute, "id = ?", dispute.ID)

	// Notify the other party asynchronously
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
				zap.String("dispute_id", dispute.ID),
				zap.Error(err),
			)
		}
	}()

	return &dispute, nil
}

// ListMyDisputes returns disputes related to the calling user (filed by or party to contract).
func (s *Service) ListMyDisputes(userID string, page, limit int, status string) ([]models.Dispute, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}

	query := s.db.Model(&models.Dispute{}).
		Joins("JOIN contracts ON contracts.id = disputes.contract_id").
		Where("disputes.filed_by_id = ? OR contracts.client_id = ? OR contracts.freelancer_id = ?", userID, userID, userID)

	if status != "" {
		query = query.Where("disputes.status = ?", status)
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
		Order("disputes.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&disputes).Error

	return disputes, total, err
}

// GetDispute returns a single dispute by ID, restricted to parties of the contract.
func (s *Service) GetDispute(id, userID string) (*models.Dispute, error) {
	var dispute models.Dispute
	err := s.db.
		Preload("FiledBy").
		Preload("Contract").
		Preload("ResolvedBy").
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

	return &dispute, nil
}

// AdminListDisputes returns all disputes with optional status filter.
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
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&disputes).Error

	return disputes, total, err
}

// AdminResolveDispute marks a dispute as resolved and optionally re-activates the contract.
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

	// If no other open disputes on the contract, revert contract back to active
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

	s.db.Preload("FiledBy").Preload("Contract").Preload("ResolvedBy").First(&dispute, "id = ?", id)

	// Notify both parties
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
