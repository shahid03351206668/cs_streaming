package user

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"tasksy/config"
	"tasksy/models"
	aws_services "tasksy/pkg"
	"tasksy/pkg/logger"
	"time"

	"github.com/stripe/stripe-go/v84"
	"github.com/stripe/stripe-go/v84/account"
	"github.com/stripe/stripe-go/v84/bankaccount"
	"github.com/stripe/stripe-go/v84/token"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrPhoneAlreadyTaken = errors.New("phone number already taken")
	ErrEmailAlreadyTaken = errors.New("email already taken")
)

type UpdateProfileData struct {
	FirstName   string
	LastName    string
	PhoneNumber string
	Email       string
	DateOfBirth *time.Time
	Verified    string
}

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

func (s *Service) DeleteDeviceToken(userID, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is required")
	}
	result := s.db.Where("token = ? AND user_id = ?", token, userID).Delete(&models.DeviceToken{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("token not found")
	}
	return nil
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

func (s *Service) CreateUser(data UserData, file *multipart.FileHeader, clientIP string) (*models.User, error) {

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

	go func() {
		stripe.Key = s.appConfig.Stripe.SecretKey
		now := time.Now().Unix()
		ip := clientIP
		if ip == "" {
			ip = "127.0.0.1"
		}
		acc, err := account.New(&stripe.AccountParams{
			Type:         stripe.String(string(stripe.AccountTypeCustom)),
			Email:        stripe.String(user.Email),
			Country:      stripe.String("GB"),
			BusinessType: stripe.String("individual"),
			BusinessProfile: &stripe.AccountBusinessProfileParams{
				URL: stripe.String("https://tasksy.co.uk"),
				MCC: stripe.String("7372"),
			},
			Individual: &stripe.PersonParams{
				FirstName: stripe.String(user.FirstName),
				LastName:  stripe.String(user.LastName),
				Email:     stripe.String(user.Email),
			},
			TOSAcceptance: &stripe.AccountTOSAcceptanceParams{
				Date: stripe.Int64(now),
				IP:   stripe.String(ip),
			},
			Capabilities: &stripe.AccountCapabilitiesParams{
				CardPayments: &stripe.AccountCapabilitiesCardPaymentsParams{
					Requested: stripe.Bool(true),
				},
				Transfers: &stripe.AccountCapabilitiesTransfersParams{
					Requested: stripe.Bool(true),
				},
			},
		})
		if err != nil {
			logger.Log.Error("failed to create stripe connect account", zap.String("user_id", user.ID), zap.Error(err))
			return
		}
		if err := s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("stripe_connect_account_id", acc.ID).Error; err != nil {
			logger.Log.Error("failed to save stripe connect account id", zap.String("user_id", user.ID), zap.Error(err))
		}
	}()

	return &user, nil
}

func (s *Service) AddPortfolio(User *models.User, data models.Portfolio, files []*multipart.FileHeader) (*models.Portfolio, error) {
	data.UserID = User.ID

	logger.Log.Info("adding portfolio files", zap.Int("file_count", len(files)))
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
		logger.Log.Info("certification file uploaded", zap.String("url", url))
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

func (s *Service) SyncUserToStripe(user *models.User) error {

	stripe.Key = s.appConfig.Stripe.SecretKey

	now := time.Now().Unix()
	acc, err := account.New(&stripe.AccountParams{
		Type:         stripe.String(string(stripe.AccountTypeCustom)),
		Email:        stripe.String(user.Email),
		Country:      stripe.String("GB"),
		BusinessType: stripe.String("individual"),
		BusinessProfile: &stripe.AccountBusinessProfileParams{
			URL: stripe.String("https://tasksy.co.uk"),
			MCC: stripe.String("7372"),
		},
		Individual: &stripe.PersonParams{
			FirstName: stripe.String(user.FirstName),
			LastName:  stripe.String(user.LastName),
			Email:     stripe.String(user.Email),
		},
		TOSAcceptance: &stripe.AccountTOSAcceptanceParams{
			Date: stripe.Int64(now),
			IP:   stripe.String("127.0.0.1"),
		},
		Capabilities: &stripe.AccountCapabilitiesParams{
			CardPayments: &stripe.AccountCapabilitiesCardPaymentsParams{
				Requested: stripe.Bool(true),
			},
			Transfers: &stripe.AccountCapabilitiesTransfersParams{
				Requested: stripe.Bool(true),
			},
		},
	})
	if err != nil {
		return err
	}
	if err := s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("stripe_connect_account_id", acc.ID).Error; err != nil {
		return err
	}

	return nil
}

func (s *Service) AddAddress(user *models.User, addr models.UserAddress) (*models.UserAddress, error) {
	addr.UserID = user.ID

	tx := s.db.Begin()
	if addr.IsDefault {
		if err := tx.Model(&models.UserAddress{}).Where("user_id = ?", user.ID).Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Create(&addr).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.syncAddressToStripe(user, &addr)
	return &addr, nil
}

func (s *Service) UpdateAddress(user *models.User, addressID string, addr models.UserAddress) (*models.UserAddress, error) {
	var existing models.UserAddress
	if err := s.db.First(&existing, "id = ? AND user_id = ?", addressID, user.ID).Error; err != nil {
		return nil, errors.New("address not found")
	}

	tx := s.db.Begin()
	if addr.IsDefault {
		if err := tx.Model(&models.UserAddress{}).Where("user_id = ? AND id != ?", user.ID, addressID).Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Model(&existing).Updates(map[string]any{
		"line1":       addr.Line1,
		"line2":       addr.Line2,
		"city":        addr.City,
		"state":       addr.State,
		"postal_code": addr.PostalCode,
		"country":     addr.Country,
		"is_default":  addr.IsDefault,
	}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	s.syncAddressToStripe(user, &existing)
	return &existing, nil
}

func (s *Service) DeleteAddress(user *models.User, addressID string) error {
	result := s.db.Where("id = ? AND user_id = ?", addressID, user.ID).Delete(&models.UserAddress{})
	if result.RowsAffected == 0 {
		return errors.New("address not found")
	}
	return result.Error
}

func (s *Service) GetAddresses(user *models.User) ([]models.UserAddress, error) {
	var addresses []models.UserAddress
	err := s.db.Where("user_id = ?", user.ID).Order("is_default DESC, created_at DESC").Find(&addresses).Error
	return addresses, err
}

func (s *Service) syncAddressToStripe(user *models.User, addr *models.UserAddress) {
	if user.StripeConnectAccountID == "" {
		return
	}
	go func() {
		stripe.Key = s.appConfig.Stripe.SecretKey
		_, err := account.Update(user.StripeConnectAccountID, &stripe.AccountParams{
			Individual: &stripe.PersonParams{
				Address: &stripe.AddressParams{
					Line1:      stripe.String(addr.Line1),
					Line2:      stripe.String(addr.Line2),
					City:       stripe.String(addr.City),
					State:      stripe.String(addr.State),
					PostalCode: stripe.String(addr.PostalCode),
					Country:    stripe.String(addr.Country),
				},
			},
		})
		if err != nil {
			logger.Log.Error("failed to sync address to stripe", zap.String("user_id", user.ID), zap.Error(err))
		}
	}()
}

func (s *Service) AddBankAccount(user *models.User, accountHolderName, sortCode, accountNumber string) error {
	if user.StripeConnectAccountID == "" {
		return errors.New("stripe account not provisioned for this user")
	}

	stripe.Key = s.appConfig.Stripe.SecretKey

	tok, err := token.New(&stripe.TokenParams{
		BankAccount: &stripe.BankAccountParams{
			Country:           stripe.String("GB"),
			Currency:          stripe.String("gbp"),
			AccountHolderName: stripe.String(accountHolderName),
			AccountHolderType: stripe.String("individual"),
			RoutingNumber:     stripe.String(sortCode),
			AccountNumber:     stripe.String(accountNumber),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tokenize bank account: %w", err)
	}

	_, err = bankaccount.New(&stripe.BankAccountParams{
		Account: stripe.String(user.StripeConnectAccountID),
		Token:   stripe.String(tok.ID),
	})
	if err != nil {
		return fmt.Errorf("failed to attach bank account: %w", err)
	}

	return s.db.Model(&models.User{}).Where("id = ?", user.ID).Update("stripe_connect_onboarded", true).Error
}

func (s *Service) LookupUserByContact(email, phone string) (*models.User, error) {
	query := s.db
	if email != "" {
		query = query.Where("email = ?", email)
	}
	if phone != "" {
		query = query.Where("phone_number = ?", phone)
	}
	var user models.User
	if err := query.First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Service) GetProfileReviews(user *models.User) ([]models.Review, error) {
	var reviews []models.Review
	err := s.db.Where("target_id = ?", user.ID).
		Preload("Reviewer").
		Preload("Contract").
		Order("created_at DESC").
		Find(&reviews).Error
	return reviews, err
}

func (s *Service) ChangePassword(user *models.User, currentPassword, newPassword string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.Model(user).Update("password", string(hashed)).Error
}

func (s *Service) VerifyCredential(user *models.User, email, phone string) error {
	if email != "" {
		if user.Email != email {
			return errors.New("email does not match user profile")
		}
		var existing models.User
		if s.db.Where("email = ?", email).Where("id != ?", user.ID).First(&existing).Error == nil {
			return errors.New("email already taken by another user")
		}
		if err := s.db.Model(user).Update("email_verified", true).Error; err != nil {
			return err
		}
	}
	if phone != "" {
		if user.PhoneNumber != phone {
			return errors.New("phone number does not match user profile")
		}
		var existing models.User
		if s.db.Where("phone_number = ?", phone).Where("id != ?", user.ID).First(&existing).Error == nil {
			return errors.New("phone number already taken by another user")
		}
		if err := s.db.Model(user).Update("phone_verified", true).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) UpdateProfile(user *models.User, data UpdateProfileData, photoFile *multipart.FileHeader) (*models.User, error) {
	updates := make(map[string]interface{})

	if data.FirstName != "" {
		updates["first_name"] = data.FirstName
	}
	if data.LastName != "" {
		updates["last_name"] = data.LastName
	}
	if data.DateOfBirth != nil {
		updates["dob"] = data.DateOfBirth
	}
	if data.Verified == "true" {
		updates["identity_verified"] = true
	}

	if data.PhoneNumber != "" {
		var existing models.User
		result := s.db.Where("phone_number = ?", data.PhoneNumber).Where("id != ?", user.ID).First(&existing)
		if result.Error == nil {
			return nil, ErrPhoneAlreadyTaken
		} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, result.Error
		}
		updates["phone_number"] = data.PhoneNumber
		updates["phone_verified"] = false
	}

	if data.Email != "" {
		var existing models.User
		result := s.db.Where("email = ?", data.Email).Where("id != ?", user.ID).First(&existing)
		if result.Error == nil {
			return nil, ErrEmailAlreadyTaken
		} else if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, result.Error
		}
		updates["email"] = data.Email
		updates["email_verified"] = false
	}

	if photoFile != nil {
		allowedTypes := map[string]bool{
			"image/jpeg": true, "image/jpg": true,
			"image/png": true, "image/gif": true, "image/webp": true,
		}
		contentType := photoFile.Header.Get("Content-Type")
		if !allowedTypes[contentType] {
			return nil, errors.New("invalid file type, only images are allowed")
		}
		if photoFile.Size > 5*1024*1024 {
			return nil, errors.New("file size too large, maximum 5MB allowed")
		}

		profilePhotoPath := filepath.Join("media/", "profiles")
		if err := os.MkdirAll(profilePhotoPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create profile photo directory: %w", err)
		}

		if user.ProfilePhoto != "" {
			if _, err := os.Stat(user.ProfilePhoto); err == nil {
				os.Remove(user.ProfilePhoto)
			}
		}

		ext := filepath.Ext(photoFile.Filename)
		fileName := fmt.Sprintf("profile_%s_%d%s", user.ID, time.Now().UnixNano(), ext)
		filePath := filepath.Join(profilePhotoPath, fileName)

		src, err := photoFile.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open uploaded file: %w", err)
		}
		defer src.Close()

		dst, err := os.Create(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create destination file: %w", err)
		}
		defer dst.Close()

		if _, err = io.Copy(dst, src); err != nil {
			os.Remove(filePath)
			return nil, fmt.Errorf("failed to save profile photo: %w", err)
		}

		updates["profile_photo"] = filePath
	}

	if len(updates) == 0 {
		return nil, errors.New("no fields to update")
	}

	if err := s.db.Model(user).Updates(updates).Error; err != nil {
		if photoPath, ok := updates["profile_photo"].(string); ok {
			os.Remove(photoPath)
		}
		return nil, err
	}

	var updated models.User
	if err := s.db.Where("id = ?", user.ID).First(&updated).Error; err != nil {
		return nil, err
	}

	s.syncProfileToStripe(updated)
	return &updated, nil
}

func (s *Service) syncProfileToStripe(user models.User) {
	if user.StripeConnectAccountID == "" {
		return
	}
	go func() {
		stripe.Key = s.appConfig.Stripe.SecretKey
		params := &stripe.AccountParams{
			Individual: &stripe.PersonParams{
				FirstName: stripe.String(user.FirstName),
				LastName:  stripe.String(user.LastName),
				Email:     stripe.String(user.Email),
			},
		}
		if user.DateOfBirth != nil {
			params.Individual.DOB = &stripe.PersonDOBParams{
				Day:   stripe.Int64(int64(user.DateOfBirth.Day())),
				Month: stripe.Int64(int64(user.DateOfBirth.Month())),
				Year:  stripe.Int64(int64(user.DateOfBirth.Year())),
			}
		}
		if _, err := account.Update(user.StripeConnectAccountID, params); err != nil {
			logger.Log.Error("failed to sync profile to stripe", zap.String("user_id", user.ID), zap.Error(err))
		}
	}()
}
