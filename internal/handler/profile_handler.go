package handler

import (
	"net/http"
	"strings"
	"safe-backend/internal/model"
	"safe-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

type MedicalProfileHandler struct {
	repo *repository.MedicalProfileRepository
}

func NewMedicalProfileHandler(repo *repository.MedicalProfileRepository) *MedicalProfileHandler {
	return &MedicalProfileHandler{repo: repo}
}

func (h *MedicalProfileHandler) GetMedicalProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	profile, err := h.repo.GetByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data profil medis"})
		return
	}

	if profile == nil {
		// Return empty default profile instead of 404 for premium UX
		c.JSON(http.StatusOK, model.MedicalProfile{
			UserID:       userID,
			BloodType:    "",
			MedicalNotes: "",
		})
		return
	}

	c.JSON(http.StatusOK, profile)
}

func (h *MedicalProfileHandler) UpsertMedicalProfile(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.MedicalProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validasi golongan darah
	bloodType := strings.ToUpper(strings.TrimSpace(req.BloodType))
	if bloodType != "" && bloodType != "A" && bloodType != "B" && bloodType != "AB" && bloodType != "O" &&
		bloodType != "A+" && bloodType != "A-" && bloodType != "B+" && bloodType != "B-" &&
		bloodType != "AB+" && bloodType != "AB-" && bloodType != "O+" && bloodType != "O-" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Golongan darah tidak valid. Gunakan: A, B, AB, O, atau dengan rhesus (+/-)"})
		return
	}

	profile := &model.MedicalProfile{
		UserID:       userID,
		BloodType:    bloodType,
		MedicalNotes: req.MedicalNotes,
	}

	if err := h.repo.Upsert(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan profil medis"})
		return
	}

	c.JSON(http.StatusOK, profile)
}
