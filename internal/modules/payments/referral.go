package payments

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"tasksy/models"
	"time"
)

// ReferralCodeResponse represents a referral code in API responses
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

// CreateReferralCodeParams parameters for creating a referral code
type CreateReferralCodeParams struct {
	OwnerID            string     `json:"owner_id"`
	Code               string     `json:"code"`
	Type               string     `json:"type"`
	DiscountAmount     int64      `json:"discount_amount"`
	DiscountPercentage int64      `json:"discount_percentage"`
	MaxUses            int        `json:"max_uses"`
	ExpiresAt          *time.Time `json:"expires_at"`
}

// ReferralUsageResponse represents referral usage in API responses
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

// generateReferralCode generates a random referral code
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

// CreateReferralCode creates a new referral code
func (s *PaymentService) CreateReferralCode(params CreateReferralCodeParams) (*ReferralCodeResponse, error) {
	// Generate code if not provided
	code := params.Code
	if code == "" {
		var err error
		code, err = generateReferralCode(8)
		if err != nil {
			return nil, errors.New("failed to generate referral code")
		}
	}
	code = strings.ToUpper(code)

	// Check if code already exists
	var existingCode models.ReferralCode
	if err := s.db.Where("UPPER(code) = ?", code).First(&existingCode).Error; err == nil {
		return nil, errors.New("referral code already exists")
	}

	// Validate owner exists
	var owner models.User
	if err := s.db.Where("id = ?", params.OwnerID).First(&owner).Error; err != nil {
		return nil, errors.New("owner not found")
	}

	// Set defaults
	codeType := params.Type
	if codeType == "" {
		codeType = "standard"
	}

	maxUses := params.MaxUses
	if maxUses == 0 {
		maxUses = -1 // unlimited
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

	return &ReferralCodeResponse{
		ID:                 referralCode.ID,
		Code:               referralCode.Code,
		OwnerID:            referralCode.OwnerID,
		Type:               referralCode.Type,
		DiscountAmount:     referralCode.DiscountAmount,
		DiscountPercentage: referralCode.DiscountPercentage,
		MaxUses:            referralCode.MaxUses,
		CurrentUses:        referralCode.CurrentUses,
		IsActive:           referralCode.IsActive,
		ExpiresAt:          referralCode.ExpiresAt,
		CreatedAt:          referralCode.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// ValidateReferralCode validates and returns a referral code
func (s *PaymentService) ValidateReferralCode(code string) (*models.ReferralCode, error) {
	var referralCode models.ReferralCode
	if err := s.db.Where("UPPER(code) = ? AND is_active = ?", strings.ToUpper(code), true).First(&referralCode).Error; err != nil {
		return nil, errors.New("invalid or inactive referral code")
	}

	// Check if expired
	if referralCode.ExpiresAt != nil && referralCode.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("referral code has expired")
	}

	// Check max uses
	if referralCode.MaxUses != -1 && referralCode.CurrentUses >= referralCode.MaxUses {
		return nil, errors.New("referral code has reached maximum uses")
	}

	return &referralCode, nil
}

// ApplyReferralCode applies a referral code for a new user
func (s *PaymentService) ApplyReferralCode(refereeID string, code string) (*models.ReferralUsage, error) {
	// Validate code
	referralCode, err := s.ValidateReferralCode(code)
	if err != nil {
		return nil, err
	}

	// Check if user is trying to use their own code
	if referralCode.OwnerID == refereeID {
		return nil, errors.New("cannot use your own referral code")
	}

	// Check if user already used a referral code
	var existingUsage models.ReferralUsage
	if err := s.db.Where("referee_id = ?", refereeID).First(&existingUsage).Error; err == nil {
		return nil, errors.New("user has already used a referral code")
	}

	// Create referral usage
	usage := models.ReferralUsage{
		ReferralCodeID: referralCode.ID,
		ReferrerID:     referralCode.OwnerID,
		RefereeID:      refereeID,
		Status:         models.ReferralStatusPending,
		IsQualified:    false,
	}

	tx := s.db.Begin()

	if err := tx.Create(&usage).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Increment usage count
	if err := tx.Model(&models.ReferralCode{}).Where("id = ?", referralCode.ID).
		Update("current_uses", referralCode.CurrentUses+1).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &usage, nil
}

// GetReferralUsageForUser gets the referral usage for a user (as referee)
func (s *PaymentService) GetReferralUsageForUser(userID string) (*models.ReferralUsage, error) {
	var usage models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Where("referee_id = ?", userID).First(&usage).Error; err != nil {
		return nil, err
	}
	return &usage, nil
}

// CalculateReferralDiscount calculates the discount amount for a transaction
func (s *PaymentService) CalculateReferralDiscount(userID string, amount int64) (int64, *models.ReferralUsage, error) {
	// Get referral usage for user
	usage, err := s.GetReferralUsageForUser(userID)
	if err != nil {
		return 0, nil, nil // No referral, no discount
	}

	// Check if already qualified (discount already applied)
	if usage.IsQualified {
		return 0, nil, nil // Already used their discount
	}

	// Load the referral code
	var referralCode models.ReferralCode
	if err := s.db.Where("id = ?", usage.ReferralCodeID).First(&referralCode).Error; err != nil {
		return 0, nil, nil
	}

	// Calculate discount
	var discount int64
	if referralCode.DiscountAmount > 0 {
		discount = referralCode.DiscountAmount
	} else if referralCode.DiscountPercentage > 0 {
		discount = (amount * referralCode.DiscountPercentage) / 100
	}

	// Cap discount at transaction amount
	if discount > amount {
		discount = amount
	}

	return discount, usage, nil
}

// MarkReferralAsQualified marks a referral as qualified after first transaction.
func (s *PaymentService) MarkReferralAsQualified(usage *models.ReferralUsage, transactionID string, discountApplied int64) error {
	now := time.Now()
	settings, _ := GetSystemSettings()
	reward := settings.ReferralRewardAmount

	return s.db.Model(&models.ReferralUsage{}).Where("id = ?", usage.ID).Updates(map[string]interface{}{
		"is_qualified":         true,
		"qualified_at":         now,
		"status":               models.ReferralStatusQualified,
		"first_transaction_id": transactionID,
		"discount_applied":     discountApplied,
		"reward_amount":        reward,
	}).Error
}

// GetReferralCodeByOwner gets all referral codes for an owner
func (s *PaymentService) GetReferralCodesByOwner(ownerID string) ([]ReferralCodeResponse, error) {
	var codes []models.ReferralCode
	if err := s.db.Where("owner_id = ?", ownerID).Order("created_at DESC").Find(&codes).Error; err != nil {
		return nil, err
	}

	result := make([]ReferralCodeResponse, 0, len(codes))
	for _, code := range codes {
		result = append(result, ReferralCodeResponse{
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
		})
	}

	return result, nil
}

// GetReferralUsagesByReferrer gets all referral usages where user is the referrer
func (s *PaymentService) GetReferralUsagesByReferrer(referrerID string) ([]ReferralUsageResponse, error) {
	var usages []models.ReferralUsage
	if err := s.db.Preload("ReferralCode").Preload("Referee").
		Where("referrer_id = ?", referrerID).
		Order("created_at DESC").
		Find(&usages).Error; err != nil {
		return nil, err
	}

	result := make([]ReferralUsageResponse, 0, len(usages))
	for _, usage := range usages {
		qualifiedAt := ""
		if usage.QualifiedAt != nil {
			qualifiedAt = usage.QualifiedAt.Format("2006-01-02T15:04:05Z")
		}

		result = append(result, ReferralUsageResponse{
			ID:              usage.ID,
			ReferralCodeID:  usage.ReferralCodeID,
			ReferralCode:    usage.ReferralCode.Code,
			ReferrerID:      usage.ReferrerID,
			RefereeID:       usage.RefereeID,
			RefereeName:     usage.Referee.FirstName + " " + usage.Referee.LastName,
			DiscountApplied: usage.DiscountApplied,
			RewardAmount:    usage.RewardAmount,
			Status:          usage.Status,
			IsQualified:     usage.IsQualified,
			QualifiedAt:     qualifiedAt,
			CreatedAt:       usage.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return result, nil
}

// GetReferralCodeByID gets a referral code by ID
func (s *PaymentService) GetReferralCodeByID(id string) (*ReferralCodeResponse, error) {
	var code models.ReferralCode
	if err := s.db.Where("id = ?", id).First(&code).Error; err != nil {
		return nil, errors.New("referral code not found")
	}

	return &ReferralCodeResponse{
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
	}, nil
}

// UpdateReferralCode updates a referral code
func (s *PaymentService) UpdateReferralCode(id string, updates map[string]interface{}) error {
	return s.db.Model(&models.ReferralCode{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteReferralCode soft deletes a referral code
func (s *PaymentService) DeleteReferralCode(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.ReferralCode{}).Error
}
