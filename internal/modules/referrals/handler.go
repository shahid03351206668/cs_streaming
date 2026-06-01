package referrals

import (
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ValidateReferralCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "Referral code is required"})
		return
	}

	referralCode, err := h.service.ValidateReferralCode(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error(), "valid": false})
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

func (h *Handler) CreateReferralCode(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var params CreateReferralCodeParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "Invalid request body: " + err.Error()})
		return
	}

	params.OwnerID = user.ID

	code, err := h.service.CreateReferralCode(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "success", "data": code})
}

func (h *Handler) GetMyReferralCodes(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	codes, err := h.service.GetReferralCodesByOwner(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "Failed to fetch referral codes: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": codes})
}

func (h *Handler) GetReferralCodeByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "Referral code ID is required"})
		return
	}

	code, err := h.service.GetReferralCodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": code})
}

func (h *Handler) UpdateReferralCode(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "Referral code ID is required"})
		return
	}

	code, err := h.service.GetReferralCodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "Referral code not found"})
		return
	}

	if code.OwnerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": "You are not authorized to update this referral code"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "Invalid request body"})
		return
	}

	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "code")
	delete(updates, "current_uses")

	if err := h.service.UpdateReferralCode(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "Failed to update referral code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) DeleteReferralCode(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "Referral code ID is required"})
		return
	}

	code, err := h.service.GetReferralCodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "Referral code not found"})
		return
	}

	if code.OwnerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": "You are not authorized to delete this referral code"})
		return
	}

	if err := h.service.DeleteReferralCode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "Failed to delete referral code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) GetMyReferrals(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	usages, err := h.service.GetReferralUsagesByReferrer(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "Failed to fetch referrals: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": usages})
}

func (h *Handler) GetMyReferralStatus(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	usage, err := h.service.GetReferralUsageForUser(user.ID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "success", "has_referral": false, "data": nil})
		return
	}

	qualifiedAt := ""
	if usage.QualifiedAt != nil {
		qualifiedAt = usage.QualifiedAt.Format("2006-01-02T15:04:05Z")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "success",
		"has_referral": true,
		"data": gin.H{
			"referral_code":       usage.ReferralCode.Code,
			"discount_amount":     usage.ReferralCode.DiscountAmount,
			"discount_percentage": usage.ReferralCode.DiscountPercentage,
			"is_qualified":        usage.IsQualified,
			"qualified_at":        qualifiedAt,
			"discount_applied":    usage.DiscountApplied,
			"status":              usage.Status,
		},
	})
}

func (h *Handler) GetAllReferralCodes(c *gin.Context) {
	codes, err := h.service.GetAllReferralCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "Failed to fetch referral codes"})
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

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}

func (h *Handler) GetAllReferralUsages(c *gin.Context) {
	usages, err := h.service.GetAllReferralUsages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "Failed to fetch referral usages"})
		return
	}

	result := make([]gin.H, 0, len(usages))
	for _, u := range usages {
		qualifiedAt := ""
		if u.QualifiedAt != nil {
			qualifiedAt = u.QualifiedAt.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, gin.H{
			"id":               u.ID,
			"referral_code":    u.ReferralCode.Code,
			"referrer_id":      u.ReferrerID,
			"referrer_name":    u.Referrer.FirstName + " " + u.Referrer.LastName,
			"referee_id":       u.RefereeID,
			"referee_name":     u.Referee.FirstName + " " + u.Referee.LastName,
			"discount_applied": u.DiscountApplied,
			"reward_amount":    u.RewardAmount,
			"status":           u.Status,
			"is_qualified":     u.IsQualified,
			"qualified_at":     qualifiedAt,
			"created_at":       u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}
