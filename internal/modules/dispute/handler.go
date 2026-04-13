package dispute

import (
	"math"
	"mime/multipart"
	"net/http"
	"strconv"

	"tasksy/models"

	"github.com/gin-gonic/gin"
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

// GET /api/v1/disputes
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

// GET /api/v1/disputes/:id
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

// GET /api/v1/admin/disputes/:id

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

// GET /api/v1/admin/disputes
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

// PUT /api/v1/admin/disputes/:id/resolve
func (h *Handler) AdminResolveDispute(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	id := c.Param("id")

	var body struct {
		Resolution string `json:"resolution" binding:"required,min=5"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	dispute, err := h.service.AdminResolveDispute(id, user.ID, body.Resolution)
	if err != nil {
		if err.Error() == "dispute not found" {
			c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": dispute})
}
