package user

import (
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"tasksy/config"
	"tasksy/models"
	aws_services "tasksy/pkg"
	"tasksy/pkg/logger"
	"time"

	"go.uber.org/zap"
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

func NewService(db *gorm.DB, appConfig *config.Config, s3Client *aws_services.S3Client) *Service {
	return &Service{db: db, s3Client: s3Client, appConfig: appConfig}
}

func (s *Service) UpsertDeviceToken(userID, token, platform string) error {
	token = strings.TrimSpace(token)
	platform = strings.TrimSpace(strings.ToLower(platform))
	if token == "" {
		return errors.New("token is required")
	}

	now := time.Now()

	var existing models.DeviceToken
	err := s.db.Where("token = ?", token).First(&existing).Error
	if err == nil {
		updates := map[string]any{"user_id": userID, "last_seen_at": now}
		if platform != "" {
			updates["platform"] = platform
		}
		return s.db.Model(&models.DeviceToken{}).Where("id = ?", existing.ID).Updates(updates).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	dt := models.DeviceToken{UserID: userID, Token: token, Platform: platform, LastSeenAt: now}
	return s.db.Create(&dt).Error
}


// service.go

type UserProfileResponse struct {
	User    UserProfile      `json:"user"`
	Reviews []map[string]any `json:"reviews"` // was []models.Review
}

func (s *Service) GetUserProfile(id string) (*UserProfileResponse, error) {
	var user models.User
	if err := s.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	user.Password = ""

	var reviews []models.Review
	if err := s.db.Preload("Reviewer").
		Where("target_id = ?", id).
		Order("created_at DESC").
		Find(&reviews).Error; err != nil {
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

	avgRating := 0.0
	if len(reviewsRes) > 0 {
		avgRating = totalRating / float64(len(reviewsRes))
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
			Rating:        avgRating,
		},
		Reviews: reviewsRes, // ✅ was `reviews` (raw models, losing reviewer info + re-triggering rating bug)
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

func (s *Service) AddPortfolio(User *models.User, data models.Portfolio, files []*multipart.FileHeader) (*models.Portfolio, error) {
	data.UserID = User.ID

	fmt.Println("files")
	fmt.Println(files)
	tx := s.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&data).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	for _, f := range files {
		file, _ := f.Open()

		defer file.Close()

		fileType := f.Header.Get("Content-Type")
		url, objectKey, err := s.s3Client.UploadFile(file, f.Filename, fileType, "", "")

		if err != nil {
			tx.Rollback()
			return nil, err
		}

		media := models.File{
			URL:        url,
			FileName:   f.Filename,
			FileSize:   f.Size,
			EntityID:   data.ID,
			ObjectKey:  objectKey,
			FileType:   fileType,
			EntityType: "portfolios",
		}

		if err := tx.Create(&media).Error; err != nil {
			return nil, err
		}

	}

	if err := tx.Commit().Error; err != nil {
		logger.Log.Error("Transaction commit failed", zap.Error(err))
		return nil, err
	}

	s.db.Preload("Media", "entity_type = ?", "portfolios").First(&data, "id = ?", data.ID)

	return &data, nil
}

func (s *Service) DeletePortfolio(userID string, id string) error {
	result := s.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Portfolio{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("portfolio not found or unauthorized")
	}

	return nil
}

func (s *Service) UpdatePortfolio(userID string, portfolioID string, data models.Portfolio, keepMediaIDs []string, newFiles []*multipart.FileHeader) (*models.Portfolio, error) {
	tx := s.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var portfolio models.Portfolio
	if err := tx.Where("id = ? AND user_id = ?", portfolioID, userID).First(&portfolio).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("portfolio not found or unauthorized")
	}

	tx.Model(&portfolio).Updates(data)
	var attachmentsToDelete []models.File
	tx.Where("entity_id = ? AND entity_type = ? AND id NOT IN ?", portfolioID, "portfolios", keepMediaIDs).Find(&attachmentsToDelete)

	for _, asset := range attachmentsToDelete {
		tx.Delete(&asset)
	}

	for _, f := range newFiles {
		src, _ := f.Open()
		defer src.Close()

		url, objectKey, err := s.s3Client.UploadFile(src, f.Filename, f.Header.Get("Content-Type"), "", "")
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		tx.Create(&models.File{
			URL:        url,
			FileName:   f.Filename,
			FileSize:   f.Size,
			ObjectKey:  objectKey,
			FileType:   f.Header.Get("Content-Type"),
			EntityID:   portfolio.ID,
			EntityType: "portfolios",
		})
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.db.Preload("Media").First(&portfolio, "id = ?", portfolioID)
	return &portfolio, nil
}

func (s *Service) AddCertification(userID string, cert models.Certification, file *multipart.FileHeader) (*models.Certification, error) {
	tx := s.db.Begin()
	cert.UserID = userID

	if file != nil {
		src, _ := file.Open()
		defer src.Close()
		url, _, err := s.s3Client.UploadFile(src, file.Filename, file.Header.Get("Content-Type"), "", "")
		fmt.Println("certification file upload url:", url)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		cert.ImageURL = url
	}

	if err := tx.Create(&cert).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	tx.Commit()
	return &cert, nil
}

func (s *Service) UpdateCertification(userID, certID string, data models.Certification, file *multipart.FileHeader) (*models.Certification, error) {
	var cert models.Certification
	if err := s.db.Where("id = ? AND user_id = ?", certID, userID).First(&cert).Error; err != nil {
		return nil, errors.New("certification not found")
	}

	if file != nil {
		src, _ := file.Open()
		defer src.Close()
		url, _, err := s.s3Client.UploadFile(src, file.Filename, file.Header.Get("Content-Type"), "", "")
		if err == nil {
			cert.ImageURL = url
		}
	}

	s.db.Model(&cert).Updates(data)
	return &cert, nil
}

func (s *Service) UpdateUser(user *models.User) {

}
func (s *Service) RedeemCode(tx *gorm.DB, code string, UserID string) error {
	var refCode models.ReferralCode

	if err := tx.Where("code = ? AND is_active = ?", strings.ToUpper(strings.TrimSpace(code)), true).First(&refCode).Error; err != nil {
		return errors.New("invalid referral code")
	}

	if refCode.OwnerID == UserID {
		return errors.New("cannot refer yourself")
	}

	// if err := tx.Model(&models.User{}).Where("id = ?", UserID).
	// 	Update("referred_by_id", refCode.OwnerID).Error; err != nil {
	// 	return err
	// }

	// usage := models.ReferralUsage{
	// 	ReferralCodeID: refCode.ID,
	// 	ReferrerID:     refCode.OwnerID,
	// 	RefereeID:      UserID,
	// 	RewardAmount:   refCode.,
	// 	Status:         "pending",
	// }
	// if err := tx.Create(&usage).Error; err != nil {
	// 	return err
	// }

	return tx.Model(&refCode).UpdateColumn("current_uses", gorm.Expr("current_uses + ?", 1)).Error
}
