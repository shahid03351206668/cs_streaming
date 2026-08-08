package email

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

func (h *Handler) ListEmailAccounts(c *gin.Context) {
	var accounts []models.EmailAccount
	if err := h.service.db.Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, accounts)
}

func (h *Handler) CreateEmailAccount(c *gin.Context) {
	var input struct {
		Name      string `json:"name" binding:"required"`
		Host      string `json:"host" binding:"required"`
		Port      int    `json:"port" binding:"required"`
		Email     string `json:"email" binding:"required"`
		Password  string `json:"password" binding:"required"`
		FromName  string `json:"from_name" binding:"required"`
		IsDefault bool   `json:"is_default"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.IsDefault {
		h.service.db.Model(&models.EmailAccount{}).
			Where("is_default = true").
			Update("is_default", false)
	}

	account := models.EmailAccount{
		Name:      input.Name,
		Host:      input.Host,
		Port:      input.Port,
		Email:     input.Email,
		Password:  input.Password,
		FromName:  input.FromName,
		IsActive:  true,
		IsDefault: input.IsDefault,
	}

	if err := h.service.db.Create(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	account.Password = "" // never return password
	c.JSON(http.StatusCreated, account)
}

func (h *Handler) UpdateEmailAccount(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Email    string `json:"email"`
		Password string `json:"password"`
		FromName string `json:"from_name"`
		IsActive *bool  `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Host != "" {
		updates["host"] = input.Host
	}
	if input.Port != 0 {
		updates["port"] = input.Port
	}
	if input.Email != "" {
		updates["email"] = input.Email
	}
	if input.Password != "" {
		updates["password"] = input.Password
	}
	if input.FromName != "" {
		updates["from_name"] = input.FromName
	}
	if input.IsActive != nil {
		updates["is_active"] = *input.IsActive
	}

	var existing models.EmailAccount
	if err := h.service.db.First(&existing, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "email account not found"})
		return
	}

	if err := h.service.db.Model(&models.EmailAccount{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.service.InvalidateEmailAccountCache(existing.Email)
	if input.Email != "" && input.Email != existing.Email {
		h.service.InvalidateEmailAccountCache(input.Email)
	}
	if existing.IsDefault {
		h.service.InvalidateEmailAccountCache("")
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *Handler) DeleteEmailAccount(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.db.Delete(&models.EmailAccount{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *Handler) SetDefaultEmailAccount(c *gin.Context) {
	id := c.Param("id")
	h.service.db.Model(&models.EmailAccount{}).
		Where("is_default = true").
		Update("is_default", false)

	if err := h.service.db.Model(&models.EmailAccount{}).
		Where("id = ?", id).
		Update("is_default", true).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.service.InvalidateEmailAccountCache("")
	c.JSON(http.StatusOK, gin.H{"message": "default updated"})
}

func (h *Handler) ListEmailTemplates(c *gin.Context) {
	var templates []models.EmailTemplate
	if err := h.service.db.Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, templates)
}

func (h *Handler) CreateEmailTemplate(c *gin.Context) {
	var input struct {
		Name    string `json:"name" binding:"required"`
		Subject string `json:"subject" binding:"required"`
		Body    string `json:"body" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tmpl := models.EmailTemplate{
		Name:    input.Name,
		Subject: input.Subject,
		Body:    input.Body,
	}

	if err := h.service.db.Create(&tmpl).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tmpl)
}

func (h *Handler) UpdateEmailTemplate(c *gin.Context) {
	id := c.Param("id")

	var input struct {
		Name    string `json:"name"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Subject != "" {
		updates["subject"] = input.Subject
	}
	if input.Body != "" {
		updates["body"] = input.Body
	}
	if err := h.service.db.Model(&models.EmailTemplate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *Handler) DeleteEmailTemplate(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.db.Delete(&models.EmailTemplate{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *Handler) SendEmail(c *gin.Context) {
	var req struct {
		Receiver string `json:"receiver" binding:"required,email"`
		Subject  string `json:"subject" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err.Error()})
		return
	}

	acc, err := h.service.GetEmailAccount("noreply@tasksy.co.uk")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": "no default email account configured"})
		return
	}

	if err := h.service.SendMail(acc.Email, req.Receiver, req.Subject, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "error", "error": err.Error(), "service": "send email service"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) SendTestEmail(c *gin.Context) {
	var body struct {
		To string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to field is required"})
		return
	}

	if err := h.service.SendMail("noreply@tasksy.co.uk", body.To, "Tasksy Test Email", "<p>This is a test email from <strong>Tasksy</strong>. If you received this, SMTP is working correctly.</p>"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Test email sent to " + body.To})
}
