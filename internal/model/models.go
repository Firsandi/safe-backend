package model

import (
	"encoding/json"
	"time"
)

// MedicalProfile represents the medical details of a user
type MedicalProfile struct {
	MedicalID    string `db:"medical_id"    json:"medical_id"`
	UserID       string `db:"user_id"       json:"user_id"`
	BloodType    string `db:"blood_type"    json:"blood_type"`
	MedicalNotes string `db:"medical_notes" json:"medical_notes"`
}

// MedicalProfileRequest represents the request body to upsert medical details
type MedicalProfileRequest struct {
	BloodType    string `json:"blood_type"    binding:"required"`
	MedicalNotes string `json:"medical_notes" binding:"required"`
}

// EmergencyContact represents the relation between requester and receiver
type EmergencyContact struct {
	ContactID   string `db:"contact_id"   json:"contact_id"`
	RequesterID string `db:"requester_id" json:"requester_id"`
	ReceiverID  string `db:"receiver_id"  json:"receiver_id"`
	Status      string `db:"status"       json:"status"` // 'pending', 'accepted', 'rejected'
}

// ContactRequest represents a request to add another user by email
type ContactRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ContactResponse represents a response to accept or reject an emergency contact
type ContactResponse struct {
	RelationID string `json:"relation_id" binding:"required"`
	Accept     bool   `json:"accept"`
}

// ContactRequestDTO represents the detailed pending or accepted contact relation
type ContactRequestDTO struct {
	ContactID   string    `db:"contact_id"   json:"contact_id"`
	RequesterID string    `db:"requester_id" json:"requester_id"`
	ReceiverID  string    `db:"receiver_id"  json:"receiver_id"`
	Status      string    `db:"status"       json:"status"`
	Name        string    `db:"name"         json:"name"`
	Email       string    `db:"email"        json:"email"`
	PhoneNumber string    `db:"phone_number" json:"phone_number"`
	FcmToken    *string   `db:"fcm_token"    json:"fcm_token,omitempty"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
}

// ContactResponseDTO represents the structure expected by the Flutter frontend
type ContactResponseDTO struct {
	ID           string  `db:"id"            json:"id"`
	Name         string  `db:"name"          json:"name"`
	PhoneNumber  string  `db:"phone_number"  json:"phone_number"`
	ProfileImage *string `db:"profile_image" json:"profile_image"`
	Status       string  `db:"status"        json:"status"`
}

// AddContactRequest represents the request body to add a contact by user id
type AddContactRequest struct {
	TargetUserID string `json:"target_user_id" binding:"required"`
	ContactName  string `json:"contact_name"`
	PhoneNumber  string `json:"phone_number"`
}

// SosEvent represents a registered SOS incident
type SosEvent struct {
	SosID            string          `db:"sos_id"            json:"sos_id"`
	UserID           string          `db:"user_id"           json:"user_id"`
	TriggerType      string          `db:"trigger_type"      json:"trigger_type"` // 'manual', 'auto'
	Status           string          `db:"status"            json:"status"`       // 'active', 'resolved', 'false_alarm'
	InitialLatitude  float64         `db:"initial_latitude"  json:"initial_latitude"`
	InitialLongitude float64         `db:"initial_longitude" json:"initial_longitude"`
	MedicalSnapshot  json.RawMessage `db:"medical_snapshot"  json:"medical_snapshot"`
	CreatedAt        time.Time       `db:"created_at"        json:"created_at"`
}

// TriggerSosRequest is the payload to launch an SOS event
type TriggerSosRequest struct {
	TriggerType string  `json:"trigger_type" binding:"required,oneof=manual auto"`
	Latitude    float64 `json:"latitude"     binding:"required"`
	Longitude   float64 `json:"longitude"    binding:"required"`
}

// TrackLocationRequest is the payload to push coordinate updates during active share location
type TrackLocationRequest struct {
	Latitude  float64 `json:"latitude"  binding:"required"`
	Longitude float64 `json:"longitude" binding:"required"`
}

// SosTracking represents a location stream point for an active SOS event
type SosTracking struct {
	TrackingID string    `db:"tracking_id" json:"tracking_id"`
	SosID      string    `db:"sos_id"      json:"sos_id"`
	Latitude   float64   `db:"latitude"    json:"latitude"`
	Longitude  float64   `db:"longitude"   json:"longitude"`
	RecordedAt time.Time `db:"recorded_at" json:"recorded_at"`
}

// SosEventDetailDTO is the complete detail of an SOS event, with snapshots and tracking list
type SosEventDetailDTO struct {
	SosID            string          `db:"sos_id"            json:"sos_id"`
	UserID           string          `db:"user_id"           json:"user_id"`
	UserName         string          `db:"user_name"         json:"user_name"`
	UserPhone        string          `db:"user_phone"        json:"user_phone"`
	TriggerType      string          `db:"trigger_type"      json:"trigger_type"`
	Status           string          `db:"status"            json:"status"`
	InitialLatitude  float64         `db:"initial_latitude"  json:"initial_latitude"`
	InitialLongitude float64         `db:"initial_longitude" json:"initial_longitude"`
	MedicalSnapshot  json.RawMessage `db:"medical_snapshot"  json:"medical_snapshot"`
	CreatedAt        time.Time       `db:"created_at"        json:"created_at"`
	TrackingPoints   []SosTracking   `json:"tracking_points"`
}
