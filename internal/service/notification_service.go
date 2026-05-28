package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

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
		"Android Channel  : emergency_channel_id_v2\n"+
		"Android Sound    : alarm_sound\n"+
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

func NewRealFcmNotificationService(opt option.ClientOption) (*RealFcmNotificationService, error) {
	ctx := context.Background()
	
	var config *firebase.Config
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID != "" {
		config = &firebase.Config{
			ProjectID: projectID,
		}
		log.Printf("Using explicit FIREBASE_PROJECT_ID configuration: %s", projectID)
	}

	app, err := firebase.NewApp(ctx, config, opt)
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
				ChannelID: "emergency_channel_id_v2", // Harus sama dengan Channel ID di Flutter
				Sound:     "alarm_sound",             // Membaca file alarm_sound.mp3 di res/raw
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
	var opt option.ClientOption

	// 1. Coba load dari environment variable berisi JSON string
	credJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON")
	if credJSON == "" {
		credJSON = os.Getenv("FIREBASE_CREDENTIAL_JSON") // Fallback tanpa 'S' (sesuai dengan penulisan di .env)
	}

	credsPath := os.Getenv("FIREBASE_CREDENTIALS_PATH")
	if credsPath == "" {
		credsPath = os.Getenv("FIREBASE_CREDENTIAL_PATH") // Fallback tanpa 'S'
	}

	if credJSON != "" {
		log.Println("Initializing Real Firebase Service using FIREBASE_CREDENTIALS_JSON...")
		credJSON = strings.TrimSpace(credJSON)
		
		// Clean surrounding quotes added by environment parsing (e.g. Railway)
		if (strings.HasPrefix(credJSON, "\"") && strings.HasSuffix(credJSON, "\"")) || 
		   (strings.HasPrefix(credJSON, "'") && strings.HasSuffix(credJSON, "'")) {
			credJSON = credJSON[1 : len(credJSON)-1]
		}
		
		// Unescape double-escaped quotes if present
		credJSON = strings.ReplaceAll(credJSON, "\\\"", "\"")
		
		// We should NOT replace "\n" with raw newline bytes in the JSON string itself,
		// as raw newlines are invalid inside JSON string literals.
		// If the JSON was double-escaped (e.g., "\\n" instead of "\n"), we restore it to "\n".
		credJSON = strings.ReplaceAll(credJSON, "\\\\n", "\\n")
		
		opt = option.WithCredentialsJSON([]byte(credJSON))
	} else if credsPath != "" {
		// 2. Coba load dari file path yang ditentukan di env
		log.Printf("Initializing Real Firebase Service using credentials file from: %s", credsPath)
		opt = option.WithCredentialsFile(credsPath)
	} else {
		// 3. Coba cari file default local jika ada di root directory proyek
		defaultPaths := []string{
			"firebase-credentials.json", 
			"firebase-service-account.json", 
			"../firebase-credentials.json", 
			"../firebase-service-account.json",
		}
		for _, path := range defaultPaths {
			if _, err := os.Stat(path); err == nil {
				log.Printf("Initializing Real Firebase Service using found local file: %s", path)
				opt = option.WithCredentialsFile(path)
				break
			}
		}
	}

	if opt != nil {
		realService, err := NewRealFcmNotificationService(opt)
		if err == nil {
			return realService
		}
		log.Printf("Gagal inisialisasi Real Firebase service: %v. Fallback ke Mock.", err)
	}

	log.Println("Initializing Local Mock Notification Service (FCM Disabled)...")
	return NewMockNotificationService()
}
