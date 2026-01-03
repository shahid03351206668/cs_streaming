package user

import (
	"errors"
	"mime/multipart"
	"tasksy/config"
	"tasksy/models"
	aws_services "tasksy/pkg"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service struct {
	db        *gorm.DB
	appConfig *config.Config
	s3Client  *aws_services.S3Client
}

type UserProfile struct {
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	Image         string    `json:"image"`
	PhoneVerified bool      `json:"phone_verified"`
	PhoneNo       string    `json:"phone_no"`
	JoinedAt      time.Time `json:"joined_at"`
	Rating        float64   `json:"rating"`
}

type UserProfileResponse struct {
	User    UserProfile     `json:"user"`
	Reviews []models.Review `json:"reviews"`
}

func NewService(db *gorm.DB, appConfig *config.Config, s3Client *aws_services.S3Client) *Service {
	return &Service{db: db, s3Client: s3Client, appConfig: appConfig}
}

func (s *Service) GetUserProfile(id string) (*UserProfileResponse, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}

	user.Password = ""
	var reviews []models.Review
	if err := s.db.Preload("Reviewer").Where("target_id = ? ", id).Order("created_at DESC").Find(&reviews).Error; err != nil {
		return nil, err
	}

	var reviewsRes []map[string]any
	var totalRating float64

	for _, i := range reviews {
		totalRating += float64(i.Rating)

		reviewsRes = append(reviewsRes, map[string]any{
			"id":         i.ID,
			"rating":     i.Rating,
			"comment":    i.Comment,
			"created_at": i.CreatedAt,
			"reviewed_by": map[string]string{
				"id":         i.Reviewer.ID,
				"last_name":  i.Reviewer.LastName,
				"first_name": i.Reviewer.FirstName,
			},
		})
	}

	userRating := 0.0
	if totalRating != 0 {
		userRating = totalRating / float64((len(reviewsRes)))
	}

	return &UserProfileResponse{
		User: UserProfile{
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			Email:         user.Email,
			Image:         user.ProfilePhoto,
			EmailVerified: user.EmailVerified,
			PhoneVerified: user.PhoneVerified,
			PhoneNo:       user.PhoneNumber,
			JoinedAt:      user.CreatedAt,
			Rating:        userRating,
		},
		Reviews: reviews,
	}, nil
}

type UserData struct {
	FirstName   string `form:"first_name" binding:"required"`
	LastName    string `form:"last_name"`
	Email       string `form:"email"`
	Password    string `form:"password" binding:"required,min=8"`
	PhoneNumber string `form:"phone_number"`
}

func (s *Service) CreateUser(data UserData, file *multipart.FileHeader) (*models.User, error) {

	var existingUser models.User
	if data.Email != "" {
		s.db.Where("email = ?", data.Email).First(&existingUser)
		if existingUser.ID != "" {
			return nil, errors.New("user with this email id already exists")
		}
	}

	if data.PhoneNumber != "" {
		s.db.Where("phone_number = ?", data.PhoneNumber).First(&existingUser)
		if existingUser.ID != "" {
			return nil, errors.New("user with this phone number already exists")
		}
	}

	imageURL := ""
	if file != nil {
		image, _ := file.Open()
		allowedTypes := map[string]bool{
			"image/jpeg": true,
			"image/jpg":  true,
			"image/png":  true,
			"image/webp": true,
		}

		fileType := file.Header.Get("Content-Type")
		if !allowedTypes[fileType] {
			return nil, errors.New("Invalid filetype for user profile image allowed types are [jpeg, jpg, png, webp]")
		}

		url, _, err := s.s3Client.UploadFile(image, file.Filename, fileType, "", "")
		if err != nil {
			return nil, err
		}
		imageURL = url
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := models.User{
		FirstName:     data.FirstName,
		LastName:      data.LastName,
		Email:         data.Email,
		PhoneVerified: true,
		PhoneNumber:   data.PhoneNumber,
		Password:      string(hashedPassword),
		ProfilePhoto:  imageURL,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// func (s *Service) UpdateUser(c *gin.Context) {
// 	var body struct {
// 		FirstName   string `form:"first_name"`
// 		LastName    string `form:"last_name"`
// 		PhoneNumber string `form:"phone_number"`
// 		Email       string `form:"email"`
// 	}\
// 	user := c.MustGet("user").(models.User)
// 	if err := c.ShouldBind(&body); err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"error":   err.Error(),
// 			"message": "Please provide valid profile data",
// 		})
// 		return
// 	}

// 	updates := make(map[string]interface{})

// 	if body.FirstName != "" {
// 		updates["first_name"] = body.FirstName
// 	}
// 	if body.LastName != "" {
// 		updates["last_name"] = body.LastName
// 	}

// 	// Validate and update phone number
// 	if body.PhoneNumber != "" {
// 		// Check if phone number is already taken by another user
// 		var existingUser models.User
// 		result := db.DB.Model(&models.User{}).
// 			Where("phone_number = ?", body.PhoneNumber).
// 			Where("id != ?", user.ID).
// 			First(&existingUser)

// 		if result.Error == nil {
// 			// Phone number exists and belongs to another user
// 			c.JSON(http.StatusConflict, gin.H{
// 				"message": "error",
// 				"error":   "phone number already taken",
// 			})
// 			return
// 		} else if result.Error != gorm.ErrRecordNotFound {
// 			// Database error (not "record not found")
// 			c.JSON(http.StatusInternalServerError, gin.H{
// 				"message": "error",
// 				"error":   "database error while checking phone number",
// 			})
// 			return
// 		}

// 		// Phone number is available
// 		updates["phone_number"] = body.PhoneNumber
// 		updates["phone_verified"] = false
// 	}

// 	// Validate and update email
// 	if body.Email != "" {
// 		// Check if email is already taken by another user
// 		var existingUser models.User
// 		result := db.DB.Model(&models.User{}).
// 			Where("email = ?", body.Email).
// 			Where("id != ?", user.ID).
// 			First(&existingUser)

// 		if result.Error == nil {
// 			c.JSON(http.StatusConflict, gin.H{
// 				"message": "error",
// 				"error":   "email already taken",
// 			})
// 			return
// 		} else if result.Error != gorm.ErrRecordNotFound {
// 			// Database error (not "record not found")
// 			c.JSON(http.StatusInternalServerError, gin.H{
// 				"message": "error",
// 				"error":   "database error while checking email",
// 			})
// 			return
// 		}

// 		// Email is available
// 		updates["email"] = body.Email
// 		updates["email_verified"] = false
// 	}

// 	// Handle profile photo upload
// 	file, err := c.FormFile("profile_photo")
// 	if err == nil && file != nil {
// 		allowedTypes := map[string]bool{
// 			"image/jpeg": true,
// 			"image/jpg":  true,
// 			"image/png":  true,
// 			"image/gif":  true,
// 			"image/webp": true,
// 		}

// 		contentType := file.Header.Get("Content-Type")
// 		if !allowedTypes[contentType] {
// 			c.JSON(http.StatusBadRequest, gin.H{
// 				"message": "Invalid file type. Only images are allowed",
// 			})
// 			return
// 		}

// 		maxFileSize := int64(5 * 1024 * 1024)
// 		if file.Size > maxFileSize {
// 			c.JSON(http.StatusBadRequest, gin.H{
// 				"message": "File size too large. Maximum 5MB allowed",
// 			})
// 			return
// 		}

// 		profilePhotoPath := filepath.Join(MEDIA_FILE_PATH, "profiles")
// 		if err := os.MkdirAll(profilePhotoPath, 0755); err != nil {
// 			c.JSON(http.StatusInternalServerError, gin.H{
// 				"message": "Failed to create profile photo directory",
// 				"error":   err.Error(),
// 			})
// 			return
// 		}

// 		// Delete old profile photo if exists
// 		if user.ProfilePhoto != "" {
// 			oldPhotoPath := user.ProfilePhoto
// 			if _, err := os.Stat(oldPhotoPath); err == nil {
// 				os.Remove(oldPhotoPath)
// 			}
// 		}

// 		ext := filepath.Ext(file.Filename)
// 		userIDStr := fmt.Sprintf("%v", user.ID)
// 		fileName := fmt.Sprintf("profile_%s_%d%s", userIDStr, time.Now().UnixNano(), ext)
// 		filePath := filepath.Join(profilePhotoPath, fileName)

// 		if err := c.SaveUploadedFile(file, filePath); err != nil {
// 			c.JSON(http.StatusInternalServerError, gin.H{
// 				"message": "Failed to upload profile photo",
// 				"error":   err.Error(),
// 			})
// 			return
// 		}

// 		updates["profile_photo"] = filePath
// 	}

// 	if len(updates) == 0 {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"message": "No fields to update",
// 		})
// 		return
// 	}

// 	// Update user profile
// 	if err := db.DB.Model(&user).Updates(updates).Error; err != nil {
// 		// Rollback: delete uploaded photo if database update fails
// 		if profilePhoto, ok := updates["profile_photo"].(string); ok {
// 			os.Remove(profilePhoto)
// 		}

// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"error":   err.Error(),
// 			"message": "Failed to update profile",
// 		})
// 		return
// 	}

// 	// Fetch updated user data
// 	var updatedUser models.User
// 	if err := db.DB.Where("id = ?", user.ID).First(&updatedUser).Error; err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"message": "Profile updated but failed to fetch updated data",
// 			"error":   err.Error(),
// 		})
// 		return
// 	}

// 	c.JSON(http.StatusOK, gin.H{
// 		"message": "Profile updated successfully",
// 		"user": gin.H{
// 			"id":            updatedUser.ID,
// 			"first_name":    updatedUser.FirstName,
// 			"last_name":     updatedUser.LastName,
// 			"email":         updatedUser.Email,
// 			"phone_number":  updatedUser.PhoneNumber,
// 			"profile_photo": updatedUser.ProfilePhoto,
// 		},
// 	})
// }
