package handler

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"safe-backend/internal/model"
	"safe-backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	repo *repository.UserRepository
}

func NewAuthHandler(repo *repository.UserRepository) *AuthHandler {
	return &AuthHandler{repo: repo}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if err := h.repo.PrepareEmailForRegistration(email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare registration"})
		return
	}

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	user := &model.User{
		Name:          req.Name,
		Email:         email,
		Password:      string(hashed),
		PhoneNumber:   req.PhoneNumber,
		EmailVerified: false,
	}

	if err := h.repo.Create(user); err != nil {
		fmt.Printf("Error creating user: %v\n", err)
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered or database issue"})
		return
	}

	verificationToken, err := generateEmailVerificationOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create email verification OTP"})
		return
	}

	if err := h.repo.SaveEmailVerificationToken(user.UserID, verificationToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save email verification OTP"})
		return
	}

	if err := sendVerificationEmail(user.Email, verificationToken); err != nil {
		log.Printf("Failed to send verification email to %s: %v", user.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Akun berhasil dibuat, tetapi kode OTP gagal dikirim. Periksa konfigurasi email server atau kirim ulang OTP.",
			"user": user,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Akun berhasil dibuat. Masukkan kode OTP yang dikirim ke email sebelum login.",
		"user": user,
	})
}

func (h *AuthHandler) CancelRegistration(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if err := h.repo.DeleteUnverifiedUserByEmail(email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membatalkan registrasi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Registrasi dibatalkan. Silakan daftar ulang."})
}

func (h *AuthHandler) VerificationStatus(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	timing, err := h.repo.GetEmailVerificationTiming(email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Status verifikasi tidak ditemukan"})
		return
	}

	if timing.OtpExpiresInSeconds <= 0 {
		if err := h.repo.DeleteUnverifiedUserByEmail(email); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membersihkan registrasi kedaluwarsa"})
			return
		}
		c.JSON(http.StatusGone, gin.H{"error": "Kode OTP kedaluwarsa. Silakan daftar ulang."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"otp_expires_in_seconds":      timing.OtpExpiresInSeconds,
		"resend_available_in_seconds": timing.ResendAvailableInSeconds,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if err := h.repo.DeleteExpiredUnverifiedUserByEmail(email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check registration status"})
		return
	}

	user, err := h.repo.FindByEmail(email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email belum terdaftar. Silakan register terlebih dahulu."})
		return
	}

	if !user.EmailVerified {
		hasActiveOtp, err := h.repo.HasActiveEmailVerificationToken(user.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check OTP status"})
			return
		}
		if !hasActiveOtp {
			if err := h.repo.DeleteUnverifiedUserByEmail(email); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membersihkan registrasi kedaluwarsa"})
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Email belum terdaftar. Silakan register terlebih dahulu."})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email belum diverifikasi. Silakan verifikasi dengan kode OTP sebelum login."})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
		return
	}

	token, err := generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{Token: token, User: *user})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		OTP   string `json:"otp" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email dan kode OTP 6 digit wajib diisi"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	otp := strings.TrimSpace(req.OTP)

	user, err := h.repo.VerifyEmailByToken(email, otp)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Kode OTP tidak valid atau sudah kedaluwarsa"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email berhasil diverifikasi. Silakan login.",
		"user": user,
	})
}

func (h *AuthHandler) ResendVerificationEmail(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.repo.FindByEmail(strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Email belum terdaftar di aplikasi"})
		return
	}
	if user.EmailVerified {
		c.JSON(http.StatusOK, gin.H{"message": "Email sudah terverifikasi. Silakan login."})
		return
	}
	canResend, err := h.repo.CanResendEmailVerificationToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengecek batas kirim ulang OTP"})
		return
	}
	if !canResend {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Tunggu 3 menit sebelum meminta OTP baru."})
		return
	}

	verificationToken, err := generateEmailVerificationOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create email verification OTP"})
		return
	}
	if err := h.repo.SaveEmailVerificationToken(user.UserID, verificationToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save email verification OTP"})
		return
	}
	if err := sendVerificationEmail(user.Email, verificationToken); err != nil {
		log.Printf("Failed to resend verification email to %s: %v", user.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Kode OTP gagal dikirim. Hubungi admin aplikasi."})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Kode OTP baru sudah dikirim."})
}

func (h *AuthHandler) UpdateFcmToken(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Akses ditolak: token tidak valid"})
		return
	}

	var req struct {
		FcmToken string `json:"fcm_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.UpdateFcmToken(userID, req.FcmToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengupdate token FCM"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "FCM Token berhasil diperbarui"})
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Akses ditolak: token tidak valid"})
		return
	}

	var req struct {
		Name         string `json:"name" binding:"required"`
		PhoneNumber  string `json:"phone_number" binding:"required"`
		ProfileImage string `json:"profile_image"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.UpdateProfile(userID, req.Name, req.PhoneNumber, req.ProfileImage); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memperbarui profil"})
		return
	}

	user, err := h.repo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Profil diperbarui tetapi gagal memuat ulang data"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profil berhasil diperbarui", "user": user})
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	var req struct {
		IDToken string `json:"id_token"`
		Email   string `json:"email"`
		Name    string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var email string

	// Jika ada ID Token asli, lakukan verifikasi keamanan Google
	if req.IDToken != "" && req.IDToken != "simulated_token" {
		tokenInfoUrl := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", req.IDToken)
		resp, err := http.Get(tokenInfoUrl)
		if err != nil || resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token Google tidak valid atau kedaluwarsa"})
			return
		}
		defer resp.Body.Close()

		var googleClaims struct {
			Email         string `json:"email"`
			Name          string `json:"name"`
			EmailVerified string `json:"email_verified"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&googleClaims); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca data dari Google"})
			return
		}

		if googleClaims.EmailVerified != "true" || googleClaims.Email == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Email Google belum terverifikasi"})
			return
		}
		email = strings.ToLower(strings.TrimSpace(googleClaims.Email))
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token Google wajib valid. Mode simulasi tidak diizinkan untuk login."})
		return
	}

	// 2. Login Google hanya boleh untuk email yang sudah terdaftar di aplikasi.
	user, err := h.repo.FindByEmail(email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email Google belum terdaftar di aplikasi. Silakan daftar akun terlebih dahulu."})
		return
	}
	if !user.EmailVerified {
		if err := h.repo.MarkEmailVerified(user.UserID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menandai email sebagai terverifikasi"})
			return
		}
		user.EmailVerified = true
	}

	// 3. Generate JWT Token untuk session di Flutter
	token, err := generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membuat token sesi"})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{Token: token, User: *user})
}

func (h *AuthHandler) UpdateLocation(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Akses ditolak: token tidak valid"})
		return
	}

	var req struct {
		Latitude  *float64 `json:"latitude" binding:"required"`
		Longitude *float64 `json:"longitude" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.UpdateLocation(userID, *req.Latitude, *req.Longitude); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memperbarui lokasi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lokasi berhasil diperbarui"})
}

func generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

func generateEmailVerificationOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func sendVerificationEmail(to string, token string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	if host == "" || port == "" || username == "" || password == "" || from == "" {
		log.Printf("Email verification OTP for %s: %s", to, token)
		return fmt.Errorf("SMTP_HOST, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, and SMTP_FROM must be configured")
	}

	subject := "Kode OTP Verifikasi SAFE"
	body := fmt.Sprintf("Halo,\n\nKode OTP verifikasi email SAFE Anda adalah:\n\n%s\n\nKode berlaku selama 5 menit. Jangan bagikan kode ini kepada siapa pun.\n", token)
	message := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n" +
		body)

	auth := smtp.PlainAuth("", username, password, host)
	return smtp.SendMail(host+":"+port, auth, from, []string{to}, message)
}
