package user

import (
	"errors"
	"fmt"

	"tasksy/models"
)

var (
	ErrCannotBlockSelf    = errors.New("you cannot block yourself")
	ErrAlreadyBlocked     = errors.New("user is already blocked")
	ErrBlockHasConnection = errors.New("cannot block a user you have an active or past job connection with")
	ErrUserNotFound       = errors.New("user not found")
	ErrBlockNotFound      = errors.New("block not found")
)

func (s *Service) BlockUser(blockerID, blockedID string) (*models.UserBlock, error) {
	if blockerID == blockedID {
		return nil, ErrCannotBlockSelf
	}

	var target models.User
	if err := s.db.First(&target, "id = ?", blockedID).Error; err != nil {
		return nil, ErrUserNotFound
	}

	var existing models.UserBlock
	if err := s.db.Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).First(&existing).Error; err == nil {
		return nil, ErrAlreadyBlocked
	}

	// Users who share a current or past job (in either role) can't block each other.
	var contractCount int64
	s.db.Model(&models.Contract{}).
		Where("(client_id = ? AND freelancer_id = ?) OR (client_id = ? AND freelancer_id = ?)",
			blockerID, blockedID, blockedID, blockerID).
		Count(&contractCount)
	if contractCount > 0 {
		return nil, ErrBlockHasConnection
	}

	block := models.UserBlock{BlockerID: blockerID, BlockedID: blockedID}
	if err := s.db.Create(&block).Error; err != nil {
		return nil, fmt.Errorf("failed to block user: %w", err)
	}

	return &block, nil
}

func (s *Service) UnblockUser(blockerID, blockedID string) error {
	result := s.db.Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).Delete(&models.UserBlock{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBlockNotFound
	}
	return nil
}

func (s *Service) GetBlockedUsers(blockerID string) ([]models.UserBlock, error) {
	var blocks []models.UserBlock
	err := s.db.Preload("Blocked").Where("blocker_id = ?", blockerID).Order("created_at DESC").Find(&blocks).Error
	return blocks, err
}
