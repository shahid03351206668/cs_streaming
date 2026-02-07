package payments

import (
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

// CreateReferralCode handles POST /api/v1/referrals/codes
func (s *StripePaymentHandler) CreateReferralCode(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var params CreateReferralCodeParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Always set owner to authenticated user (security: users can only create codes for themselves)
	params.OwnerID = user.ID

	code, err := s.service.CreateReferralCode(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"data":    code,
	})
}

// ValidateReferralCode handles GET /api/v1/referrals/validate/:code
func (s *StripePaymentHandler) ValidateReferralCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Referral code is required",
		})
		return
	}

	referralCode, err := s.service.ValidateReferralCode(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
			"valid":   false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"valid":   true,
		"data": gin.H{
			"code":                referralCode.Code,
			"discount_amount":     referralCode.DiscountAmount,
			"discount_percentage": referralCode.DiscountPercentage,
			"type":                referralCode.Type,
		},
	})
}

// GetMyReferralCodes handles GET /api/v1/referrals/codes/my
func (s *StripePaymentHandler) GetMyReferralCodes(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	codes, err := s.service.GetReferralCodesByOwner(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch referral codes: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    codes,
	})
}

// GetMyReferrals handles GET /api/v1/referrals/my
func (s *StripePaymentHandler) GetMyReferrals(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	usages, err := s.service.GetReferralUsagesByReferrer(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch referrals: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    usages,
	})
}

// GetMyReferralStatus handles GET /api/v1/referrals/status
// Returns the referral status for the authenticated user (as a referee)
func (s *StripePaymentHandler) GetMyReferralStatus(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	usage, err := s.service.GetReferralUsageForUser(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message":      "success",
			"has_referral": false,
			"data":         nil,
		})
		return
	}

	// Load referral code details
	var referralCode models.ReferralCode
	s.service.db.Where("id = ?", usage.ReferralCodeID).First(&referralCode)

	qualifiedAt := ""
	if usage.QualifiedAt != nil {
		qualifiedAt = usage.QualifiedAt.Format("2006-01-02T15:04:05Z")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "success",
		"has_referral": true,
		"data": gin.H{
			"referral_code":       referralCode.Code,
			"discount_amount":     referralCode.DiscountAmount,
			"discount_percentage": referralCode.DiscountPercentage,
			"is_qualified":        usage.IsQualified,
			"qualified_at":        qualifiedAt,
			"discount_applied":    usage.DiscountApplied,
			"status":              usage.Status,
		},
	})
}

// GetReferralCodeByID handles GET /api/v1/referrals/codes/:id
func (s *StripePaymentHandler) GetReferralCodeByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Referral code ID is required",
		})
		return
	}

	code, err := s.service.GetReferralCodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    code,
	})
}

// UpdateReferralCode handles PUT /api/v1/referrals/codes/:id
func (s *StripePaymentHandler) UpdateReferralCode(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Referral code ID is required",
		})
		return
	}

	// Verify ownership
	code, err := s.service.GetReferralCodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "error",
			"error":   "Referral code not found",
		})
		return
	}

	if code.OwnerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "You are not authorized to update this referral code",
		})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Invalid request body",
		})
		return
	}

	// Prevent updating sensitive fields
	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "code")
	delete(updates, "current_uses")

	if err := s.service.UpdateReferralCode(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to update referral code",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}

// DeleteReferralCode handles DELETE /api/v1/referrals/codes/:id
func (s *StripePaymentHandler) DeleteReferralCode(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Referral code ID is required",
		})
		return
	}

	// Verify ownership
	code, err := s.service.GetReferralCodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "error",
			"error":   "Referral code not found",
		})
		return
	}

	if code.OwnerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "You are not authorized to delete this referral code",
		})
		return
	}

	if err := s.service.DeleteReferralCode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to delete referral code",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}

// GetAllReferralCodes handles GET /api/v1/referrals/codes (admin)
func (s *StripePaymentHandler) GetAllReferralCodes(c *gin.Context) {
	var codes []models.ReferralCode
	if err := s.service.db.Preload("Owner").Order("created_at DESC").Find(&codes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch referral codes",
		})
		return
	}

	result := make([]gin.H, 0, len(codes))
	for _, code := range codes {
		result = append(result, gin.H{
			"id":                  code.ID,
			"code":                code.Code,
			"owner_id":            code.OwnerID,
			"owner_name":          code.Owner.FirstName + " " + code.Owner.LastName,
			"type":                code.Type,
			"discount_amount":     code.DiscountAmount,
			"discount_percentage": code.DiscountPercentage,
			"max_uses":            code.MaxUses,
			"current_uses":        code.CurrentUses,
			"is_active":           code.IsActive,
			"expires_at":          code.ExpiresAt,
			"created_at":          code.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    result,
	})
}

// GetAllReferralUsages handles GET /api/v1/referrals/usages (admin)
func (s *StripePaymentHandler) GetAllReferralUsages(c *gin.Context) {
	var usages []models.ReferralUsage
	if err := s.service.db.Preload("ReferralCode").Preload("Referrer").Preload("Referee").
		Order("created_at DESC").Find(&usages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch referral usages",
		})
		return
	}

	result := make([]gin.H, 0, len(usages))
	for _, usage := range usages {
		qualifiedAt := ""
		if usage.QualifiedAt != nil {
			qualifiedAt = usage.QualifiedAt.Format("2006-01-02T15:04:05Z")
		}

		result = append(result, gin.H{
			"id":               usage.ID,
			"referral_code":    usage.ReferralCode.Code,
			"referrer_id":      usage.ReferrerID,
			"referrer_name":    usage.Referrer.FirstName + " " + usage.Referrer.LastName,
			"referee_id":       usage.RefereeID,
			"referee_name":     usage.Referee.FirstName + " " + usage.Referee.LastName,
			"discount_applied": usage.DiscountApplied,
			"reward_amount":    usage.RewardAmount,
			"status":           usage.Status,
			"is_qualified":     usage.IsQualified,
			"qualified_at":     qualifiedAt,
			"created_at":       usage.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    result,
	})
}
