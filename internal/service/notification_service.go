package service

import (
	"context"
	"fmt"
	"log"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// NotificationService defines the contract for sending push notifications
type NotificationService interface {
	SendPush(token, title, body string, data map[string]string) error
}

// MockNotificationService is a dummy notification sender for local development and testing
type MockNotificationService struct{}

func NewMockNotificationService() *MockNotificationService {
	return &MockNotificationService{}
}

func (s *MockNotificationService) SendPush(token, title, body string, data map[string]string) error {
	log.Printf("\n======================================================\n"+
		"[PUSH NOTIFICATION TRIGGERED - HIGH PRIORITY (DND BYPASS)]\n"+
		"Target FCM Token : %s\n"+
		"Title            : %s\n"+
		"Body             : %s\n"+
		"Android Channel  : emergency_channel_id\n"+
		"Android Sound    : alarm_sound\n"+
		"APNs Critical    : Volume=1.0, Sound=alarm_sound.caf\n"+
		"Custom Data      : %+v\n"+
		"======================================================\n",
		token, title, body, data,
	)
	return nil
}

// RealFcmNotificationService sends push notifications using the official Go Firebase Admin SDK
type RealFcmNotificationService struct {
	client *messaging.Client
}

func NewRealFcmNotificationService(credentialsFile string) (*RealFcmNotificationService, error) {
	ctx := context.Background()
	opt := option.WithCredentialsFile(credentialsFile)
	
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("gagal menginisialisasi firebase app: %v", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil messaging client: %v", err)
	}

	return &RealFcmNotificationService{client: client}, nil
}

func (s *RealFcmNotificationService) SendPush(token, title, body string, data map[string]string) error {
	ctx := context.Background()

	message := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: "emergency_channel_id", // Harus sama dengan Channel ID di Flutter
				Sound:     "alarm_sound",          // Membaca file alarm_sound.mp3 di res/raw
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority": "10",
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					CriticalSound: &messaging.CriticalSound{
						Critical: true, // Memintas mode silent/DND pada iOS
						Name:     "alarm_sound.caf",
						Volume:   1.0,
					},
				},
			},
		},
	}

	res, err := s.client.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("gagal mengirim push via FCM: %v", err)
	}

	log.Printf("[FCM SUCCESS] Notifikasi berhasil terkirim. Message ID: %s", res)
	return nil
}

// GetNotificationService initializes the configured service based on environment variables
func GetNotificationService() NotificationService {
	credsPath := os.Getenv("FIREBASE_CREDENTIALS_PATH")
	if credsPath != "" {
		log.Printf("Initializing Real Firebase Cloud Messaging Service with credentials from: %s", credsPath)
		realService, err := NewRealFcmNotificationService(credsPath)
		if err == nil {
			return realService
		}
		log.Printf("Gagal inisialisasi Real Firebase service: %v. Fallback ke Mock.", err)
	}

	log.Println("Initializing Local Mock Notification Service (FCM Disabled)...")
	return NewMockNotificationService()
}
