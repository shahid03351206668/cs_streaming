package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"html"
	"mime"
	"net"
	"net/smtp"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"tasksy/models"
	"tasksy/pkg/logger"

	"go.uber.org/zap"
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

	from := fmt.Sprintf("%s <%s>", headerSafe(acc.FromName), acc.Email)
	auth := &loginAuth{username: acc.Email, password: acc.Password}

	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n",
		from, headerSafe(to), mime.QEncoding.Encode("UTF-8", headerSafe(subject)),
		time.Now().Format(time.RFC1123Z), messageID(acc.Email),
	)
	msg := []byte(headers + htmlBody)

	// Port 465 is implicit TLS (encrypted from the first byte) — smtp.SendMail
	// only supports plaintext-then-STARTTLS, so it must be dialed separately.
	if acc.Port == 465 {
		return sendMailImplicitTLS(acc.Host, acc.Port, auth, acc.Email, to, msg)
	}

	addr := fmt.Sprintf("%s:%d", acc.Host, acc.Port)
	return smtp.SendMail(addr, auth, acc.Email, []string{to}, msg)
}

// headerSafe keeps user-supplied text on one line, so it can't add headers or
// start the message body.
func headerSafe(v string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
}

func messageID(from string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	domain := "tasksy.co.uk"
	if i := strings.LastIndex(from, "@"); i >= 0 {
		domain = from[i+1:]
	}
	return "<" + hex.EncodeToString(b) + "@" + domain + ">"
}

func sendMailImplicitTLS(host string, port int, auth smtp.Auth, from, to string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", host, port)

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close body: %w", err)
	}

	return c.Quit()
}

var placeholderRE = regexp.MustCompile(`\{\{\s*\.?\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Names match however they are written: job_title, JobTitle and .JobTitle are the same.
func normalizeKey(k string) string { return strings.ToLower(strings.ReplaceAll(k, "_", "")) }

// fillTemplate replaces {{job_title}} and {{.JobTitle}} style placeholders. One
// that has no value is dropped, so a customer never sees raw template syntax.
func fillTemplate(template, name string, vars map[string]string, clean func(string) string) string {
	values := make(map[string]string, len(vars))
	for k, v := range vars {
		values[normalizeKey(k)] = v
	}
	return placeholderRE.ReplaceAllStringFunc(template, func(m string) string {
		key := placeholderRE.FindStringSubmatch(m)[1]
		v, ok := values[normalizeKey(key)]
		if !ok {
			if logger.Log != nil {
				logger.Log.Warn("email template uses a placeholder the app doesn't fill in",
					zap.String("template", name), zap.String("placeholder", key))
			}
			return ""
		}
		return clean(v)
	})
}

// SendTemplatedEmail fetches a template by name, fills in its placeholders, and sends from noreply@tasksy.co.uk.
func (s *Service) SendTemplatedEmail(templateName, to string, vars map[string]string) error {
	var tmpl models.EmailTemplate
	if err := s.db.Where("name = ?", templateName).First(&tmpl).Error; err != nil {
		return fmt.Errorf("email template %q not found: %w", templateName, err)
	}

	subject := fillTemplate(tmpl.Subject, templateName, vars, headerSafe)
	// Values such as job titles come from users and the body is HTML.
	body := fillTemplate(tmpl.Body, templateName, vars, html.EscapeString)

	return s.SendMail(fromEmail, to, subject, body)
}
