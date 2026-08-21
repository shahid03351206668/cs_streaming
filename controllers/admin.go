package controllers

import (
	"net/http"
	"strconv"
	"tasksy/db"
	"tasksy/internal/modules/payments"
	"tasksy/lib"
	"tasksy/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func GetUsers(c *gin.Context) {
	type UserResponse struct {
		ID          string `json:"id"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}

	var users []UserResponse
	DB := db.DB

	if err := DB.Model(&models.User{}).
		Select("id", "first_name", "last_name", "email", "phone_number", "created_at", "updated_at").
		Find(&users).
		Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	if users == nil {
		users = make([]UserResponse, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"users":   users,
	})
}

func ListRecords(c *gin.Context) {
	model := c.Param("model")
	if model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "model is missing in the query params",
		})
		return
	}

}

func AdminUserListController(c *gin.Context) {
	type UserResponse struct {
		ID            string `json:"id"`
		FirstName     string `json:"first_name"`
		LastName      string `json:"last_name"`
		Email         string `json:"email"`
		PhoneNumber   string `json:"phone_number"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Disabled      bool   `json:"disabled"`
		EmailVerified bool   `json:"email_verified"`
		PhoneVerified bool   `json:"phone_verified"`
	}

	limit, err := strconv.ParseInt(c.Query("limit"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 20
	}
	page, err := strconv.ParseInt(c.Query("page"), 10, 64)
	if err != nil || page < 0 {
		page = 0
	}

	query := db.DB.Model(&models.User{})

	if status := c.Query("status"); status == "active" {
		query = query.Where("disabled = ?", false)
	} else if status == "disabled" {
		query = query.Where("disabled = ?", true)
	}
	if emailVerified := c.Query("email_verified"); emailVerified != "" {
		query = query.Where("email_verified = ?", emailVerified == "true")
	}
	if phoneVerified := c.Query("phone_verified"); phoneVerified != "" {
		query = query.Where("phone_verified = ?", phoneVerified == "true")
	}
	if fromDate := c.Query("from_date"); fromDate != "" {
		if t, err := time.Parse("2006-01-02", fromDate); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if toDate := c.Query("to_date"); toDate != "" {
		if t, err := time.Parse("2006-01-02", toDate); err == nil {
			query = query.Where("created_at <= ?", t.Add(24*time.Hour))
		}
	}
	if search := c.Query("search"); search != "" {
		pattern := "%" + search + "%"
		query = query.Where(
			"first_name ILIKE ? OR last_name ILIKE ? OR email ILIKE ? OR phone_number ILIKE ?",
			pattern, pattern, pattern, pattern,
		)
	}

	var total int64
	query.Count(&total)

	users := []UserResponse{}
	if err := query.
		Limit(int(limit)).Offset(int(page * limit)).
		Order("created_at DESC").
		Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	c.JSON(http.StatusOK, gin.H{
		"data": users,
		"meta": gin.H{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

func AdminGetUserController(c *gin.Context) {
	userID := c.Param("id")
	var user models.User

	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "User not found",
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": user,
	})
}
func AdminUserCreateController(c *gin.Context) {
	var body struct {
		FirstName   string `json:"first_name" binding:"required,min=2"`
		LastName    string `json:"last_name" binding:"required,min=2"`
		Email       string `json:"email" binding:"required,email"`
		PhoneNumber string `json:"phone_number" binding:"required,min=6"`
		Password    string `json:"password" binding:"required,min=6"`

		PhoneVerified bool `json:"phone_verified"`
		EmailVerified bool `json:"email_verified"`
		Disabled      bool `json:"disabled"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	DB := db.DB

	hashedPassword := ""
	if body.Password != "" {
		hashedPassword = lib.MakePassword(body.Password)
	}

	err := DB.Create(models.User{
		FirstName:     body.FirstName,
		LastName:      body.LastName,
		Email:         body.Email,
		Password:      hashedPassword,
		PhoneNumber:   body.PhoneNumber,
		PhoneVerified: body.PhoneVerified,
		EmailVerified: body.EmailVerified,
	}).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
	})

}

func AdminUpdateUserController(c *gin.Context) {
	userID := c.Param("id")

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "user not found"})
		return
	}

	var body struct {
		FirstName        *string `json:"first_name"`
		LastName         *string `json:"last_name"`
		Email            *string `json:"email"`
		PhoneNumber      *string `json:"phone_number"`
		Disabled         *bool   `json:"disabled"`
		EmailVerified    *bool   `json:"email_verified"`
		PhoneVerified    *bool   `json:"phone_verified"`
		IdentityVerified *bool   `json:"identity_verified"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if body.FirstName != nil {
		updates["first_name"] = *body.FirstName
	}
	if body.LastName != nil {
		updates["last_name"] = *body.LastName
	}
	if body.Email != nil {
		updates["email"] = *body.Email
	}
	if body.PhoneNumber != nil {
		updates["phone_number"] = *body.PhoneNumber
	}
	if body.Disabled != nil {
		updates["disabled"] = *body.Disabled
	}
	if body.EmailVerified != nil {
		updates["email_verified"] = *body.EmailVerified
	}
	if body.PhoneVerified != nil {
		updates["phone_verified"] = *body.PhoneVerified
	}
	if body.IdentityVerified != nil {
		updates["identity_verified"] = *body.IdentityVerified
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "no fields to update"})
		return
	}

	if err := db.DB.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": user})
}

func AdminChangeUserPasswordController(c *gin.Context) {
	userID := c.Param("id")

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "user not found"})
		return
	}

	var body struct {
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	hashed := lib.MakePassword(body.NewPassword)
	if err := db.DB.Model(&user).UpdateColumn("password", hashed).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func AdminUserWalletController(c *gin.Context) {
	userID := c.Param("id")

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":                      user.ID,
			"first_name":              user.FirstName,
			"last_name":               user.LastName,
			"email":                   user.Email,
			"phone_number":            user.PhoneNumber,
			"profile_photo":           user.ProfilePhoto,
			"email_verified":          user.EmailVerified,
			"referral_reward_balance": user.ReferralRewardBalance,
		},
		"summary": gin.H{
			"total_paid":               0,
			"total_earned":             0,
			"total_discounts":          0,
			"total_referral_discounts": 0,
			"total_app_fees_paid":      0,
			"total_referral_rewards":   0,
			"total_transactions":       0,
			"paid_count":               0,
			"received_count":           0,
		},
		"transactions": []interface{}{},
	})
}

func AdminUpdateJobController(c *gin.Context) {
	jobID := c.Param("id")

	var job models.JobPost
	if err := db.DB.First(&job, "id = ?", jobID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "job not found"})
		return
	}

	var body struct {
		Title       *string  `json:"title"`
		Description *string  `json:"description"`
		Budget      *float64 `json:"budget"`
		OpenBudget  *bool    `json:"open_budget"`
		Address     *string  `json:"address"`
		Status      *string  `json:"status"`
		CategoryID  *string  `json:"category_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if body.Title != nil {
		updates["title"] = *body.Title
	}
	if body.Description != nil {
		updates["description"] = *body.Description
	}
	if body.Budget != nil {
		updates["budget"] = *body.Budget
	}
	if body.OpenBudget != nil {
		updates["open_budget"] = *body.OpenBudget
	}
	if body.Address != nil {
		updates["address"] = *body.Address
	}
	if body.Status != nil {
		updates["status"] = *body.Status
	}
	if body.CategoryID != nil {
		updates["category_id"] = *body.CategoryID
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "no fields to update"})
		return
	}

	if err := db.DB.Model(&job).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": job})
}

func AdminGetJobDetailController(c *gin.Context) {
	jobID := c.Param("id")

	var job models.JobPost
	if err := db.DB.
		Preload("CreatedBy").
		Preload("Category").
		Preload("JobMedia").
		First(&job, "id = ?", jobID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "job not found"})
		return
	}

	var location models.JobPostLocation
	db.DB.Where("job_post_id = ?", jobID).First(&location)

	var proposals []models.Proposal
	db.DB.Where("job_post_id = ?", jobID).
		Preload("Freelancer").
		Preload("ProposalAttachments").
		Order("created_at DESC").
		Find(&proposals)

	var contract models.Contract
	contractFound := db.DB.Where("job_post_id = ?", jobID).
		Preload("Client").
		Preload("Freelancer").
		Preload("Proposal").
		First(&contract).Error == nil

	var reports []models.JobReport
	db.DB.Where("job_post_id = ?", jobID).
		Preload("Reporter").
		Order("created_at DESC").
		Find(&reports)

	resp := gin.H{
		"job":       job,
		"location":  location,
		"proposals": proposals,
		"payments":  []interface{}{},
		"reports":   reports,
	}
	if contractFound {
		resp["contract"] = contract
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func AdminListJobsController(c *gin.Context) {
	type JobResponse struct {
		ID          string  `json:"id"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Budget      float64 `json:"budget"`
		OpenBudget  bool    `json:"open_budget"`
		Status      string  `json:"status"`
		Address     string  `json:"address"`
		CreatedByID string  `json:"created_by_id"`
		CategoryID  string  `json:"category_id"`
		CreatedAt   string  `json:"created_at"`
		UpdatedAt   string  `json:"updated_at"`
	}

	limit, err := strconv.ParseInt(c.Query("limit"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 20
	}
	page, err := strconv.ParseInt(c.Query("page"), 10, 64)
	if err != nil || page < 0 {
		page = 0
	}

	query := db.DB.Model(&models.JobPost{})

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if categoryID := c.Query("category_id"); categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}
	if openBudget := c.Query("open_budget"); openBudget != "" {
		query = query.Where("open_budget = ?", openBudget == "true")
	}
	if minBudget := c.Query("min_budget"); minBudget != "" {
		query = query.Where("budget >= ?", minBudget)
	}
	if maxBudget := c.Query("max_budget"); maxBudget != "" {
		query = query.Where("budget <= ?", maxBudget)
	}
	if fromDate := c.Query("from_date"); fromDate != "" {
		if t, err := time.Parse("2006-01-02", fromDate); err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if toDate := c.Query("to_date"); toDate != "" {
		if t, err := time.Parse("2006-01-02", toDate); err == nil {
			query = query.Where("created_at <= ?", t.Add(24*time.Hour))
		}
	}
	if search := c.Query("search"); search != "" {
		pattern := "%" + search + "%"
		query = query.Where("title ILIKE ? OR address ILIKE ?", pattern, pattern)
	}

	var total int64
	query.Count(&total)

	var jobs []JobResponse
	if err := query.
		Select("id", "title", "description", "budget", "open_budget", "status", "address", "created_by_id", "category_id", "created_at", "updated_at").
		Limit(int(limit)).Offset(int(page * limit)).
		Order("created_at DESC").
		Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	if jobs == nil {
		jobs = make([]JobResponse, 0)
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  jobs,
		"total": total,
		"meta": gin.H{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// ─── Job Reports ────────────────────────────────────────────────────────────

func AdminListJobReportsController(c *gin.Context) {
	limit, err := strconv.ParseInt(c.Query("limit"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 20
	}
	page, err := strconv.ParseInt(c.Query("page"), 10, 64)
	if err != nil || page < 0 {
		page = 0
	}

	query := db.DB.Model(&models.JobReport{})

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if jobPostID := c.Query("job_post_id"); jobPostID != "" {
		query = query.Where("job_post_id = ?", jobPostID)
	}

	var total int64
	query.Count(&total)

	var reports []models.JobReport
	if err := query.
		Preload("Reporter").
		Preload("JobPost").
		Order("created_at DESC").
		Limit(int(limit)).Offset(int(page * limit)).
		Find(&reports).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	if reports == nil {
		reports = make([]models.JobReport, 0)
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	c.JSON(http.StatusOK, gin.H{
		"data": reports,
		"meta": gin.H{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

func AdminUpdateJobReportController(c *gin.Context) {
	id := c.Param("id")

	var body struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	switch body.Status {
	case models.JobReportStatusPending, models.JobReportStatusReviewed, models.JobReportStatusDismissed:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status", "message": "error"})
		return
	}

	if err := db.DB.Model(&models.JobReport{}).Where("id = ?", id).Update("status", body.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// ─── System Settings ────────────────────────────────────────────────────────

func GetSystemSettings(c *gin.Context) {
	settings := models.SystemSettings{ID: "system_settings"}
	if err := db.DB.FirstOrCreate(&settings, "id = ?", "system_settings").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{
		"id":                               settings.ID,
		"client_commission_percentage":     settings.ClientCommissionPercentage,
		"freelancer_commission_percentage": settings.FreelancerCommissionPercentage,
		"application_fee_amount":           settings.ApplicationFeeAmount,
		"app_fee_percentage":               settings.AppFeePercentage,
		"referral_discount_percentage":     settings.ReferralDiscountPercentage,
		"referral_reward_amount":           settings.ReferralRewardAmount,
	}})
}

func UpdateSystemSettings(c *gin.Context) {
	var body struct {
		ClientCommissionPercentage     *float64         `json:"client_commission_percentage"`
		FreelancerCommissionPercentage *float64         `json:"freelancer_commission_percentage"`
		ApplicationFeeAmount           *decimal.Decimal `json:"application_fee_amount"`
		AppFeePercentage               *float64         `json:"app_fee_percentage"`
		ReferralDiscountPercentage     *float64         `json:"referral_discount_percentage"`
		ReferralRewardAmount           *decimal.Decimal `json:"referral_reward_amount"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if body.ClientCommissionPercentage != nil {
		updates["client_commission_percentage"] = *body.ClientCommissionPercentage
	}
	if body.FreelancerCommissionPercentage != nil {
		updates["freelancer_commission_percentage"] = *body.FreelancerCommissionPercentage
	}
	if body.ApplicationFeeAmount != nil {
		updates["application_fee_amount"] = body.ApplicationFeeAmount
	}
	if body.AppFeePercentage != nil {
		updates["app_fee_percentage"] = *body.AppFeePercentage
	}
	if body.ReferralDiscountPercentage != nil {
		updates["referral_discount_percentage"] = *body.ReferralDiscountPercentage
	}

	if body.ReferralRewardAmount != nil {
		updates["referral_reward_amount"] = body.ReferralRewardAmount
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "no fields to update"})
		return
	}

	if err := db.DB.Model(&models.SystemSettings{}).
		Where("id = ?", "system_settings").
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}

	// Clear the in-process settings cache so payment calculations use the new rates immediately.
	payments.InvalidateSettingsCache()

	var updated models.SystemSettings
	db.DB.FirstOrCreate(&updated, "id = ?", "system_settings")
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{
		"id":                               updated.ID,
		"client_commission_percentage":     updated.ClientCommissionPercentage,
		"freelancer_commission_percentage": updated.FreelancerCommissionPercentage,
		"application_fee_amount":           updated.ApplicationFeeAmount,
		"app_fee_percentage":               updated.AppFeePercentage,
		"referral_discount_percentage":     updated.ReferralDiscountPercentage,
		"referral_reward_amount":           updated.ReferralRewardAmount,
	}})
}

// ─── Banks ──────────────────────────────────────────────────────────────────

func ListBanks(c *gin.Context) {
	var banks []models.Bank
	if err := db.DB.Where("is_active = ?", true).Order("name ASC").Find(&banks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": banks})
}

func AdminListBanks(c *gin.Context) {
	var banks []models.Bank
	if err := db.DB.Order("name ASC").Find(&banks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": banks})
}

func AdminCreateBank(c *gin.Context) {
	var body struct {
		Name     string `json:"name" binding:"required"`
		SortCode string `json:"sort_code"`
		LogoURL  string `json:"logo_url"`
		IsActive *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	isActive := true
	if body.IsActive != nil {
		isActive = *body.IsActive
	}

	bank := models.Bank{
		Name:     body.Name,
		SortCode: body.SortCode,
		LogoURL:  body.LogoURL,
		IsActive: isActive,
	}
	if err := db.DB.Create(&bank).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "success", "data": bank})
}

func AdminUpdateBank(c *gin.Context) {
	bankID := c.Param("id")

	var bank models.Bank
	if err := db.DB.First(&bank, "id = ?", bankID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "bank not found"})
		return
	}

	var body struct {
		Name     *string `json:"name"`
		SortCode *string `json:"sort_code"`
		LogoURL  *string `json:"logo_url"`
		IsActive *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if body.Name != nil {
		updates["name"] = *body.Name
	}
	if body.SortCode != nil {
		updates["sort_code"] = *body.SortCode
	}
	if body.LogoURL != nil {
		updates["logo_url"] = *body.LogoURL
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}

	if err := db.DB.Model(&bank).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": bank})
}

func AdminDeleteBank(c *gin.Context) {
	bankID := c.Param("id")

	var bank models.Bank
	if err := db.DB.First(&bank, "id = ?", bankID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "error": "bank not found"})
		return
	}

	if err := db.DB.Delete(&bank).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}
