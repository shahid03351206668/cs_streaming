package payments

import (
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func (s *StripePaymentHandler) GetPaymentTransactions(c *gin.Context) {
	var params TransactionListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Invalid query parameters: " + err.Error(),
		})
		return
	}

	transactions, total, err := s.service.GetPaymentTransactions(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch transactions: " + err.Error(),
		})
		return
	}

	// Calculate total pages safely
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transactions,
		"meta": gin.H{
			"total":       total,
			"page":        params.Page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// GetPaymentTransactionByID handles GET /api/v1/payments/transactions/:id
func (s *StripePaymentHandler) GetPaymentTransactionByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Transaction ID is required",
		})
		return
	}

	transaction, err := s.service.GetPaymentTransactionByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "error",
			"error":   "Transaction not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transaction,
	})
}

func (s *StripePaymentHandler) GetUserPaymentTransactions(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	userID := user.ID

	var params TransactionListParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Invalid query parameters: " + err.Error(),
		})
		return
	}

	params.UserID = userID
	transactions, total, err := s.service.GetPaymentTransactions(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "Failed to fetch transactions: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    transactions,
		"meta": gin.H{
			"total":       total,
			"page":        params.Page,
			"limit":       params.Limit,
			"total_pages": (total + int64(params.Limit) - 1) / int64(params.Limit),
		},
	})
}
