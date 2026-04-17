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
		ID          string `json:"id"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		Email       string `json:"email"`
		PhoneNumber string `json:"phone_number"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
		Disabled    bool   `json:"disabled"`
	}

	users := []UserResponse{}

	limit, err := strconv.ParseInt(c.Query("limit"), 10, 64)
	if err != nil {
		limit = 20
	}

	offset, err := strconv.ParseInt(c.Query("page"), 10, 64)
	if err != nil {
		offset = 0
	}

	if err := db.DB.Model(models.User{}).Find(&users).Limit(int(limit)).Offset(int(offset)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": users,
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

	type TxRow struct {
		ID                         string    `json:"id"`
		TransactionDate            time.Time `json:"transaction_date"`
		FromUserID                 string    `json:"from_user_id"`
		ToUserID                   string    `json:"to_user_id"`
		ReferenceType              string    `json:"reference_type"`
		ReferenceID                string    `json:"reference_id"`
		Amount                     int64     `json:"amount"`
		NetAmount                  int64     `json:"net_amount"`
		DiscountAmount             int64     `json:"discount_amount"`
		ReferralDiscountAmount     int64     `json:"referral_discount_amount"`
		AppFeeAmount               int64     `json:"app_fee_amount"`
		ClientCommissionAmount     int64     `json:"client_commission_amount"`
		FreelancerCommissionAmount int64     `json:"freelancer_commission_amount"`
		ReferralRewardAmount       int64     `json:"referral_reward_amount"`
		PaymentMethod              string    `json:"payment_method"`
		Currency                   string    `json:"currency"`
		Status                     string    `json:"status"`
		Direction                  string    `json:"direction"` // "paid" | "received"
	}

	var transactions []TxRow

	// Transactions where user paid
	var paid []TxRow
	db.DB.Model(&models.PaymentTransaction{}).
		Where("from_user_id = ?", userID).
		Select("id, transaction_date, from_user_id, to_user_id, reference_type, reference_id, amount, net_amount, discount_amount, referral_discount_amount, app_fee_amount, client_commission_amount, freelancer_commission_amount, referral_reward_amount, payment_method, currency, status").
		Order("transaction_date DESC").
		Find(&paid)
	for i := range paid {
		paid[i].Direction = "paid"
	}

	// Transactions where user earned
	var received []TxRow
	db.DB.Model(&models.PaymentTransaction{}).
		Where("to_user_id = ?", userID).
		Select("id, transaction_date, from_user_id, to_user_id, reference_type, reference_id, amount, net_amount, discount_amount, referral_discount_amount, app_fee_amount, client_commission_amount, freelancer_commission_amount, referral_reward_amount, payment_method, currency, status").
		Order("transaction_date DESC").
		Find(&received)
	for i := range received {
		received[i].Direction = "received"
	}

	transactions = append(paid, received...)

	// Summary
	var totalPaid, totalEarned, totalDiscounts, totalReferralDiscounts, totalAppFees, totalReferralRewards int64
	for _, t := range paid {
		if t.Status == "success" || t.Status == "completed" {
			totalPaid += t.Amount
			totalAppFees += t.ClientCommissionAmount
			totalDiscounts += t.DiscountAmount
			totalReferralDiscounts += t.ReferralDiscountAmount
		}
	}
	for _, t := range received {
		if t.Status == "success" || t.Status == "completed" {
			totalEarned += t.NetAmount
			totalReferralRewards += t.ReferralRewardAmount
		}
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
			"total_paid":               totalPaid,
			"total_earned":             totalEarned,
			"total_discounts":          totalDiscounts,
			"total_referral_discounts": totalReferralDiscounts,
			"total_app_fees_paid":      totalAppFees,
			"total_referral_rewards":   totalReferralRewards,
			"total_transactions":       len(transactions),
			"paid_count":               len(paid),
			"received_count":           len(received),
		},
		"transactions": transactions,
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

	var payments []models.PaymentTransaction
	if contractFound {
		db.DB.Where("reference_id = ? AND reference_type = ?", contract.ID, "contract").
			Preload("FromUser").
			Preload("ToUser").
			Order("transaction_date DESC").
			Find(&payments)
	}

	type PaymentRow struct {
		ID              string    `json:"id"`
		TransactionDate time.Time `json:"transaction_date"`
		FromUserID      string    `json:"from_user_id"`
		FromUserName    string    `json:"from_user_name"`
		ToUserID        string    `json:"to_user_id"`
		ToUserName      string    `json:"to_user_name"`
		Amount          int64     `json:"amount"`
		NetAmount       int64     `json:"net_amount"`
		AppFeeAmount    int64     `json:"app_fee_amount"`
		DiscountAmount  int64     `json:"discount_amount"`
		Currency        string    `json:"currency"`
		Status          string    `json:"status"`
		PaymentMethod   string    `json:"payment_method"`
	}

	paymentRows := make([]PaymentRow, 0, len(payments))
	for _, p := range payments {
		paymentRows = append(paymentRows, PaymentRow{
			ID:              p.ID,
			TransactionDate: p.TransactionDate,
			FromUserID:      p.FromUserID,
			FromUserName:    p.FromUser.FirstName + " " + p.FromUser.LastName,
			ToUserID:        p.ToUserID,
			ToUserName:      p.ToUser.FirstName + " " + p.ToUser.LastName,
			Amount:          p.Amount,
			NetAmount:       p.NetAmount,
			AppFeeAmount:    p.AppFeeAmount,
			DiscountAmount:  p.DiscountAmount,
			Currency:        p.Currency,
			Status:          p.Status,
			PaymentMethod:   p.PaymentMethod,
		})
	}

	resp := gin.H{
		"job":       job,
		"location":  location,
		"proposals": proposals,
		"payments":  paymentRows,
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

	var jobs []JobResponse
	if err := db.DB.Model(&models.JobPost{}).
		Select("id", "title", "description", "budget", "open_budget", "status", "address", "created_by_id", "category_id", "created_at", "updated_at").
		Limit(int(limit)).Offset(int(page * limit)).
		Order("created_at DESC").
		Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	var total int64
	db.DB.Model(&models.JobPost{}).Count(&total)

	c.JSON(http.StatusOK, gin.H{
		"data":  jobs,
		"total": total,
	})
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
		"application_fee_amount":           float64(settings.ApplicationFeeAmount),
		"app_fee_percentage":               settings.AppFeePercentage,
		"referral_discount_percentage":     settings.ReferralDiscountPercentage,
		"referral_reward_amount":           float64(settings.ReferralRewardAmount),
	}})
}

func UpdateSystemSettings(c *gin.Context) {
	var body struct {
		ClientCommissionPercentage     *float64 `json:"client_commission_percentage"`
		FreelancerCommissionPercentage *float64 `json:"freelancer_commission_percentage"`
		ApplicationFeeAmount           *float64 `json:"application_fee_amount"`
		AppFeePercentage               *float64 `json:"app_fee_percentage"`
		ReferralDiscountPercentage     *float64 `json:"referral_discount_percentage"`
		ReferralRewardAmount           *float64 `json:"referral_reward_amount"`
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
		"application_fee_amount":           float64(updated.ApplicationFeeAmount) / 100.0,
		"app_fee_percentage":               updated.AppFeePercentage,
		"referral_discount_percentage":     updated.ReferralDiscountPercentage,
		"referral_reward_amount":           float64(updated.ReferralRewardAmount) / 100.0,
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
