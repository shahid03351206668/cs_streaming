package promotions

import (
	"errors"
	"tasksy/models"

	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

type CreateOfferParams struct {
	Name               string  `json:"name" binding:"required"`
	Description        string  `json:"description"`
	MinJobsCompleted   int     `json:"min_jobs_completed"`
	MinJobsPosted      int     `json:"min_jobs_posted"`
	DiscountPercentage float64 `json:"discount_percentage"`
	DiscountAmount     int64   `json:"discount_amount"`
	MaxDiscountAmount  int64   `json:"max_discount_amount"`
	IsActive           bool    `json:"is_active"`
	ExpiresAt          *string `json:"expires_at"`
}

func (s *Service) CreateOffer(params CreateOfferParams) (*models.PromotionalOffer, error) {
	if params.DiscountPercentage == 0 && params.DiscountAmount == 0 {
		return nil, errors.New("at least one of discount_percentage or discount_amount is required")
	}

	offer := models.PromotionalOffer{
		Name:               params.Name,
		Description:        params.Description,
		MinJobsCompleted:   params.MinJobsCompleted,
		MinJobsPosted:      params.MinJobsPosted,
		DiscountPercentage: params.DiscountPercentage,
		DiscountAmount:     params.DiscountAmount,
		MaxDiscountAmount:  params.MaxDiscountAmount,
		IsActive:           params.IsActive,
	}

	if err := s.db.Create(&offer).Error; err != nil {
		return nil, err
	}

	return &offer, nil
}

func (s *Service) ListActiveOffers() ([]models.PromotionalOffer, error) {
	var offers []models.PromotionalOffer
	err := s.db.Where("is_active = ? AND (expires_at IS NULL OR expires_at > NOW())", true).
		Order("created_at DESC").Find(&offers).Error
	return offers, err
}

func (s *Service) ListAllOffers() ([]models.PromotionalOffer, error) {
	var offers []models.PromotionalOffer
	err := s.db.Order("created_at DESC").Find(&offers).Error
	return offers, err
}

func (s *Service) UpdateOffer(id string, updates map[string]interface{}) error {
	delete(updates, "id")
	return s.db.Model(&models.PromotionalOffer{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) DeleteOffer(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.PromotionalOffer{}).Error
}

// CheckEligibility checks which active promotions a user qualifies for and returns the best discount.
func (s *Service) CheckEligibility(userID string, amountInCents int64) (int64, *models.PromotionalOffer, error) {
	var completedJobs int64
	s.db.Model(&models.Contract{}).
		Where("freelancer_id = ? AND status = ?", userID, models.ContractStatusCompleted).
		Count(&completedJobs)

	var postedJobs int64
	s.db.Model(&models.JobPost{}).
		Where("created_by_id = ? AND status = ?", userID, models.JobStatusCompleted).
		Count(&postedJobs)

	offers, err := s.ListActiveOffers()
	if err != nil {
		return 0, nil, err
	}

	var bestDiscount int64
	var bestOffer *models.PromotionalOffer

	for i, offer := range offers {
		qualifies := true
		if offer.MinJobsCompleted > 0 && int(completedJobs) < offer.MinJobsCompleted {
			qualifies = false
		}
		if offer.MinJobsPosted > 0 && int(postedJobs) < offer.MinJobsPosted {
			qualifies = false
		}
		if !qualifies {
			continue
		}

		var discount int64
		if offer.DiscountAmount > 0 {
			discount = offer.DiscountAmount
		} else if offer.DiscountPercentage > 0 {
			discount = int64(float64(amountInCents) * offer.DiscountPercentage / 100)
		}

		if offer.MaxDiscountAmount > 0 && discount > offer.MaxDiscountAmount {
			discount = offer.MaxDiscountAmount
		}

		if discount > bestDiscount {
			bestDiscount = discount
			bestOffer = &offers[i]
		}
	}

	return bestDiscount, bestOffer, nil
}
