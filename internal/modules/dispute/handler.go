package dispute

import (
	"errors"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"

	paymentsv3 "tasksy/internal/modules/payments-v3"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

func (h *Handler) CreateDispute(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	var body struct {
		ContractID  string `form:"contract_id" binding:"required"`
		Reason      string `form:"reason"      binding:"required,max=255"`
		Description string `form:"description" binding:"required,min=10"`
	}
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	var files []*multipart.FileHeader
	if form, err := c.MultipartForm(); err == nil && form != nil {
		files = form.File["file"]
	}
	if len(files) > 5 {
		files = files[:5]
	}

	dispute, err := h.service.CreateDispute(body.ContractID, user.ID, body.Reason, body.Description, files)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "success", "data": dispute})
}

func (h *Handler) ListMyDisputes(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	status := c.Query("status")

	disputes, total, err := h.service.ListMyDisputes(user.ID, page, limit, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    disputes,
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

func (h *Handler) GetDispute(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	dispute, err := h.service.GetDispute(id, user.ID)
	if err != nil {
		switch err.Error() {
		case "dispute not found":
			c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
		case "access denied":
			c.JSON(http.StatusForbidden, gin.H{"message": "error", "error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": dispute})
}

func (h *Handler) AdminGetDispute(c *gin.Context) {
	id := c.Param("id")

	dispute, err := h.service.AdminGetDispute(id)
	if err != nil {
		if err.Error() == "dispute not found" {
			c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": dispute})
}

func (h *Handler) AdminListDisputes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")
	contractID := c.Query("contract_id")

	disputes, total, err := h.service.AdminListDisputes(page, limit, status, contractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    disputes,
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": int(math.Ceil(float64(total) / float64(limit))),
		},
	})
}

func (h *Handler) AdminResolveDispute(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	var body struct {
		Resolution string `json:"resolution" binding:"required,min=5"`
		Outcome    string `json:"outcome"`
		RefundFees *bool  `json:"refund_fees"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	refundFees := body.RefundFees == nil || *body.RefundFees

	dispute, err := h.service.AdminResolveDispute(id, user.ID, body.Resolution, body.Outcome, refundFees)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case err.Error() == "dispute not found":
			status = http.StatusNotFound
		case errors.Is(err, paymentsv3.ErrRefundStateChanged), errors.Is(err, paymentsv3.ErrAlreadyRefunded),
			errors.Is(err, paymentsv3.ErrContractClosed):
			status = http.StatusConflict
		case isStripeFailure(err):
			status = http.StatusBadGateway
		}
		c.JSON(status, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": dispute})
}

func isStripeFailure(err error) bool {
	var se *stripe.Error
	return errors.As(err, &se)
}
