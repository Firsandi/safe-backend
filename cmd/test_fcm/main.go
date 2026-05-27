package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

func main() {
	// 1. Get FCM Token from args
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cmd/test_fcm/main.go <FCM_TOKEN> [title] [body]")
		fmt.Println("\nExample:")
		fmt.Println("  go run cmd/test_fcm/main.go \"fcm_token_device_anda\" \"Tes SOS\" \"Ini adalah notifikasi darurat!\"")
		os.Exit(1)
	}

	fcmToken := os.Args[1]
	title := "Tes SOS (Safe App)"
	body := "Ini adalah notifikasi darurat uji coba FCM!"

	if len(os.Args) >= 3 {
		title = os.Args[2]
	}
	if len(os.Args) >= 4 {
		body = os.Args[3]
	}

	// 2. Find service account key
	credentialsFile := "firebase-service-account.json"
	if _, err := os.Stat(credentialsFile); os.IsNotExist(err) {
		// Try parent directory
		credentialsFile = filepath.Join("..", "firebase-service-account.json")
		if _, err := os.Stat(credentialsFile); os.IsNotExist(err) {
			log.Fatalf("File credential Firebase (%s) tidak ditemukan!", credentialsFile)
		}
	}

	log.Printf("Menggunakan file credentials: %s", credentialsFile)
	opt := option.WithCredentialsFile(credentialsFile)

	// 3. Initialize Firebase App
	ctx := context.Background()
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		log.Fatalf("Gagal menginisialisasi firebase app: %v", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		log.Fatalf("Gagal mengambil messaging client: %v", err)
	}

	// 4. Construct message (matching the channels defined in the Flutter application)
	message := &messaging.Message{
		Token: fcmToken,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: map[string]string{
			"click_action": "FLUTTER_NOTIFICATION_CLICK",
			"sos_id":       "test-123-sos-id",
			"type":         "sos_alert",
			"latitude":     "-6.200000",
			"longitude":    "106.816666",
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				ChannelID: "emergency_channel_id_v2", // Must match the channel ID in your Flutter app config
				Sound:     "alarm_sound",             // The sound resource in res/raw
			},
		},
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "alarm_sound.caf",
				},
			},
		},
	}

	// 5. Send message
	log.Printf("Mengirim notifikasi ke token: %s...", fcmToken)
	res, err := client.Send(ctx, message)
	if err != nil {
		log.Fatalf("Gagal mengirim push via FCM: %v", err)
	}

	fmt.Printf("\n[FCM SUCCESS] Notifikasi berhasil dikirim!\nMessage ID: %s\n", res)
}
