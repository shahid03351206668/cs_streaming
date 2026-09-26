package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"tasksy/db"
	paymentsv3 "tasksy/internal/modules/payments-v3"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

// proposalFieldName maps a Go struct field to the JSON key clients actually
// send, so validation errors are readable by whatever's showing them to a
// user instead of leaking Go/validator internals.
var proposalFieldName = map[string]string{
	"JobPostID":   "job_post_id",
	"CoverLetter": "cover_letter",
	"BidAmount":   "bid_amount",
	"Duration":    "duration",
}

// friendlyBindError turns a ShouldBindJSON error into a plain-language
// message: which field, and what's wrong with it.
func friendlyBindError(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		fe := ve[0]
		field := proposalFieldName[fe.Field()]
		if field == "" {
			field = fe.Field()
		}
		switch fe.Tag() {
		case "required":
			return field + " is required"
		case "gt":
			return field + " must be greater than " + fe.Param()
		default:
			return field + " is invalid"
		}
	}

	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return ute.Field + " has the wrong type (expected " + ute.Type.String() + ")"
	}

	if errors.Is(err, io.EOF) {
		return "request body is required"
	}

	return "invalid request body"
}

// dateTimeLayouts covers every format this API has been sent in practice:
// date-only, space-separated date+time (no timezone), and full RFC3339.
var dateTimeLayouts = []string{
	"2006-01-02",
	"2006-01-02 15:04:05",
	time.RFC3339,
}

// parseFlexibleDateTime returns nil for an empty string (field omitted), or a
// clear error naming the accepted formats if none of them match.
func parseFlexibleDateTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	for _, layout := range dateTimeLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return &t, nil
		}
	}
	return nil, errors.New("must be one of: YYYY-MM-DD, YYYY-MM-DD HH:MM:SS, or RFC3339")
}

func CreateProposal(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	if !user.IdentityVerfied {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "only verified user can send proposals on jobs",
		})
		return
	}
	var body struct {
		JobPostID           string `json:"job_post_id" binding:"required"`
		CoverLetter         string `json:"cover_letter" binding:"required"`
		AvailabilityDate    string `json:"availability_date"`
		AvailabilityDateEnd string `json:"availability_date_end"`

		BidAmount float64 `json:"bid_amount" binding:"required,gt=0"`
		Duration  int     `json:"duration" binding:"required,gt=0"`
		// Attachments      []string `json:"attachments"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   friendlyBindError(err),
		})
		return
	}

	availabilityDate, err := parseFlexibleDateTime(body.AvailabilityDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "availability_date " + err.Error()})
		return
	}
	availabilityDateEnd, err := parseFlexibleDateTime(body.AvailabilityDateEnd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "availability_date_end " + err.Error()})
		return
	}
	if availabilityDate != nil && availabilityDateEnd != nil && availabilityDateEnd.Before(*availabilityDate) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "availability_date_end must not be before availability_date"})
		return
	}

	jobPost := models.JobPost{}
	if err := db.DB.Where("id = ?", body.JobPostID).First(&jobPost).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "job post not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch job post",
		})
		return
	}

	if jobPost.Status != models.JobStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "job post is not accepting proposals",
		})
		return
	}

	if jobPost.CreatedByID == user.ID {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "you cannot bid on your own job post",
		})
		return
	}

	existingProposal := models.Proposal{}
	if err := db.DB.Where("job_post_id = ? AND freelancer_id = ?", body.JobPostID, user.ID).
		First(&existingProposal).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "you have already submitted a proposal for this job",
		})
		return
	}

	if !jobPost.OpenBudget && body.BidAmount > jobPost.Budget {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "bid amount exceeds job budget",
		})
		return
	}

	// Create proposal
	proposal := models.Proposal{
		JobPostID:        body.JobPostID,
		FreelancerID:     user.ID,
		AvailabilityDate: availabilityDate,

		AvailabilityDateEnd: availabilityDateEnd,
		CoverLetter:         body.CoverLetter,
		BidAmount:           body.BidAmount,
		Duration:            body.Duration,
		Status:              models.ProposalStatusPending,
	}

	tx := db.DB.Begin()

	if err := tx.Create(&proposal).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to create proposal",
		})
		return
	}

	// if len(body.Attachments) > 0 {
	// 	for _, attachmentURL := range body.Attachments {
	// 		attachment := models.ProposalAttachment{
	// 			ProposalID: proposal.ID,
	// 			URL:        attachmentURL,
	// 		}
	// 		if err := tx.Create(&attachment).Error; err != nil {
	// 			tx.Rollback()
	// 			c.JSON(http.StatusInternalServerError, gin.H{
	// 				"message": "error",
	// 				"error":   "failed to add attachments",
	// 			})
	// 			return
	// 		}
	// 	}
	// }

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to save proposal",
		})
		return
	}

	if err := db.DB.Preload("JobPost").
		Preload("JobPost.CreatedBy").
		Preload("ProposalAttachments").
		Where("id = ?", proposal.ID).
		First(&proposal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to load proposal details",
			"exce":    err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"data":    proposal,
	})

	go func(p models.Proposal, actor models.User) {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if notificationService != nil {
			_ = notificationService.NotifyProposalSent(ctx, actor.ID, p.JobPost.Title, p.ID, p.JobPostID)
			_ = notificationService.NotifyProposalReceived(ctx, p.JobPost.CreatedByID, p.JobPost.Title, p.ID, p.JobPostID)
		}
		if emailService != nil && p.JobPost.CreatedBy.Email != "" {
			_ = emailService.SendTemplatedEmail("proposal_received", p.JobPost.CreatedBy.Email, map[string]string{
				"first_name":      p.JobPost.CreatedBy.FirstName,
				"freelancer_name": actor.FirstName + " " + actor.LastName,
				"job_title":       p.JobPost.Title,
			})
		}
	}(proposal, user)
}

func UpdateProposal(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	proposalID := c.Param("id")

	if proposalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "proposal ID is required",
		})
		return
	}

	DB := db.DB

	proposal := models.Proposal{}

	if err := DB.Where("id = ?", proposalID).First(&proposal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "proposal not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch proposal",
		})
		return
	}

	if proposal.FreelancerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "you are not authorized to update this proposal",
		})
		return
	}

	if proposal.Status != models.ProposalStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "cannot update proposal with status: " + proposal.Status,
		})
		return
	}

	var body struct {
		CoverLetter         string     `json:"cover_letter"`
		BidAmount           float64    `json:"bid_amount"`
		Duration            int        `json:"duration"`
		AvailabilityDate    *time.Time `json:"availability_date"`
		AvailabilityDateEnd *time.Time `json:"availability_date_end"`
		Attachments         []string   `json:"attachments"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	updates := make(map[string]interface{})

	if body.CoverLetter != "" {
		updates["cover_letter"] = body.CoverLetter
	}

	if body.BidAmount > 0 {
		updates["bid_amount"] = body.BidAmount
	}

	if body.Duration > 0 {
		updates["duration"] = body.Duration
	}

	if body.AvailabilityDate != nil {
		updates["availability_date"] = body.AvailabilityDate
	}
	if body.AvailabilityDateEnd != nil {
		updates["availability_date_end"] = body.AvailabilityDateEnd

	}
	if len(updates) == 0 && len(body.Attachments) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "no valid fields to update",
		})
		return
	}

	// Update proposal
	if len(updates) > 0 {
		if err := db.DB.Model(&proposal).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"message": "error",
				"error":   "failed to update proposal",
			})
			return
		}
	}

	// Reload with associations
	if err := db.DB.Preload("JobPost").
		Preload("Freelancer").
		Preload("ProposalAttachments").
		First(&proposal, proposalID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to reload proposal",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    proposal,
	})
}

func WithdrawProposal(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	proposalID := c.Param("id")
	if proposalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "proposal ID is required",
		})
		return
	}

	// Find proposal
	proposal := models.Proposal{}
	if err := db.DB.Where("id = ?", proposalID).First(&proposal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "proposal not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch proposal",
		})
		return
	}

	if proposal.FreelancerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "you are not authorized to withdraw this proposal",
		})
		return
	}

	if proposal.Status != models.ProposalStatusPending &&
		proposal.Status != models.ProposalStatusShortlisted {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "cannot withdraw proposal with status: " + proposal.Status,
		})
		return
	}

	if err := db.DB.Model(&proposal).Update("status", models.ProposalStatusWithdrawn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to withdraw proposal",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"proposal_id": proposalID,
			"status":      models.ProposalStatusWithdrawn,
		},
	})
}

// DeleteProposal - Hard delete a proposal (only if pending/withdrawn)
func DeleteProposal(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	proposalID := c.Param("id")
	if proposalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "proposal ID is required",
		})
		return
	}

	// Find proposal
	proposal := models.Proposal{}
	if err := db.DB.Where("id = ?", proposalID).First(&proposal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "proposal not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch proposal",
		})
		return
	}

	// Check ownership
	if proposal.FreelancerID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "you are not authorized to delete this proposal",
		})
		return
	}

	// Only allow deletion of pending or withdrawn proposals
	if proposal.Status != models.ProposalStatusPending &&
		proposal.Status != models.ProposalStatusWithdrawn {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "cannot delete proposal with status: " + proposal.Status,
		})
		return
	}

	// Delete proposal (cascade will delete attachments)
	if err := db.DB.Delete(&proposal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to delete proposal",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"proposal_id": proposalID,
			"deleted":     true,
		},
	})
}

func GetProposal(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	proposalID := c.Param("id")
	if proposalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "proposal ID is required",
		})
		return
	}

	proposal := models.Proposal{}
	if err := db.DB.Preload("JobPost").
		Preload("JobPost.CreatedBy").
		Preload("Freelancer").
		Preload("ProposalAttachments").
		Where("id = ?", proposalID).
		First(&proposal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "proposal not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch proposal",
		})
		return
	}

	if proposal.FreelancerID != user.ID && proposal.JobPost.CreatedByID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "you are not authorized to view this proposal",
		})
		return
	}

	settings, err := paymentsv3.NewService(db.DB).GetSystemSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to load system settings",
		})
		return
	}

	fees := paymentsv3.CalculateFees(proposal.BidAmount, settings)
	summary := fees.Summary()
	// Legacy flat keys kept so existing clients keep working.
	summary["commission_pct"] = fees.ClientCommissionPct.InexactFloat64()
	summary["commission_amount"] = fees.ClientCommission.InexactFloat64()
	summary["app_fees"] = fees.ClientPlatformFee.InexactFloat64()
	summary["grand_total"] = fees.ClientTotal.InexactFloat64()

	c.JSON(http.StatusOK, gin.H{
		"message":         "success",
		"data":            proposal,
		"payment_summary": summary,
	})
}

func GetMyProposals(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	status := c.Query("status")
	DB := db.DB

	query := DB.Preload("JobPost").
		Preload("JobPost.Category").
		Preload("JobPost.CreatedBy").
		Preload("Freelancer").
		Preload("ProposalAttachments").
		Where("freelancer_id = ?", user.ID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var proposals []models.Proposal
	if err := query.Order("created_at DESC").Find(&proposals).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch proposals",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    proposals,
		"count":   len(proposals),
	})
}

func GetJobProposals(c *gin.Context) {
	user := c.MustGet("user").(models.User)

	jobPostID := c.Param("id")

	if jobPostID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "Job id is required",
		})
		return
	}

	jobPost := models.JobPost{}
	if err := db.DB.Where("id = ?", jobPostID).First(&jobPost).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "error",
				"error":   "job post not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch job post",
		})
		return
	}

	if jobPost.CreatedByID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"message": "error",
			"error":   "you are not authorized to view proposals for this job",
		})
		return
	}

	var proposals []models.Proposal
	if err := db.DB.Preload("Freelancer").
		Preload("ProposalAttachments").
		Where("job_post_id = ?", jobPostID).
		Order("created_at DESC").
		Find(&proposals).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   "failed to fetch proposals",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    proposals,
		"count":   len(proposals),
	})
}

func ManageProposalDecision(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	proposalID := c.Param("id")

	var body struct {
		Status string `json:"status" binding:"required,oneof=accepted rejected"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid status. Allowed values: 'accepted', 'rejected'",
		})
		return
	}

	DB := db.DB
	targetStatus := body.Status
	var proposal models.Proposal
	if err := DB.Preload("JobPost").Preload("Freelancer").First(&proposal, "id = ?", proposalID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proposal not found"})
		return
	}

	if proposal.JobPost.CreatedByID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to manage this proposal"})
		return
	}

	if proposal.Status != models.ProposalStatusPending && proposal.Status != models.ProposalStatusShortlisted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot change status of a proposal that is " + proposal.Status})
		return
	}

	if targetStatus == models.ProposalStatusRejected {
		if err := DB.Model(&proposal).Update("status", models.ProposalStatusRejected).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reject proposal"})
			return
		}

		go func(p models.Proposal) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if notificationService != nil {
				_ = notificationService.NotifyProposalDecision(ctx, p.FreelancerID, models.ProposalStatusRejected, p.ID, p.JobPostID)
			}
			if emailService != nil && p.Freelancer.Email != "" {
				_ = emailService.SendTemplatedEmail("proposal_rejected", p.Freelancer.Email, map[string]string{
					"first_name": p.Freelancer.FirstName,
					"job_title":  p.JobPost.Title,
				})
			}
		}(proposal)

		c.JSON(http.StatusOK, gin.H{
			"message": "Proposal rejected",
			"status":  models.ProposalStatusRejected,
		})
		return
	}

	if targetStatus == models.ProposalStatusAccepted {
		if proposal.JobPost.Status != models.JobStatusOpen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Job is not open, cannot accept new proposals"})
			return
		}

		tx := DB.Begin()
		if err := tx.Model(&proposal).Update("status", models.ProposalStatusAccepted).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update proposal status"})
			return
		}

		if err := tx.Model(&models.JobPost{BaseModel: models.BaseModel{ID: proposal.JobPostID}}).
			Update("status", models.JobStatusInProgress).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to lock job"})
			return
		}

		// startDate := time.Now()
		// endDate := startDate.AddDate(0, 0, proposal.Duration)
		// contract := models.Contract{
		//     JobPostID:    proposal.JobPostID,
		//     ProposalID:   proposal.ID,
		//     ClientID:     user.ID,
		//     FreelancerID: proposal.FreelancerID,
		//     Title:        proposal.JobPost.Title,
		//     Description:  proposal.CoverLetter,
		//     TotalAmount:  proposal.BidAmount,
		//     StartDate:    startDate,
		//     EndDate:      endDate,
		//     Status:       models.ContractStatusActive,
		// }

		// if err := tx.Create(&contract).Error; err != nil {
		//     tx.Rollback()
		//     c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate contract"})
		//     return
		// }

		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed"})
			return
		}

		go func(p models.Proposal) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if notificationService != nil {
				_ = notificationService.NotifyProposalDecision(ctx, p.FreelancerID, models.ProposalStatusAccepted, p.ID, p.JobPostID)
			}
			if emailService != nil && p.Freelancer.Email != "" {
				_ = emailService.SendTemplatedEmail("proposal_accepted", p.Freelancer.Email, map[string]string{
					"first_name": p.Freelancer.FirstName,
					"job_title":  p.JobPost.Title,
				})
			}
		}(proposal)

		c.JSON(http.StatusOK, gin.H{
			"message": "Proposal accepted and contract created",
			"status":  models.ProposalStatusAccepted,
			// "contract_id":  contract.ID,
			"job_status": models.JobStatusInProgress,
		})
		return
	}
}
