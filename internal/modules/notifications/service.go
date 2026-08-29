package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"tasksy/models"
	"tasksy/pkg/fcm"
	"tasksy/pkg/logger"
	"time"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	db  *gorm.DB
	fcm *fcm.FCMClient
}

func NewService(db *gorm.DB, fcmClient *fcm.FCMClient) *Service {
	return &Service{db: db, fcm: fcmClient}
}

type UpsertPreferencesInput struct {
	EnableProposalSent     *bool
	EnableProposalReceived *bool
	EnableProposalDecision *bool
	EnableNewJobs          *bool
	JobRadiusKM            *float64
	City                   *string
	Latitude               *float64
	Longitude              *float64
	CategoryIDs            *[]string
}


func (s *Service) defaultPreference(userID string) models.NotificationPreference {
	return models.NotificationPreference{
		UserID:                 userID,
		EnableProposalSent:     true,
		EnableProposalReceived: true,
		EnableProposalDecision: true,
		EnableNewJobs:          true,
		JobRadiusKM:            50,
	}
}

func (s *Service) GetNotificationPreferences(userID string) (*models.NotificationPreference, error) {
	if s.db == nil {
		return nil, nil
	}

	var pref models.NotificationPreference
	err := s.db.Preload("Categories").Preload("Categories.Category").Where("user_id = ?", userID).First(&pref).Error
	if err == nil {
		return &pref, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	pref = s.defaultPreference(userID)
	if err := s.db.Create(&pref).Error; err != nil {
		return nil, err
	}
	if err := s.db.Preload("Categories").Preload("Categories.Category").First(&pref, "id = ?", pref.ID).Error; err != nil {
		return nil, err
	}
	return &pref, nil
}

func (s *Service) UpsertNotificationPreferences(userID string, input UpsertPreferencesInput) (*models.NotificationPreference, error) {
	if s.db == nil {
		return nil, nil
	}

	pref, err := s.GetNotificationPreferences(userID)
	if err != nil {
		return nil, err
	}

	if input.Latitude != nil && input.Longitude == nil {
		return nil, errors.New("longitude is required when latitude is provided")
	}
	if input.Longitude != nil && input.Latitude == nil {
		return nil, errors.New("latitude is required when longitude is provided")
	}
	if input.JobRadiusKM != nil && *input.JobRadiusKM < 0 {
		return nil, errors.New("job_radius_km must be non-negative")
	}

	tx := s.db.Begin()
	updates := map[string]any{}
	if input.EnableProposalSent != nil {
		updates["enable_proposal_sent"] = *input.EnableProposalSent
	}
	if input.EnableProposalReceived != nil {
		updates["enable_proposal_received"] = *input.EnableProposalReceived
	}
	if input.EnableProposalDecision != nil {
		updates["enable_proposal_decision"] = *input.EnableProposalDecision
	}
	if input.EnableNewJobs != nil {
		updates["enable_new_jobs"] = *input.EnableNewJobs
	}
	if input.JobRadiusKM != nil {
		updates["job_radius_km"] = *input.JobRadiusKM
	}
	if input.City != nil {
		updates["city"] = strings.TrimSpace(*input.City)
	}
	if input.Latitude != nil {
		updates["latitude"] = *input.Latitude
	}
	if input.Longitude != nil {
		updates["longitude"] = *input.Longitude
	}

	if len(updates) > 0 {
		if err := tx.Model(&models.NotificationPreference{}).Where("id = ?", pref.ID).Updates(updates).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if input.CategoryIDs != nil {
		if err := tx.Where("preference_id = ?", pref.ID).Delete(&models.NotificationPreferenceCategory{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		for _, cid := range *input.CategoryIDs {
			cid = strings.TrimSpace(cid)
			if cid == "" {
				continue
			}
			item := models.NotificationPreferenceCategory{PreferenceID: pref.ID, CategoryID: cid}
			if err := tx.Create(&item).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return s.GetNotificationPreferences(userID)
}

func (s *Service) getUserDeviceTokens(userID string) ([]string, error) {
	var tokens []string
	err := s.db.Model(&models.DeviceToken{}).Where("user_id = ?", userID).Pluck("token", &tokens).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []string{}, nil
	}
	return tokens, err
}

func (s *Service) isNotificationEnabled(userID, notifType string) (bool, error) {
	var pref models.NotificationPreference
	err := s.db.Where("user_id = ?", userID).First(&pref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	switch notifType {
	case "proposal_sent":
		return pref.EnableProposalSent, nil
	case "proposal_received":
		return pref.EnableProposalReceived, nil
	case "proposal_decision":
		return pref.EnableProposalDecision, nil
	case "new_job":
		return pref.EnableNewJobs, nil
	default:
		return true, nil
	}
}

// storeNotification persists a notification record so the recipient can view
// it in-app, independent of whether push delivery succeeds or the user has
// any registered device tokens.
func (s *Service) storeNotification(userID, title, body, notifType, screen string, recordJSON []byte) {
	notification := models.Notification{
		UserID: userID,
		Title:  title,
		Body:   body,
		Type:   notifType,
		Screen: screen,
		Data:   datatypes.JSON(recordJSON),
	}
	if err := s.db.Create(&notification).Error; err != nil {
		logger.Log.Error("failed to store notification", zap.String("user_id", userID), zap.String("type", notifType), zap.Error(err))
	}
}

func (s *Service) notifyUser(ctx context.Context, userID, title, body, notifType, screen string, record map[string]string) error {
	if s.db == nil {
		return nil
	}
	enabled, err := s.isNotificationEnabled(userID, notifType)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	recordJSON, _ := json.Marshal(record)
	s.storeNotification(userID, title, body, notifType, screen, recordJSON)

	if s.fcm == nil {
		return nil
	}
	tokens, err := s.getUserDeviceTokens(userID)
	if err != nil || len(tokens) == 0 {
		return err
	}

	data := map[string]string{
		"type":              notifType,
		"notification_time": time.Now().Format("2006-01-02 15:04:05"),
		"click_action":      "FLUTTER_NOTIFICATION_CLICK",
		"screen":            screen,
		"record":            string(recordJSON),
	}

	for _, token := range tokens {
		_, _ = s.fcm.SendToDevice(ctx, token, title, body, data)
	}
	return nil
}

// GetUserNotifications returns a page of the user's stored notifications, newest first.
func (s *Service) GetUserNotifications(userID string, page, limit int) ([]models.Notification, int64, error) {
	if s.db == nil {
		return []models.Notification{}, 0, nil
	}

	query := s.db.Model(&models.Notification{}).Where("user_id = ?", userID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	notifications := make([]models.Notification, 0, limit)
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&notifications).Error; err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

func (s *Service) NotifyProposalReceived(ctx context.Context, recipientUserID, jobTitle, proposalID, jobPostID string) error {
	return s.notifyUser(
		ctx,
		recipientUserID,
		"New Proposal Received",
		"You received a new proposal for: "+jobTitle,
		"proposal_received",
		"/proposal",
		map[string]string{"proposal_id": proposalID, "job_post_id": jobPostID},
	)
}

func (s *Service) NotifyProposalDecision(ctx context.Context, recipientUserID, decision, proposalID, jobPostID string) error {
	title := "Proposal Accepted"
	if decision == "rejected" {
		title = "Proposal Update"
	}
	return s.notifyUser(
		ctx,
		recipientUserID,
		title,
		"Your proposal has been "+decision,
		"proposal_decision",
		"/proposal",
		map[string]string{"decision": decision, "proposal_id": proposalID, "job_post_id": jobPostID},
	)
}

func (s *Service) NotifyProposalSent(ctx context.Context, recipientUserID, jobTitle, proposalID, jobPostID string) error {
	return s.notifyUser(
		ctx,
		recipientUserID,
		"Proposal Submitted",
		"Your proposal for '"+jobTitle+"' was sent successfully",
		"proposal_sent",
		"/proposal",
		map[string]string{"proposal_id": proposalID, "job_post_id": jobPostID},
	)
}

// NotifyJobCompleted notifies both parties that a contract was fully completed.
func (s *Service) NotifyJobCompleted(ctx context.Context, recipientUserID, jobTitle, contractID, jobPostID string) error {
	return s.notifyUser(
		ctx,
		recipientUserID,
		"Job Completed 🎉",
		"'"+jobTitle+"' has been marked as complete by both parties",
		"job_completed",
		"/contract",
		map[string]string{"contract_id": contractID, "job_post_id": jobPostID},
	)
}

// NotifyAwaitingCompletion notifies one party that the other has marked the contract complete.
func (s *Service) NotifyAwaitingCompletion(ctx context.Context, recipientUserID, jobTitle, contractID, jobPostID string) error {
	return s.notifyUser(
		ctx,
		recipientUserID,
		"Action Required",
		"The other party has marked '"+jobTitle+"' as complete. Please confirm to release payment.",
		"awaiting_completion",
		"/contract",
		map[string]string{"contract_id": contractID, "job_post_id": jobPostID},
	)
}

// NotifyNewMessage notifies a user about a new chat message: it stores exactly
// one notification record for the recipient and pushes to each of their
// registered device tokens.
func (s *Service) NotifyNewMessage(ctx context.Context, recipientUserID string, deviceTokens []string, senderName, conversationID, jobPostID string) error {
	title := "New Message"
	body := senderName + " sent you a message"

	record := map[string]string{
		"conversation_id": conversationID,
	}
	if strings.TrimSpace(jobPostID) != "" {
		record["job_post_id"] = jobPostID
	}
	recordJSON, _ := json.Marshal(record)

	if s.db != nil {
		s.storeNotification(recipientUserID, title, body, "new_message", "/chat", recordJSON)
	}

	if s.fcm == nil || len(deviceTokens) == 0 {
		return nil
	}

	data := map[string]string{
		"type":              "new_message",
		"notification_time": time.Now().Format("2006-01-02 15:04:05"),
		"click_action":      "FLUTTER_NOTIFICATION_CLICK",
		"screen":            "/chat",
		"record":            string(recordJSON),
	}

	var firstErr error
	for _, token := range deviceTokens {
		if _, err := s.fcm.SendToDevice(ctx, token, title, body, data); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NotifyDisputeCreated notifies the other party that a dispute has been filed.
func (s *Service) NotifyDisputeCreated(ctx context.Context, recipientUserID, contractTitle, disputeID, contractID string) error {
	return s.notifyUser(
		ctx,
		recipientUserID,
		"Dispute Filed",
		"A dispute has been filed on contract: "+contractTitle,
		"dispute_created",
		"/dispute",
		map[string]string{"dispute_id": disputeID, "contract_id": contractID},
	)
}

// NotifyDisputeResolved notifies a party that a dispute has been resolved.
func (s *Service) NotifyDisputeResolved(ctx context.Context, recipientUserID, contractTitle, disputeID, contractID string) error {
	return s.notifyUser(
		ctx,
		recipientUserID,
		"Dispute Resolved",
		"A dispute on contract '"+contractTitle+"' has been resolved",
		"dispute_resolved",
		"/dispute",
		map[string]string{"dispute_id": disputeID, "contract_id": contractID},
	)
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKM * c
}

func (s *Service) NotifyNewJobPostedToInterestedUsers(ctx context.Context, job *models.JobPost, loc *models.JobPostLocation) error {
	if s.db == nil || s.fcm == nil || job == nil || loc == nil {
		return nil
	}

	var prefs []models.NotificationPreference
	if err := s.db.Preload("Categories").Where("enable_new_jobs = ?", true).Find(&prefs).Error; err != nil {
		return err
	}

	for _, pref := range prefs {
		if pref.UserID == job.CreatedByID {
			continue
		}

		if pref.City != "" && !strings.EqualFold(strings.TrimSpace(pref.City), strings.TrimSpace(loc.City)) {
			continue
		}

		if len(pref.Categories) > 0 {
			matchesCategory := false
			for _, c := range pref.Categories {
				if c.CategoryID == job.CategoryID {
					matchesCategory = true
					break
				}
			}
			if !matchesCategory {
				continue
			}
		}

		if pref.Latitude != nil && pref.Longitude != nil && pref.JobRadiusKM > 0 {
			distance := haversineKM(*pref.Latitude, *pref.Longitude, loc.Latitude, loc.Longitude)
			if distance > pref.JobRadiusKM {
				continue
			}
		}

		_ = s.notifyUser(
			ctx,
			pref.UserID,
			"New Job Posted",
			job.Title,
			"new_job",
			"/job",
			map[string]string{"job_post_id": job.ID, "category_id": job.CategoryID, "city": loc.City},
		)
	}

	return nil
}
