package user

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"tasksy/lib"
	"tasksy/models"
	"tasksy/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/webhook"
	"go.uber.org/zap"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetUserProfile(c *gin.Context) {
	paramID := c.Param("id")
	var id string

	if paramID != "" {
		id = paramID
	} else {
		user := c.MustGet("user").(models.User)
		id = user.ID
	}

	res, err := h.service.GetUserProfile(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   err.Error(),
			"message": "error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":    res.User,
		"reviews": res.Reviews,
	})
}

func (h *Handler) RegisterUser(c *gin.Context) {
	var data struct {
		UserData
		ReferralCode string `form:"referral_code" json:"referral_code"`
	}

	if err := c.ShouldBind(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	file, _ := c.FormFile("image")
	user, err := h.service.CreateUser(data.UserData, file)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	// Handle referral code if provided
	var referralApplied bool
	var referralError string
	if data.ReferralCode != "" {
		var refCode models.ReferralCode
		if err := h.service.db.Where("UPPER(code) = ? AND is_active = ?", strings.ToUpper(data.ReferralCode), true).First(&refCode).Error; err == nil {
			// Check if code is not expired
			if refCode.ExpiresAt == nil || refCode.ExpiresAt.After(time.Now()) {
				// Check max uses
				if refCode.MaxUses == -1 || refCode.CurrentUses < refCode.MaxUses {
					// Check user is not using their own code
					if refCode.OwnerID != user.ID {
						// Create referral usage
						usage := models.ReferralUsage{
							ReferralCodeID: refCode.ID,
							ReferrerID:     refCode.OwnerID,
							RefereeID:      user.ID,
							Status:         "pending",
							IsQualified:    false,
						}

						tx := h.service.db.Begin()
						if err := tx.Create(&usage).Error; err == nil {
							// Increment usage count
							if err := tx.Model(&models.ReferralCode{}).Where("id = ?", refCode.ID).
								Update("current_uses", refCode.CurrentUses+1).Error; err == nil {
								tx.Commit()
								referralApplied = true
							} else {
								tx.Rollback()
								referralError = "failed to update referral code usage"
							}
						} else {
							tx.Rollback()
							referralError = "failed to create referral usage"
						}
					} else {
						referralError = "cannot use your own referral code"
					}
				} else {
					referralError = "referral code has reached maximum uses"
				}
			} else {
				referralError = "referral code has expired"
			}
		} else {
			referralError = "invalid or inactive referral code"
		}
	}

	tokens, err := lib.GenerateAuthTokens(user.ID, 0)

	response := gin.H{
		"message": "success",
		"user": map[string]any{
			"id":            user.ID,
			"created_at":    user.CreatedAt,
			"updated_at":    user.UpdatedAt,
			"first_name":    user.FirstName,
			"last_name":     user.LastName,
			"email":         user.Email,
			"phone_number":  user.PhoneNumber,
			"profile_photo": user.ProfilePhoto,
		},
		"tokens": tokens,
	}

	if data.ReferralCode != "" {
		response["referral_applied"] = referralApplied
		if referralError != "" {
			response["referral_error"] = referralError
		}
	}

	c.JSON(http.StatusCreated, response)
}

func (h *Handler) StripeIdentityWebhookHandler(c *gin.Context) {
	const MaxRequestSize = int64(65536)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxRequestSize)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	endpointSecret := h.service.appConfig.Stripe.WebhookSecret

	if endpointSecret == "" {
		logger.Log.Error("Stripe Identity Webhook Secret is missing")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Configuration error"})
		return
	}

	event, err := webhook.ConstructEvent(body, c.GetHeader("Stripe-Signature"), endpointSecret)

	switch event.Type {
	case "identity.verification_session.verified":
		var session stripe.IdentityVerificationSession

		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			logger.Log.Error("Error parsing webhook JSON", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "JSON parsing error"})
			return
		}

		UserID := session.Metadata["user_id"]
		if UserID != "" {
			var user models.User
			if err := h.service.db.Where("id = ?", UserID).First(&user).Error; err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   err.Error(),
				})
				return
			}
			if user.ID == "" {
				logger.Log.Error("Webhook received but no user_id found in metadata")
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   fmt.Sprintf("invalid user ID %s", user.ID),
				})
				return
			}
			result := h.service.db.Select("id = ?", user.ID).Update("identity_verified", true)

			if result.Error != nil {
				logger.Log.Error("Database update failed", zap.Error(result.Error))
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "error",
					"error":   fmt.Sprintf("error while updating user %s ", err.Error()),
				})
				return
			}
		}
	default:
		fmt.Fprintf(os.Stdout, "Unhandled event type: %v", event.Type)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}

func (h *Handler) AddPortfolio(c *gin.Context) {
	var user models.User

	if err := h.service.db.Where("id = ?", c.Param("id")).Find(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(400, gin.H{"error": "File too large or invalid format"})
		return
	}

	var input models.Portfolio
	input.Title = c.PostForm("title")
	input.Description = c.PostForm("description")
	input.ProjectURL = c.PostForm("project_url")

	if input.Title == "" {
		c.JSON(400, gin.H{"error": "Title is required"})
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid form data"})
		return
	}

	result, err := h.service.AddPortfolio(&user, input, form.File["media"])
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    result,
	})

}

func (h *Handler) UpdatePortfolio(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	portfolioID := c.Param("id")

	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large or invalid data"})
		return
	}

	var input models.Portfolio
	input.Title = c.PostForm("title")
	input.Description = c.PostForm("description")
	input.ProjectURL = c.PostForm("project_url")
	keepIDsRaw := c.PostFormArray("keep_ids")

	var keepIDs []string
	for _, id := range keepIDsRaw {
		keepIDs = append(keepIDs, id)
	}

	form, _ := c.MultipartForm()
	newFiles := form.File["new_media"]

	updatedPortfolio, err := h.service.UpdatePortfolio(
		user.ID,
		portfolioID,
		input,
		keepIDs,
		newFiles,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Update failed",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Portfolio updated successfully",
		"data":    updatedPortfolio,
	})
}

func (h *Handler) GetPortfolio(c *gin.Context) {
	var data []models.Portfolio

	id := c.Param("id")

	if err := h.service.db.
		Preload("Media", "entity_type = ?", "portfolios").
		Where("user_id = ?", id).
		Find(&data).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    data,
	})

}

func (h *Handler) DeletePortfolio(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	err := h.service.DeletePortfolio(user.ID, c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "success",
			"error":   err,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    fmt.Sprintf("%s portfolio deleted", c.Param("id")),
	})
}

func (h *Handler) GetCertifications(c *gin.Context) {
	var results []models.Certification
	userID := c.Param("id")

	if err := h.service.db.Where("user_id = ?", userID).Find(&results).Error; err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"message": "error",
			"error":   err.Error(),
		})

		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    results,
	})
}

func (h *Handler) AddCertification(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	issueDate, _ := time.Parse("2006-01-02", c.PostForm("issue_date"))

	var expiryDate *time.Time
	if expStr := c.PostForm("expiration_date"); expStr != "" {
		t, _ := time.Parse("2006-01-02", expStr)
		expiryDate = &t
	}

	cert := models.Certification{
		Name:           c.PostForm("name"),
		IssuingOrg:     c.PostForm("issuing_organization"),
		IssueDate:      issueDate,
		ExpirationDate: expiryDate,
	}

	file, _ := c.FormFile("image")
	result, err := h.service.AddCertification(user.ID, cert, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, result)
}

func (h *Handler) UpdateCertification(c *gin.Context) {
	user := c.MustGet("user").(models.User)
	certID := c.Param("id")

	issueDate, _ := time.Parse("2006-01-02", c.PostForm("issue_date"))

	certData := models.Certification{
		Name:       c.PostForm("name"),
		IssuingOrg: c.PostForm("issuing_organization"),
		IssueDate:  issueDate,
	}

	file, _ := c.FormFile("image")

	result, err := h.service.UpdateCertification(user.ID, certID, certData, file)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) DeleteCertification(c *gin.Context) {
	id := c.Param("id")
	user := c.MustGet("user").(models.User)

	debug_message := fmt.Sprintf("user id: %s \n certification id %s ", user.ID, id)
	fmt.Println(debug_message)

	result := h.service.db.Where("id = ? AND user_id = ?", id, user.ID).Delete(&models.Certification{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": fmt.Sprintf("invalid certification id %s ", id)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"results": "certification deleted",
	})
}
