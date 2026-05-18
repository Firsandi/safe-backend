package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		"[PUSH NOTIFICATION TRIGGERED]\n"+
		"Target FCM Token : %s\n"+
		"Title            : %s\n"+
		"Body             : %s\n"+
		"Custom Data      : %+v\n"+
		"======================================================\n",
		token, title, body, data,
	)
	return nil
}

// FcmNotificationService sends push notifications through the FCM HTTP v1 API
type FcmNotificationService struct {
	projectId string
	apiKey    string
}

func NewFcmNotificationService(projectId, apiKey string) *FcmNotificationService {
	return &FcmNotificationService{
		projectId: projectId,
		apiKey:    apiKey,
	}
}

func (s *FcmNotificationService) SendPush(token, title, body string, data map[string]string) error {
	if s.projectId == "" || s.apiKey == "" {
		// Fallback to Mock if Firebase is not fully configured
		log.Printf("[FCM Fallback] Firebase not fully configured. Logging instead.")
		mock := NewMockNotificationService()
		return mock.SendPush(token, title, body, data)
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectId)

	message := map[string]interface{}{
		"message": map[string]interface{}{
			"token": token,
			"notification": map[string]string{
				"title": title,
				"body":  body,
			},
			"data": data,
		},
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	// If they use API Key or OAuth2 token
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM returned status code %d", resp.StatusCode)
	}

	log.Printf("[FCM Success] Notification sent to token %s successfully", token)
	return nil
}

// GetNotificationService initializes the configured service based on environment variables
func GetNotificationService() NotificationService {
	log.Println("Initializing Local Mock Notification Service (FCM Disabled)...")
	return NewMockNotificationService()
}
