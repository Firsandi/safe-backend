package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

	// Hash password
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	user := &model.User{
		Name:        req.Name,
		Email:       req.Email,
		Password:    string(hashed),
		PhoneNumber: req.PhoneNumber,
	}

	if err := h.repo.Create(user); err != nil {
		fmt.Printf("Error creating user: %v\n", err)
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered or database issue"})
		return
	}

	token, err := generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusCreated, model.AuthResponse{Token: token, User: *user})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.repo.FindByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
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

func (h *AuthHandler) UpdateLocation(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Akses ditolak: token tidak valid"})
		return
	}

	var req struct {
		Latitude  float64 `json:"latitude" binding:"required"`
		Longitude float64 `json:"longitude" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.UpdateLocation(userID, req.Latitude, req.Longitude); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memperbarui lokasi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lokasi berhasil diperbarui"})
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
		email = googleClaims.Email
		name = googleClaims.Name
	} else {
		// Mode Simulasi (Fallback untuk testing lokal tanpa SHA-1 setup)
		if req.Email == "" || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Email dan Nama wajib diisi dalam mode simulasi"})
			return
		}
		email = req.Email
		name = req.Name
	}

	// 2. Cek/Daftarkan email di database Supabase
	user, err := h.repo.FindByEmail(email)
	if err != nil {
		user = &model.User{
			Name:        name,
			Email:       email,
			Password:    "", // Kosong karena autentikasi dilakukan via Google
			PhoneNumber: "+62800000000",
		}

		if err := h.repo.Create(user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mendaftarkan user Google ke database"})
			return
		}
	}

	// 3. Generate JWT Token untuk session di Flutter
	token, err := generateToken(user.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membuat token sesi"})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{Token: token, User: *user})
}

func generateToken(userID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}