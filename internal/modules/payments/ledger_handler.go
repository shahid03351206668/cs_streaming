package payments

import (
	"net/http"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LedgerHandler exposes admin ledger report endpoints.
type LedgerHandler struct {
	ledger *LedgerService
	db     *gorm.DB
}

func NewLedgerHandler(ledger *LedgerService, db *gorm.DB) *LedgerHandler {
	return &LedgerHandler{ledger: ledger, db: db}
}

// --- response types ---

type GLEntryResponse struct {
	ID        string          `json:"id"`
	AccountID string          `json:"account_id"`
	Account   AccountResponse `json:"account"`
	Amount    int64           `json:"amount"`
	Category  string          `json:"category"`
	CreatedAt string          `json:"created_at"`
}

type AccountResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	UserID string `json:"user_id,omitempty"`
}

type LedgerTransactionResponse struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	ReferenceID string            `json:"reference_id"`
	Status      string            `json:"status"`
	Description string            `json:"description"`
	PostingDate string            `json:"posting_date"`
	CreatedAt   string            `json:"created_at"`
	Entries     []GLEntryResponse `json:"entries"`
}

type LedgerReportResponse struct {
	SystemBalance   int64                       `json:"system_balance"`
	SystemHealthy   bool                        `json:"system_healthy"`
	Summary         LedgerSummary               `json:"summary"`
	Transactions    []LedgerTransactionResponse `json:"transactions"`
	Meta            PaginationMeta              `json:"meta"`
}

type LedgerSummary struct {
	TotalEscrow    int64 `json:"total_escrow"`
	TotalRevenue   int64 `json:"total_revenue"`
	TotalMarketing int64 `json:"total_marketing"`
}

type PaginationMeta struct {
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	TotalPages int64 `json:"total_pages"`
}

// GetAdminLedgerReport returns the full ledger report for admin monitoring.
// GET /api/v1/admin/ledger
func (h *LedgerHandler) GetAdminLedgerReport(c *gin.Context) {
	// Parse query params
	page := 1
	limit := 20
	if p := c.Query("page"); p != "" {
		if v, err := parseIntParam(p); err == nil && v > 0 {
			page = v
		}
	}
	if l := c.Query("limit"); l != "" {
		if v, err := parseIntParam(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	txType := c.Query("type")
	fromDate := c.Query("from_date")
	toDate := c.Query("to_date")
	search := c.Query("search")

	// 1. System integrity check
	systemBalance, _ := h.ledger.SystemIntegrityCheck()

	// 2. Account summaries
	escrowBal, _ := h.ledger.GetAccountBalanceByType(models.AccountTypeEscrow)
	revenueBal, _ := h.ledger.GetAccountBalanceByType(models.AccountTypeRevenue)
	marketingBal, _ := h.ledger.GetAccountBalanceByType(models.AccountTypeMarketing)

	// 3. Query transactions with filters
	query := h.db.Model(&models.LedgerTransaction{})

	if txType != "" {
		query = query.Where("type = ?", txType)
	}
	if fromDate != "" {
		if t, err := time.Parse("2006-01-02", fromDate); err == nil {
			query = query.Where("posting_date >= ?", t)
		}
	}
	if toDate != "" {
		if t, err := time.Parse("2006-01-02", toDate); err == nil {
			query = query.Where("posting_date <= ?", t.Add(24*time.Hour))
		}
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("reference_id ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
	}

	var total int64
	query.Count(&total)

	offset := (page - 1) * limit
	var transactions []models.LedgerTransaction
	query.
		Preload("Entries", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Account")
		}).
		Order("posting_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions)

	// 4. Build response
	txnResponses := make([]LedgerTransactionResponse, 0, len(transactions))
	for _, txn := range transactions {
		entryResponses := make([]GLEntryResponse, 0, len(txn.Entries))
		for _, e := range txn.Entries {
			ar := AccountResponse{}
			if e.Account != nil {
				ar.ID = e.Account.ID
				ar.Name = e.Account.Name
				ar.Type = string(e.Account.Type)
				if e.Account.UserID != nil {
					ar.UserID = *e.Account.UserID
				}
			}
			entryResponses = append(entryResponses, GLEntryResponse{
				ID:        e.ID,
				AccountID: e.AccountID,
				Account:   ar,
				Amount:    e.Amount,
				Category:  e.Category,
				CreatedAt: e.CreatedAt.Format(time.RFC3339),
			})
		}

		txnResponses = append(txnResponses, LedgerTransactionResponse{
			ID:          txn.ID,
			Type:        string(txn.Type),
			ReferenceID: txn.ReferenceID,
			Status:      txn.Status,
			Description: txn.Description,
			PostingDate: txn.PostingDate.Format(time.RFC3339),
			CreatedAt:   txn.CreatedAt.Format(time.RFC3339),
			Entries:     entryResponses,
		})
	}

	totalPages := (total + int64(limit) - 1) / int64(limit)
	if total == 0 {
		totalPages = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": LedgerReportResponse{
			SystemBalance: systemBalance,
			SystemHealthy: systemBalance == 0,
			Summary: LedgerSummary{
				TotalEscrow:    escrowBal,
				TotalRevenue:   revenueBal,
				TotalMarketing: marketingBal,
			},
			Transactions: txnResponses,
			Meta: PaginationMeta{
				Total:      total,
				Page:       page,
				Limit:      limit,
				TotalPages: totalPages,
			},
		},
	})
}

// GetAccountBalanceHandler returns the balance for a specific account.
// GET /api/v1/admin/ledger/accounts/:id/balance
func (h *LedgerHandler) GetAccountBalanceHandler(c *gin.Context) {
	accountID := c.Param("id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "account ID is required"})
		return
	}

	balance, err := h.ledger.GetAccountBalance(accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"account_id": accountID,
			"balance":    balance,
		},
	})
}
