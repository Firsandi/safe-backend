package model

import "time"

type User struct {
	UserID       string    `db:"user_id"       json:"user_id"`
	Name         string    `db:"name"          json:"name"`
	Email        string    `db:"email"         json:"email"`
	Password     string    `db:"password"      json:"-"`
	PhoneNumber  string    `db:"phone_number"  json:"phone_number"`
	ProfileImage string    `db:"profile_image" json:"profile_image"`
	FcmToken     *string   `db:"fcm_token"     json:"fcm_token,omitempty"`
	EmailVerified bool     `db:"email_verified" json:"email_verified"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
}

type RegisterRequest struct {
	Name        string `json:"name"         binding:"required"`
	Email       string `json:"email"        binding:"required,email"`
	Password    string `json:"password"     binding:"required,min=6"`
	PhoneNumber string `json:"phone_number" binding:"required"`
	FcmToken    *string `json:"fcm_token"    binding:"omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
