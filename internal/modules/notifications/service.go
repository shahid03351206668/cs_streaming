package notifications

import (
	"context"
	"tasksy/pkg/fcm"
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
func (s *Service) NotifyNewMessage(ctx context.Context, deviceToken, senderName string) error {
	_, err := s.fcm.SendToDevice(ctx, deviceToken,
		"New Message",
		senderName+" sent you a message",
		map[string]string{"type": "new_message"},
	)
	return err
}
