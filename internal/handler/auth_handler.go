package handler

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"safe-backend/internal/model"
	"safe-backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/resend/resend-go/v3"
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
	apiKey := os.Getenv("RESEND_API_KEY")
	from := os.Getenv("RESEND_FROM")

	if apiKey == "" || from == "" {
		log.Printf("Email verification OTP for %s: %s (Subject: %s)", to, token, subject)
		return fmt.Errorf("RESEND_API_KEY and RESEND_FROM must be configured")
	}

	client := resend.NewClient(apiKey)

	htmlTemplate := `<!DOCTYPE html>
<html lang="id">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body {
            margin: 0;
            padding: 0;
            background-color: #F3F4F6;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }
    </style>
</head>
<body style="margin: 0; padding: 0; background-color: #F3F4F6; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">
    <table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color: #F3F4F6; padding: 40px 20px;">
        <tr>
            <td align="center">
                <table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="max-width: 480px; background-color: #ffffff; border-radius: 24px; box-shadow: 0 10px 25px rgba(194, 26, 26, 0.05); overflow: hidden; border: 1px solid #E5E7EB;">
                    <!-- HEADER GRADIENT BAR -->
                    <tr>
                        <td align="center" style="background: linear-gradient(135deg, #C21A1A 0%%, #E53E3E 100%%); padding: 36px 32px 32px 32px;">
                            <!-- SHIELD ICON / BRAND -->
                            <table role="presentation" border="0" cellpadding="0" cellspacing="0" style="margin-bottom: 12px;">
                                <tr>
                                    <td align="center" style="background-color: rgba(255, 255, 255, 0.15); border-radius: 20px; width: 56px; height: 56px; text-align: center; vertical-align: middle; border: 1px solid rgba(255, 255, 255, 0.25);">
                                        <span style="font-size: 28px; line-height: 56px; display: block; margin: 0;">🛡️</span>
                                    </td>
                                </tr>
                            </table>
                            <h1 style="margin: 0; font-size: 26px; font-weight: 800; color: #ffffff; letter-spacing: 0.5px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">SAFE</h1>
                            <p style="margin: 6px 0 0 0; font-size: 11px; font-weight: 700; color: rgba(255, 255, 255, 0.9); text-transform: uppercase; letter-spacing: 2px; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;">%s</p>
                        </td>
                    </tr>
                    
                    <!-- BODY CONTENT -->
                    <tr>
                        <td style="padding: 40px 32px 32px 32px; color: #374151; font-size: 15px; line-height: 1.6;">
                            <p style="margin-top: 0; margin-bottom: 12px; font-weight: 700; color: #111827; font-size: 17px;">Halo,</p>
                            <p style="margin-top: 0; margin-bottom: 28px; color: #4B5563;">%s</p>
                            
                            <!-- OTP BOX -->
                            <table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="margin-bottom: 28px;">
                                <tr>
                                    <td align="center" style="background-color: #FFF5F5; border: 1px dashed #FEB2B2; border-radius: 16px; padding: 24px 0;">
                                        <span style="font-size: 11px; font-weight: 700; color: #E53E3E; text-transform: uppercase; letter-spacing: 2px; display: block; margin-bottom: 8px;">KODE VERIFIKASI OTP</span>
                                        <span style="font-family: 'Courier New', Courier, monospace; font-size: 38px; font-weight: 800; letter-spacing: 10px; color: #C21A1A; display: block; padding-left: 10px; margin: 0;">%s</span>
                                    </td>
                                </tr>
                            </table>
                            
                            <!-- INFORMATION -->
                            <table role="presentation" border="0" cellpadding="0" cellspacing="0" width="100%%" style="background-color: #F9FAFB; border: 1px solid #E5E7EB; border-radius: 12px; padding: 16px; margin-bottom: 24px;">
                                <tr>
                                    <td style="font-size: 13px; color: #6B7280; line-height: 1.6;">
                                        <span style="color: #C21A1A; font-weight: bold; margin-right: 4px;">•</span> Kode ini berlaku selama <strong>5 menit</strong>.<br>
                                        <span style="color: #C21A1A; font-weight: bold; margin-right: 4px;">•</span> Demi keamanan akun Anda, <strong>jangan bagikan kode ini</strong> kepada siapa pun.
                                    </td>
                                </tr>
                            </table>
                            
                            <p style="margin: 0; font-size: 12px; color: #9CA3AF; text-align: center; line-height: 1.5;">Jika Anda tidak merasa meminta kode ini, silakan abaikan email ini dengan aman.</p>
                        </td>
                    </tr>
                    
                    <!-- FOOTER -->
                    <tr>
                        <td align="center" style="padding: 32px; background-color: #F9FAFB; border-top: 1px solid #E5E7EB; color: #9CA3AF; font-size: 12px;">
                            <p style="margin: 0 0 4px 0; font-weight: 700; color: #4B5563; font-size: 13px;">SAFE App</p>
                            <p style="margin: 0; color: #6B7280; font-size: 12px;">Sistem Pengamanan dan Respons Darurat Cepat</p>
                            <p style="margin: 16px 0 0 0; font-size: 11px; color: #9CA3AF;">© 2026 SAFE. Hak Cipta Dilindungi.</p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`

	body := fmt.Sprintf(htmlTemplate, subject, title, description, token)

	params := &resend.SendEmailRequest{
		From:    from,
		To:      []string{to},
		Subject: subject,
		Html:    body,
	}

	_, err := client.Emails.Send(params)
	if err != nil {
		return fmt.Errorf("failed to send email via Resend: %w", err)
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
		if err := sendVerificationEmail(email, token, "Kode OTP Reset Password SAFE", "Reset Kata Sandi", "Kami menerima permintaan untuk mengatur ulang kata sandi akun SAFE Anda. Gunakan kode OTP di bawah ini untuk melanjutkan:"); err != nil {
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
