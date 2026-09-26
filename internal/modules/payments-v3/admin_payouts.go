package paymentsv3

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tasksy/models"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
)

type adminUserRow struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	HasPayoutAccount bool   `json:"has_payout_account"`
}

type adminEscrowRow struct {
	ID               string       `json:"id"`
	Status           string       `json:"status"`
	Amount           float64      `json:"amount"`
	Currency         string       `json:"currency"`
	HeldAt           time.Time    `json:"held_at"`
	ReleasedAt       *time.Time   `json:"released_at"`
	StripeTransferID string       `json:"stripe_transfer_id"`
	ReleasedBy       string       `json:"released_by,omitempty"`
	ReleaseNote      string       `json:"release_note,omitempty"`
	Contract         gin.H        `json:"contract"`
	Freelancer       adminUserRow `json:"freelancer"`
	Client           adminUserRow `json:"client"`
	// CanRelease/BlockReason describe the admin override, which skips the
	// both-parties-complete rule but not disputes or closed contracts.
	CanRelease  bool   `json:"can_release"`
	BlockReason string `json:"block_reason,omitempty"`
}

func toAdminUser(u models.User) adminUserRow {
	return adminUserRow{
		ID:               u.ID,
		Name:             strings.TrimSpace(u.FirstName + " " + u.LastName),
		Email:            u.Email,
		HasPayoutAccount: u.StripeConnectAccountID != "",
	}
}

func pageParams(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

func pageMeta(total int64, page, limit int) gin.H {
	return gin.H{
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": (total + int64(limit) - 1) / int64(limit),
	}
}

// HandleAdminListEscrows lists escrowed payments with the context an admin
// needs to decide on a release. Filter with ?status=held|releasing|released|refunded.
func (h *Handler) HandleAdminListEscrows(c *gin.Context) {
	page, limit := pageParams(c)

	query := h.service.db.Model(&models.EscrowTransactionV3{})
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	escrows := make([]models.EscrowTransactionV3, 0)
	if err := query.Order("held_at DESC").Limit(limit).Offset((page - 1) * limit).Find(&escrows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	contractIDs := make([]string, 0, len(escrows))
	for _, e := range escrows {
		contractIDs = append(contractIDs, e.ContractID)
	}

	var contracts []models.Contract
	h.service.db.Preload("Client").Preload("Freelancer").Where("id IN ?", contractIDs).Find(&contracts)
	byID := make(map[string]models.Contract, len(contracts))
	for _, ct := range contracts {
		byID[ct.ID] = ct
	}

	type disputeCount struct {
		ContractID string
		N          int64
	}
	var counts []disputeCount
	h.service.db.Model(&models.Dispute{}).
		Select("contract_id, COUNT(*) AS n").
		Where("contract_id IN ? AND status = ?", contractIDs, models.DisputeStatusOpen).
		Group("contract_id").Scan(&counts)
	openDisputes := make(map[string]int64, len(counts))
	for _, dc := range counts {
		openDisputes[dc.ContractID] = dc.N
	}

	rows := make([]adminEscrowRow, 0, len(escrows))
	for _, e := range escrows {
		ct := byID[e.ContractID]
		amount, _ := e.Amount.Float64()
		row := adminEscrowRow{
			ID:               e.ID,
			Status:           e.Status,
			Amount:           amount,
			Currency:         e.Currency,
			HeldAt:           e.HeldAt,
			ReleasedAt:       e.ReleasedAt,
			StripeTransferID: e.StripeTransferID,
			ReleasedBy:       e.ReleasedBy,
			ReleaseNote:      e.ReleaseNote,
			Contract: gin.H{
				"id":                   ct.ID,
				"title":                ct.Title,
				"status":               ct.Status,
				"client_completed":     ct.ClientCompleted,
				"freelancer_completed": ct.FreelancerCompleted,
				"open_disputes":        openDisputes[e.ContractID],
			},
			Freelancer: toAdminUser(ct.Freelancer),
			Client:     toAdminUser(ct.Client),
		}

		switch {
		case e.Status != escrowHeld:
			row.BlockReason = "not held (" + e.Status + ")"
		case ct.Status == models.ContractStatusCancelled || ct.Status == models.ContractStatusTerminated:
			row.BlockReason = "contract " + ct.Status
		case openDisputes[e.ContractID] > 0 || ct.Status == models.ContractStatusDisputed:
			row.BlockReason = "open dispute"
		case ct.Freelancer.StripeConnectAccountID == "":
			row.BlockReason = "freelancer has no payout account"
		default:
			row.CanRelease = true
		}
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": rows, "meta": pageMeta(total, page, limit)})
}

// HandleAdminReleaseEscrow force-releases one escrow to the freelancer.
func (h *Handler) HandleAdminReleaseEscrow(c *gin.Context) {
	admin := c.MustGet("user").(models.User)

	var body struct {
		Note string `json:"note" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "a note explaining the release is required"})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey

	if err := h.service.AdminReleaseEscrow(c.Param("id"), admin.ID, strings.TrimSpace(body.Note)); err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, ErrNothingToRelease), errors.Is(err, ErrContractDisputed), errors.Is(err, ErrContractClosed):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "payment released to freelancer"})
}

// HandleAdminRefundContract refunds the client for a contract's payment —
// typically used to resolve a dispute in the admin panel's favour of the
// client. A reason is required and kept on the payment record for audit.
func (h *Handler) HandleAdminRefundContract(c *gin.Context) {
	admin := c.MustGet("user").(models.User)

	var body struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Reason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "a reason for the refund is required"})
		return
	}

	stripe.Key = h.config.Stripe.SecretKey

	refunded, err := h.service.AdminRefundContract(c.Param("id"), admin.ID, strings.TrimSpace(body.Reason))
	if err != nil {
		respondRefund(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "client refunded", "data": refunded})
}

func respondRefund(c *gin.Context, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, ErrAlreadyRefunded), errors.Is(err, ErrRefundStateChanged), errors.Is(err, ErrNoPaymentForContract):
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"message": "error", "error": err.Error()})
}

// HandleAdminListWithdrawals lists tasker withdrawals. Filter with ?status=.
func (h *Handler) HandleAdminListWithdrawals(c *gin.Context) {
	page, limit := pageParams(c)

	query := h.service.db.Model(&models.WithdrawalV3{})
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	withdrawals := make([]models.WithdrawalV3, 0)
	if err := query.Order("created_at DESC").Limit(limit).Offset((page - 1) * limit).Find(&withdrawals).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	userIDs := make([]string, 0, len(withdrawals))
	for _, w := range withdrawals {
		userIDs = append(userIDs, w.UserID)
	}
	var users []models.User
	h.service.db.Where("id IN ?", userIDs).Find(&users)
	byID := make(map[string]models.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	rows := make([]gin.H, 0, len(withdrawals))
	for _, w := range withdrawals {
		amount, _ := w.Amount.Float64()
		rows = append(rows, gin.H{
			"id":               w.ID,
			"user":             toAdminUser(byID[w.UserID]),
			"amount":           amount,
			"currency":         w.Currency,
			"status":           w.Status,
			"bank_last4":       w.BankLast4,
			"stripe_payout_id": w.StripePayoutID,
			"failure_message":  w.FailureMessage,
			"arrival_date":     w.ArrivalDate,
			"created_at":       w.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": rows, "meta": pageMeta(total, page, limit)})
}
