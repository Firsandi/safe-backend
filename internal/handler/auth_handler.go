package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
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
		if errors.Is(err, repository.ErrEmailAlreadyRegistered) {
			c.JSON(http.StatusConflict, gin.H{"error": "Email sudah terdaftar. Silakan login."})
			return
		}
		if errors.Is(err, repository.ErrEmailVerificationIsPending) {
			c.JSON(http.StatusConflict, gin.H{"error": "Email sudah didaftarkan dan menunggu verifikasi OTP."})
			return
		}
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

	var bloodType, medicalNotes string
	if req.BloodType != nil {
		bloodType = *req.BloodType
	}
	if req.MedicalNotes != nil {
		medicalNotes = *req.MedicalNotes
	}

	if err := h.repo.CreateWithMedical(user, bloodType, medicalNotes); err != nil {
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

	go func(email, token string) {
		if err := sendVerificationEmail(email, token, "Kode OTP Verifikasi SAFE", "Verifikasi Pendaftaran", "Terima kasih telah mendaftar di SAFE. Silakan gunakan kode OTP di bawah ini untuk memverifikasi alamat email Anda:"); err != nil {
			log.Printf("Failed to send verification email to %s: %v", email, err)
		}
	}(user.Email, verificationToken)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Akun berhasil dibuat. Masukkan kode OTP yang dikirim ke email sebelum login.",
		"user":    user,
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

	if req.DeviceToken != nil && *req.DeviceToken != "" {
		// Verify device token
		if _, err := h.repo.VerifyTrustedDevice(email, *req.DeviceToken); err == nil {
			// Device token valid, bypass OTP
			token, err := generateToken(user.UserID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"token": token, "user": *user, "require_otp": false, "device_token": *req.DeviceToken})
			return
		}
	}

	// Device token invalid or not provided, generate OTP
	otp, err := generateEmailVerificationOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}

	if err := h.repo.SaveLoginOtpToken(user.UserID, otp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save OTP"})
		return
	}

	go func(email, token string) {
		if err := sendVerificationEmail(email, token, "Kode OTP Login SAFE", "Verifikasi Login", "Kami mendeteksi aktivitas login baru pada akun Anda. Gunakan kode OTP di bawah ini untuk melanjutkan:"); err != nil {
			log.Printf("Failed to send login OTP email to %s: %v", email, err)
		}
	}(user.Email, otp)

	c.JSON(http.StatusOK, gin.H{
		"message":     "OTP telah dikirim ke email Anda",
		"require_otp": true,
	})
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
		"user":    user,
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
	if err := sendVerificationEmail(user.Email, verificationToken, "Kode OTP Verifikasi SAFE", "Verifikasi Pendaftaran", "Berikut adalah kode OTP verifikasi pendaftaran akun SAFE Anda yang baru:"); err != nil {
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
		FcmToken string `json:"fcm_token"`
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
	var name string

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
		name = strings.TrimSpace(googleClaims.Name)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token Google wajib valid. Mode simulasi tidak diizinkan untuk login."})
		return
	}

	// 2. Login/Register Google
	user, err := h.repo.FindByEmail(email)
	if err != nil {
		// Auto-register new Google user
		hashed, err := bcrypt.GenerateFromPassword([]byte("google_oauth_placeholder_password_123!"), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
			return
		}

		if name == "" {
			name = strings.Split(email, "@")[0]
		}

		user = &model.User{
			Name:          name,
			Email:         email,
			Password:      string(hashed),
			PhoneNumber:   "",
			EmailVerified: true,
		}

		if err := h.repo.Create(user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mendaftarkan akun Google baru: " + err.Error()})
			return
		}
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
		"exp":     time.Now().Add(60 * 24 * time.Hour).Unix(),
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

func sendVerificationEmail(to string, token string, subject string, title string, description string) error {
	apiKey := os.Getenv("BREVO_API_KEY")
	from := os.Getenv("SMTP_FROM")

	if apiKey == "" || from == "" {
		log.Printf("Email verification OTP for %s: %s (Subject: %s)", to, token, subject)
		return fmt.Errorf("BREVO_API_KEY and SMTP_FROM must be configured")
	}

	subject := "Kode OTP Verifikasi SAFE"
	body := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OTP</title>
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; background-color: #ffffff; color: #111111;">
  <table width="100%" border="0" cellspacing="0" cellpadding="0" style="background-color: #ffffff; padding: 20px 0;">
    <tr>
      <td align="left" style="padding: 0 20px;">
        <p style="font-size: 16px; line-height: 1.5; color: #111111; margin: 0 0 20px 0;">Berikut adalah kode verifikasi OTP Anda:</p>
        <div style="font-family: monospace; font-size: 32px; font-weight: bold; color: #111111; letter-spacing: 4px; margin-bottom: 20px;">%s</div>
        <p style="font-size: 14px; line-height: 1.5; color: #666666; margin: 0;">Kode ini berlaku selama 5 menit. Jangan bagikan kode ini kepada siapa pun.</p>
      </td>
    </tr>
  </table>
</body>
</html>`, token)

	message := []byte("From: SAFE <" + from + ">\r\n" +
		"Reply-To: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n" +
		body)

	auth := smtp.PlainAuth("", username, password, host)
	return smtp.SendMail(host+":"+port, auth, from, []string{to}, message)
}

func sendPasswordResetEmail(to string, token string) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	if host == "" || port == "" || username == "" || password == "" || from == "" {
		log.Printf("Password reset OTP for %s: %s", to, token)
		return fmt.Errorf("SMTP settings not configured")
	}

	subject := "Reset Password SAFE"
	body := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OTP</title>
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; background-color: #ffffff; color: #111111;">
  <table width="100%" border="0" cellspacing="0" cellpadding="0" style="background-color: #ffffff; padding: 20px 0;">
    <tr>
      <td align="left" style="padding: 0 20px;">
        <p style="font-size: 16px; line-height: 1.5; color: #111111; margin: 0 0 20px 0;">Berikut adalah kode OTP untuk mereset password Anda:</p>
        <div style="font-family: monospace; font-size: 32px; font-weight: bold; color: #111111; letter-spacing: 4px; margin-bottom: 20px;">%s</div>
        <p style="font-size: 14px; line-height: 1.5; color: #666666; margin: 0;">Kode ini berlaku selama 5 menit. Jika Anda tidak meminta reset password, abaikan email ini.</p>
      </td>
    </tr>
  </table>
</body>
</html>`, token)

	message := []byte("From: SAFE <" + from + ">\r\n" +
		"Reply-To: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n" +
		body)

	auth := smtp.PlainAuth("", username, password, host)
	return sendMailWithTimeout(host, port, auth, from, []string{to}, message, 10*time.Second)
}

func sendMailWithTimeout(host string, port string, auth smtp.Auth, from string, to []string, msg []byte, timeout time.Duration) error {
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: timeout}

	var conn net.Conn
	var err error
	if port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	url := "https://api.brevo.com/v3/smtp/email"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to Brevo API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("brevo API returned status %s: %v", resp.Status, errResp)
	}

	return nil
}

// Trusted Devices & Login OTP

func (h *AuthHandler) VerifyLoginOtp(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		OTP   string `json:"otp" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := h.repo.VerifyLoginOtpToken(email, req.OTP)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kode OTP salah atau sudah kedaluwarsa"})
		return
	}

	token, err := generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	// Generate new device token (UUID-like random hex string)
	b := make([]byte, 16)
	rand.Read(b)
	deviceTokenStr := fmt.Sprintf("%x", b)

	if err := h.repo.SaveTrustedDevice(user.UserID, deviceTokenStr); err != nil {
		log.Printf("Failed to save trusted device token: %v", err)
		// We can still proceed even if saving device token fails, they just won't get bypass next time.
	}

	c.JSON(http.StatusOK, gin.H{
		"token":        token,
		"device_token": deviceTokenStr,
		"user":         *user,
	})
}

// Password Reset

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := h.repo.FindByEmail(email)
	if err != nil {
		// Do not leak if email exists or not, but for our case, user requested 404
		c.JSON(http.StatusNotFound, gin.H{"error": "Email tidak terdaftar"})
		return
	}
	if !user.EmailVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email belum diverifikasi"})
		return
	}

	otp, err := generateEmailVerificationOTP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate OTP"})
		return
	}

	if err := h.repo.SavePasswordResetToken(user.UserID, otp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save OTP"})
		return
	}

	go func(email, token string) {
		if err := sendVerificationEmail(email, token); err != nil {
			log.Printf("Failed to send password reset OTP email to %s: %v", email, err)
		}
	}(user.Email, otp)

	c.JSON(http.StatusOK, gin.H{"message": "Kode OTP untuk reset password telah dikirim ke email Anda"})
}

func (h *AuthHandler) VerifyResetOtp(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
		OTP   string `json:"otp" binding:"required,len=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	_, err := h.repo.VerifyPasswordResetToken(email, req.OTP)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kode OTP salah atau sudah kedaluwarsa"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Kode OTP valid"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		OTP         string `json:"otp" binding:"required,len=6"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	userID, err := h.repo.VerifyPasswordResetToken(email, req.OTP)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kode OTP salah atau sudah kedaluwarsa"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process new password"})
		return
	}

	if err := h.repo.UpdatePassword(userID, string(hashed)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengubah password"})
		return
	}

	if err := h.repo.MarkPasswordResetTokenUsed(req.OTP); err != nil {
		log.Printf("Failed to mark reset token used: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password berhasil diubah. Silakan login."})
}
