package notifications

import (
	"context"
	"encoding/json"
	"tasksy/pkg/fcm"
	"time"
)

type Service struct {
	fcm *fcm.FCMClient
}

func NewService(fcmClient *fcm.FCMClient) *Service {
	return &Service{fcm: fcmClient}
}

// NotifyProposalReceived notifies a job poster when a proposal is submitted
func (s *Service) NotifyProposalReceived(ctx context.Context, deviceToken, jobTitle string) error {
	_, err := s.fcm.SendToDevice(ctx, deviceToken,
		"New Proposal Received",
		"You received a new proposal for: "+jobTitle,
		map[string]string{"type": "proposal_received"},
	)
	return err
}

// NotifyProposalDecision notifies a freelancer when their proposal is accepted/rejected
func (s *Service) NotifyProposalDecision(ctx context.Context, deviceToken, decision string) error {
	title := "Proposal Accepted 🎉"
	if decision == "rejected" {
		title = "Proposal Update"
	}
	_, err := s.fcm.SendToDevice(ctx, deviceToken,
		title,
		"Your proposal has been "+decision,
		map[string]string{"type": "proposal_decision", "decision": decision},
	)
	return err
}

// NotifyNewMessage notifies a user about a new chat message
// and includes the conversation_id inside a "record" object in the data payload
// so the mobile app can deep-link into the correct chat screen.
func (s *Service) NotifyNewMessage(ctx context.Context, deviceToken, senderName, conversationID, jobPostID string) error {
	record := map[string]string{
		"id":         conversationID,
		"jobpost_id": jobPostID,
	}

	recordJSON, _ := json.Marshal(record)

	data := map[string]string{
		"type":              "new_message",
		"notification_time": time.Now().Format("2006-01-02 15:04:05"),
		"click_action":      "FLUTTER_NOTIFICATION_CLICK",
		"screen":            "/chat",
		"record":            string(recordJSON),
	}

	_, err := s.fcm.SendToDevice(ctx, deviceToken,
		"New Message",
		senderName+" sent you a message",
		data,
	)
	return err
}
