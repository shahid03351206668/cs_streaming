package promotions

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateOffer handles POST /api/v1/admin/promotions
func (h *Handler) CreateOffer(c *gin.Context) {
	var params CreateOfferParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	offer, err := h.service.CreateOffer(params)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "success", "data": offer})
}

// ListOffers handles GET /api/v1/admin/promotions
func (h *Handler) ListOffers(c *gin.Context) {
	offers, err := h.service.ListAllOffers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": offers})
}

// ListActiveOffers handles GET /api/v1/promotions (public)
func (h *Handler) ListActiveOffers(c *gin.Context) {
	offers, err := h.service.ListActiveOffers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": offers})
}

// UpdateOffer handles PUT /api/v1/admin/promotions/:id
func (h *Handler) UpdateOffer(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "invalid request body"})
		return
	}
	if err := h.service.UpdateOffer(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// DeleteOffer handles DELETE /api/v1/admin/promotions/:id
func (h *Handler) DeleteOffer(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteOffer(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// CheckMyEligibility handles GET /api/v1/promotions/my-eligibility?amount=<cents>
func (h *Handler) CheckMyEligibility(c *gin.Context) {
	type userWithID struct {
		ID string
	}
	u, _ := c.Get("user")
	user, _ := u.(userWithID)
	userID := user.ID

	var amountParam struct {
		Amount int64 `form:"amount"`
	}
	c.ShouldBindQuery(&amountParam)
	if amountParam.Amount == 0 {
		amountParam.Amount = 10000 // Default 100.00 for preview
	}

	discount, offer, _ := h.service.CheckEligibility(userID, amountParam.Amount)

	response := gin.H{
		"discount_amount": discount,
		"offer":           nil,
	}
	if offer != nil {
		response["offer"] = gin.H{
			"id":                  offer.ID,
			"name":                offer.Name,
			"description":         offer.Description,
			"discount_percentage": offer.DiscountPercentage,
			"discount_amount":     offer.DiscountAmount,
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": response})
}
