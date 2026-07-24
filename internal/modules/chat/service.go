package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"tasksy/config"
	"tasksy/models"
	"tasksy/pkg/logger"
	"tasksy/utils"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var ErrNotParticipant = errors.New("not a participant")

type Service interface {
	InitiateChat(userA, userB, title string) (*models.ChatConversation, error)
	SendMessage(senderID, convID, content, msgType string, files []*multipart.FileHeader) (*models.ChatMessage, error)
	GetInbox(userID string) ([]models.ChatConversation, error)
	GetChatHistory(conversationID string, page, limit int) ([]models.ChatMessage, error)
	GetUnreadMessages(conversationID, userID string, page, limit int) ([]models.ChatMessage, error)
	MarkConversationRead(conversationID, userID string) (int64, error)
	RegisterClient(client *Client)
	UnregisterClient(client *Client)
}

type Notifier interface {
	NotifyNewMessage(ctx context.Context, deviceToken, senderName, conversationID, jobPostID string) error
}

type chatService struct {
	repo     Repository
	notifier Notifier
	db       *gorm.DB
	// In-Memory Connection Store
	// UserID -> *Client
	clients   map[string]*Client
	mu        sync.RWMutex
	s3Client  *s3.Client
	appConfig *config.Config
}

func NewService(repo Repository, appConfig *config.Config, notifier Notifier, db *gorm.DB) Service {

	creds := credentials.NewStaticCredentialsProvider(
		appConfig.AWS.AccessKeyID,
		appConfig.AWS.SecretAccessKey,
		"",
	)
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(appConfig.AWS.Region),
		awsconfig.WithCredentialsProvider(creds),
	)

	var s3Client *s3.Client
	if err == nil {
		s3Client = s3.NewFromConfig(cfg)
	} else {
		logger.Log.Error("failed to load AWS config for chat service", zap.Error(err))
	}
	return &chatService{
		db:        db,
		repo:      repo,
		notifier:  notifier,
		clients:   make(map[string]*Client),
		s3Client:  s3Client,
		appConfig: appConfig,
	}
}

func (s *chatService) uploadToS3(file io.Reader, filename, mimeType string) (string, error) {
	if s.s3Client == nil {
		return "", fmt.Errorf("S3 client not initialized")
	}

	region := s.appConfig.AWS.Region
	if region == "" {
		region = "eu-north-1"
	}

	key := fmt.Sprintf("chat/%d_%s", time.Now().UnixNano(), filename)
	bucketName := s.appConfig.AWS.BucketName

	_, err := s.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(mimeType),
	})

	if err != nil {
		return "", err
	}

	if s.appConfig.AWS.BucketURL != "" {
		baseURL := strings.TrimRight(s.appConfig.AWS.BucketURL, "/")
		return fmt.Sprintf("%s/%s", baseURL, key), nil
	}

	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, key), nil
}

func (s *chatService) InitiateChat(userA, userB, jobID string) (*models.ChatConversation, error) {
	existing, err := s.repo.FindPrivateChat(userA, userB, jobID)
	if err == nil {
		return existing, nil
	}
	return s.repo.CreateConversation([]string{userA, userB}, jobID)
}

func (s *chatService) SendMessage(senderID, convID, content, msgType string, files []*multipart.FileHeader) (*models.ChatMessage, error) {
	sender, err := s.repo.GetUserByID(senderID)

	if err != nil {
		return nil, err
	}

	var attachments []models.ChatAttachment

	if len(files) > 0 {
		msgType = "attachment"

		for _, fileHeader := range files {
			src, err := fileHeader.Open()
			if err != nil {
				return nil, err
			}

			ext := filepath.Ext(fileHeader.Filename)
			tempFile, err := os.CreateTemp("", "upload-*"+ext)
			if err != nil {
				src.Close()
				return nil, err
			}
			if _, err := io.Copy(tempFile, src); err != nil {
				src.Close()
				tempFile.Close()
				return nil, err
			}

			src.Close()
			tempFilePath := tempFile.Name()
			tempFile.Close()

			defer os.Remove(tempFilePath)

			mimeType := fileHeader.Header.Get("Content-Type")
			fileType := utils.GetFileType(mimeType)
			fileSize := fileHeader.Size
			duration := 0

			if fileType == "video" || fileType == "audio" {
				duration = utils.GetMediaDuration(tempFilePath)
			}

			// D. Upload to S3 (Using the Temp File)
			// We open the temp file to upload it
			uploadFile, err := os.Open(tempFilePath)
			if err != nil {
				return nil, err
			}

			fileURL, err := s.uploadToS3(uploadFile, fileHeader.Filename, mimeType)
			uploadFile.Close() // Close after upload

			if err != nil {
				return nil, fmt.Errorf("failed to upload file %s: %v", fileHeader.Filename, err)
			}

			attachments = append(attachments, models.ChatAttachment{
				URL:      fileURL,
				FileType: fileType,
				FileName: fileHeader.Filename,
				FileSize: fileSize,
				Duration: duration,
			})
		}
	}

	msg := &models.ChatMessage{
		ConversationID: convID,
		SenderID:       senderID,
		Content:        content,
		Type:           msgType,
		IsRead:         false,
		Sender:         *sender,
		Attachments:    attachments,
	}

	if err := s.repo.SaveMessage(msg); err != nil {
		return nil, err
	}

	if err := s.repo.IncrementUnreadCount(convID, senderID); err != nil {
		return nil, err
	}

	var JobPostID string
	s.db.Raw("select job_post_id from chat_conversation where id = ? ", convID).Scan(&JobPostID)

	go func() {
		participants, err := s.repo.GetParticipantIDs(convID)
		if err != nil {
			return
		}

		senderName := strings.TrimSpace(strings.TrimSpace(sender.FirstName) + " " + strings.TrimSpace(sender.LastName))
		if senderName == "" {
			senderName = "Someone"
		}

		s.mu.RLock()
		for _, uid := range participants {
			if client, isOnline := s.clients[uid]; isOnline {
				select {
				case client.Send <- msg:
				default:
				}
			}
		}
		s.mu.RUnlock()

		if s.notifier == nil {
			return
		}
		for _, uid := range participants {
			if uid == senderID {
				continue
			}
			tokens, err := s.repo.GetDeviceTokensByUserID(uid)
			if err != nil || len(tokens) == 0 {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			logger.Log.Info("sending FCM push",
				zap.String("recipient_user_id", uid),
				zap.Int("token_count", len(tokens)),
				zap.String("conversation_id", convID),
				zap.String("job_post_id", JobPostID),
			)
			for _, t := range tokens {
				if err := s.notifier.NotifyNewMessage(ctx, t, senderName, convID, JobPostID); err != nil {
					logger.Log.Error("FCM push failed",
						zap.String("recipient_user_id", uid),
						zap.String("token", t),
						zap.Error(err),
					)
				}
			}
			cancel()
		}
	}()

	return msg, nil
}

func (s *chatService) GetUnreadMessages(conversationID, userID string, page, limit int) ([]models.ChatMessage, error) {
	ok, err := s.repo.IsUserParticipant(conversationID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotParticipant
	}
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 50
	}
	offset := (page - 1) * limit
	return s.repo.GetUnreadMessages(conversationID, userID, limit, offset)
}

func (s *chatService) MarkConversationRead(conversationID, userID string) (int64, error) {
	ok, err := s.repo.IsUserParticipant(conversationID, userID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrNotParticipant
	}
	return s.repo.MarkConversationRead(conversationID, userID)
}
func (s *chatService) GetInbox(userID string) ([]models.ChatConversation, error) {
	return s.repo.GetUserConversations(userID)
}

func (s *chatService) RegisterClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[c.UserID] = c
}

func (s *chatService) UnregisterClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clients[c.UserID]; ok {
		delete(s.clients, c.UserID)
		close(c.Send)
	}
}

func (s *chatService) GetChatHistory(conversationID string, page, limit int) ([]models.ChatMessage, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit
	return s.repo.GetHistory(conversationID, limit, offset)
}
