package admin

import (
	"mime/multipart"
	"tasksy/config"
	"tasksy/models"

	"gorm.io/gorm"
)

type Service struct {
	db        *gorm.DB
	appCOnfig *config.Config
}

func makePassword(value string) string {
	password := value
	return password
}

func (s *Service) CreateUser(payload *UserPayload, image *multipart.File) (*models.User, error) {
	tx := s.db.Begin()

	user := models.User{
		Email:           payload.Email,
		FirstName:       payload.FirstName,
		LastName:        payload.LastName,
		PhoneNumber:     payload.PhoneNumber,
		Disabled:        payload.Disabled,
		PhoneVerified:   payload.PhoneVerified,
		EmailVerified:   payload.EmailVerified,
		IdentityVerfied: payload.IdentityVerified,
		ProfilePhoto:    "",
		Password:        makePassword(payload.Password),
	}

	defer func() {
		if recover() != nil {
			tx.Rollback()
		}
	}()

	if err := s.db.Create(&user).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	return &user, nil
}
