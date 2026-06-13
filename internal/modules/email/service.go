package email

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"tasksy/models"
)

const fromEmail = "noreply@tasksy.co.uk"

const (
	emailAccountCacheTTL    = 10 * time.Minute
	emailAccountCachePrefix = "email_account:"
)

type Service struct {
	db    *gorm.DB
	redis *redis.Client
}

func NewService(db *gorm.DB, redis *redis.Client) *Service {
	return &Service{db: db, redis: redis}
}

func (s *Service) GetEmailAccount(email string) (*models.EmailAccount, error) {
	cacheKey := emailAccountCachePrefix + "default"
	if email != "" {
		cacheKey = emailAccountCachePrefix + email
	}

	if cached, err := s.redis.Get(context.Background(), cacheKey).Bytes(); err == nil {
		var acc models.EmailAccount
		if err := json.Unmarshal(cached, &acc); err == nil {
			return &acc, nil
		}
	}

	var acc models.EmailAccount
	var dbErr error

	if email != "" {
		dbErr = s.db.First(&acc, "email = ? AND is_active = true", email).Error
	} else {
		dbErr = s.db.Where("is_default = true AND is_active = true").First(&acc).Error
	}

	if dbErr != nil {
		return nil, fmt.Errorf("email account not found: %w", dbErr)
	}

	accToCache := acc
	// accToCache.Password = ""
	if data, err := json.Marshal(accToCache); err == nil {
		s.redis.Set(context.Background(), cacheKey, data, emailAccountCacheTTL)
	}

	return &acc, nil
}

func (s *Service) InvalidateEmailAccountCache(email string) {
	cacheKey := emailAccountCachePrefix + "default"

	if email != "" {
		cacheKey = emailAccountCachePrefix + email
	}

	s.redis.Del(context.Background(), cacheKey)
}

// loginAuth implements AUTH LOGIN for SMTP servers (e.g. Microsoft 365)
// that do not accept the standard AUTH PLAIN method.
type loginAuth struct{ username, password string }

func (a *loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected SMTP challenge: %s", fromServer)
	}
}

func (s *Service) SendMail(email, to, subject, htmlBody string) error {
	acc, err := s.GetEmailAccount(email)
	if err != nil {
		return err
	}

	from := fmt.Sprintf("%s <%s>", acc.FromName, acc.Email)
	addr := fmt.Sprintf("%s:%d", acc.Host, acc.Port)
	auth := &loginAuth{username: acc.Email, password: acc.Password}

	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n",
		from, to, subject,
	)

	return smtp.SendMail(addr, auth, acc.Email, []string{to}, []byte(headers+htmlBody))
}

// SendTemplatedEmail fetches a template by name, substitutes {{key}} vars, and sends from noreply@tasksy.co.uk.
func (s *Service) SendTemplatedEmail(templateName, to string, vars map[string]string) error {
	var tmpl models.EmailTemplate
	if err := s.db.Where("name = ?", templateName).First(&tmpl).Error; err != nil {
		return fmt.Errorf("email template %q not found: %w", templateName, err)
	}

	subject := tmpl.Subject
	body := tmpl.Body
	for k, v := range vars {
		placeholder := "{{" + k + "}}"
		subject = strings.ReplaceAll(subject, placeholder, v)
		body = strings.ReplaceAll(body, placeholder, v)
	}

	return s.SendMail(fromEmail, to, subject, body)
}
