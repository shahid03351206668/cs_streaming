package fcm

import (
	"context"

	"tasksy/pkg/logger"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"
	"google.golang.org/api/option"
)

type FCMClient struct {
	client *messaging.Client
}

func NewFCMClient(credentials string) (*FCMClient, error) {
	ctx := context.Background()
	config := &firebase.Config{ProjectID: "tasksy-40049"}
	opt := option.WithCredentialsJSON([]byte(credentials))

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		return nil, err
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}

	logger.Log.Info("FCM client initialized")
	return &FCMClient{client: client}, nil
}


func (f *FCMClient) SendToDevice(ctx context.Context, token, title, body string, data map[string]string) (string, error) {
	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
		Android: &messaging.AndroidConfig{
			Priority: "high",
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
				},
			},
		},
	}

	msgID, err := f.client.Send(ctx, message)
	if err != nil {
		logger.Log.Error("FCM send failed",
			zap.String("token", token),
			zap.String("title", title),
			zap.Error(err),
		)
	} else {
		logger.Log.Info("FCM send success",
			zap.String("message_id", msgID),
			zap.String("token", token),
			zap.String("title", title),
		)
	}
	return msgID, err
}

// SendToMultiple sends a notification to multiple device tokens
func (f *FCMClient) SendToMultiple(ctx context.Context, tokens []string, title, body string, data map[string]string) (*messaging.BatchResponse, error) {
	message := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	return f.client.SendEachForMulticast(ctx, message)
}

// SendToTopic sends a notification to a topic
func (f *FCMClient) SendToTopic(ctx context.Context, topic, title, body string, data map[string]string) (string, error) {
	message := &messaging.Message{
		Topic: topic,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	return f.client.Send(ctx, message)
}
