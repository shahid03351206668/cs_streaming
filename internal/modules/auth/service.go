package auth

import (
	"errors"
	"net/http"
	"tasksy/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	db *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{db: db}
}

func (s *AuthService) AuthenticateUser(email, password string) error {
	if email == "" {
		return errors.New("email is required")
	}
	
	if password == "" {
		return errors.New("password is required")
	}

	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("invalid credentials")
		}
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return errors.New("invalid credentials")
	}

	return nil
}

func (s *AuthService) LoginHandler(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": err})
		return
	}

	if err := s.AuthenticateUser(body.Email, body.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "error": "invalid credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}
