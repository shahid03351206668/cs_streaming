package referrals

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"tasksy/models"
	"time"

	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

type ReferralCodeResponse struct {
	ID                 string     `json:"id"`
	Code               string     `json:"code"`
	OwnerID            string     `json:"owner_id"`
	Type               string     `json:"type"`
	DiscountAmount     int64      `json:"discount_amount"`
	DiscountPercentage int64      `json:"discount_percentage"`
	MaxUses            int        `json:"max_uses"`
	CurrentUses        int        `json:"current_uses"`
	IsActive           bool       `json:"is_active"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	CreatedAt          string     `json:"created_at"`
}

type CreateReferralCodeParams struct {
	OwnerID            string     `json:"owner_id"`
	Code               string     `json:"code"`
	Type               string     `json:"type"`
	DiscountAmount     int64      `json:"discount_amount"`
	DiscountPercentage int64      `json:"discount_percentage"`
	MaxUses            int        `json:"max_uses"`
	ExpiresAt          *time.Time `json:"expires_at"`
}

type ReferralUsageResponse struct {
	ID              string `json:"id"`
	ReferralCodeID  string `json:"referral_code_id"`
	ReferralCode    string `json:"referral_code"`
	ReferrerID      string `json:"referrer_id"`
	ReferrerName    string `json:"referrer_name"`
	RefereeID       string `json:"referee_id"`
	RefereeName     string `json:"referee_name"`
	DiscountApplied int64  `json:"discount_applied"`
	RewardAmount    int64  `json:"reward_amount"`
	Status          string `json:"status"`
	IsQualified     bool   `json:"is_qualified"`
	QualifiedAt     string `json:"qualified_at,omitempty"`
	CreatedAt       string `json:"created_at"`
}

func generateReferralCode(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

func toReferralCodeResponse(code models.ReferralCode) ReferralCodeResponse {
	return ReferralCodeResponse{
		ID:                 code.ID,
		Code:               code.Code,
		OwnerID:            code.OwnerID,
		Type:               code.Type,
		DiscountAmount:     code.DiscountAmount,
		DiscountPercentage: code.DiscountPercentage,
		MaxUses:            code.MaxUses,
		CurrentUses:        code.CurrentUses,
		IsActive:           code.IsActive,
		ExpiresAt:          code.ExpiresAt,
		CreatedAt:          code.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *Service) CreateReferralCode(params CreateReferralCodeParams) (*ReferralCodeResponse, error) {
	code := params.Code
	if code == "" {
		var err error
		code, err = generateReferralCode(8)
		if err != nil {
			return nil, errors.New("failed to generate referral code")
		}
	}
	code = strings.ToUpper(code)

	var existing models.ReferralCode
	if err := s.db.Where("UPPER(code) = ?", code).First(&existing).Error; err == nil {
		return nil, errors.New("referral code already exists")
	}

	var owner models.User
	if err := s.db.Where("id = ?", params.OwnerID).First(&owner).Error; err != nil {
		return nil, errors.New("owner not found")
	}

	codeType := params.Type
	if codeType == "" {
		codeType = "standard"
	}
	maxUses := params.MaxUses
	if maxUses == 0 {
		maxUses = -1
	}

	referralCode := models.ReferralCode{
		Code:               code,
		OwnerID:            params.OwnerID,
		Type:               codeType,
		DiscountAmount:     params.DiscountAmount,
		DiscountPercentage: params.DiscountPercentage,
		MaxUses:            maxUses,
		CurrentUses:        0,
		IsActive:           true,
		ExpiresAt:          params.ExpiresAt,
	}

	if err := s.db.Create(&referralCode).Error; err != nil {
		return nil, err
	}

	resp := toReferralCodeResponse(referralCode)
	return &resp, nil
}

func (s *Service) ValidateReferralCode(code string) (*models.ReferralCode, error) {
	var referralCode models.ReferralCode
	if err := s.db.Where("UPPER(code) = ? AND is_active = ?", strings.ToUpper(code), true).First(&referralCode).Error; err != nil {
		return nil, errors.New("invalid or inactive referral code")
	}
	if referralCode.ExpiresAt != nil && referralCode.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("referral code has expired")
	}
	if referralCode.MaxUses != -1 && referralCode.CurrentUses >= referralCode.MaxUses {
		return nil, errors.New("referral code has reached maximum uses")
	}
	return &referralCode, nil
}

func (s *Service) GetReferralCodeByID(id string) (*ReferralCodeResponse, error) {
	var code models.ReferralCode
	if err := s.db.Where("id = ?", id).First(&code).Error; err != nil {
		return nil, errors.New("referral code not found")
	}
	resp := toReferralCodeResponse(code)
	return &resp, nil
}

func (s *Service) GetReferralCodesByOwner(ownerID string) ([]ReferralCodeResponse, error) {
	var codes []models.ReferralCode
	if err := s.db.Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&codes).Error; err != nil {
		return nil, err
	}
	result := make([]ReferralCodeResponse, 0, len(codes))
	for _, c := range codes {
		result = append(result, toReferralCodeResponse(c))
	}
	return result, nil
}

func (s *Service) UpdateReferralCode(id string, updates map[string]interface{}) error {
	return s.db.Model(&models.ReferralCode{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) DeleteReferralCode(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.ReferralCode{}).Error
}

func (s *Service) GetReferralUsageForUser(userID string) (*models.ReferralUsage, error) {
	var usage models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Where("referee_id = ?", userID).First(&usage).Error; err != nil {
		return nil, err
	}
	return &usage, nil
}

func (s *Service) GetReferralUsagesByReferrer(referrerID string) ([]ReferralUsageResponse, error) {
	var usages []models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Preload("Referee").
		Where("referrer_id = ?", referrerID).
		Order("created_at DESC").
		Find(&usages).Error; err != nil {
		return nil, err
	}

	result := make([]ReferralUsageResponse, 0, len(usages))
	for _, u := range usages {
		qualifiedAt := ""
		if u.QualifiedAt != nil {
			qualifiedAt = u.QualifiedAt.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, ReferralUsageResponse{
			ID:              u.ID,
			ReferralCodeID:  u.ReferralCodeID,
			ReferralCode:    u.ReferralCode.Code,
			ReferrerID:      u.ReferrerID,
			RefereeID:       u.RefereeID,
			RefereeName:     u.Referee.FirstName + " " + u.Referee.LastName,
			DiscountApplied: u.DiscountApplied,
			RewardAmount:    u.RewardAmount,
			Status:          u.Status,
			IsQualified:     u.IsQualified,
			QualifiedAt:     qualifiedAt,
			CreatedAt:       u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

func (s *Service) GetAllReferralCodes() ([]models.ReferralCode, error) {
	var codes []models.ReferralCode
	if err := s.db.Preload("Owner").Order("created_at DESC").Find(&codes).Error; err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) GetAllReferralUsages() ([]models.ReferralUsage, error) {
	var usages []models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Preload("Referrer").Preload("Referee").
		Order("created_at DESC").Find(&usages).Error; err != nil {
		return nil, err
	}
	return usages, nil
}
