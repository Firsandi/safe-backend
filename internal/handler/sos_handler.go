package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"safe-backend/internal/model"
	"safe-backend/internal/repository"
	"safe-backend/internal/service"

	"github.com/gin-gonic/gin"
)

type SosHandler struct {
	sosRepo      *repository.SosRepository
	medicalRepo  *repository.MedicalProfileRepository
	contactRepo  *repository.EmergencyContactRepository
	userRepo     *repository.UserRepository
	notifier     service.NotificationService
}

func NewSosHandler(
	sosRepo *repository.SosRepository,
	medicalRepo *repository.MedicalProfileRepository,
	contactRepo *repository.EmergencyContactRepository,
	userRepo *repository.UserRepository,
	notifier service.NotificationService,
) *SosHandler {
	return &SosHandler{
		sosRepo:     sosRepo,
		medicalRepo: medicalRepo,
		contactRepo: contactRepo,
		userRepo:    userRepo,
		notifier:    notifier,
	}
}

// TriggerSos handles manual and auto SOS triggers
func (h *SosHandler) TriggerSos(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.TriggerSosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if there is already an active SOS
	activeEvent, err := h.sosRepo.GetActiveEvent(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memeriksa status SOS aktif"})
		return
	}
	if activeEvent != nil {
		// Return the active event
		c.JSON(http.StatusOK, activeEvent)
		return
	}

	// Get User Profile details for the push notification
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data profil pengguna"})
		return
	}

	// Fetch Medical Profile to take snapshot
	var medicalSnapshot json.RawMessage
	medProfile, err := h.medicalRepo.GetByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data medis untuk snapshot"})
		return
	}

	if medProfile != nil {
		snapshotBytes, err := json.Marshal(medProfile)
		if err == nil {
			medicalSnapshot = json.RawMessage(snapshotBytes)
		}
	}

	if len(medicalSnapshot) == 0 {
		// Fallback empty snapshot
		emptyProfile := model.MedicalProfile{
			UserID:       userID,
			BloodType:    "-",
			MedicalNotes: "Tidak ada catatan medis",
		}
		snapshotBytes, _ := json.Marshal(emptyProfile)
		medicalSnapshot = json.RawMessage(snapshotBytes)
	}

	// Create SOS Event
	event := &model.SosEvent{
		UserID:           userID,
		TriggerType:      req.TriggerType,
		InitialLatitude:  req.Latitude,
		InitialLongitude: req.Longitude,
		MedicalSnapshot:  medicalSnapshot,
	}

	if err := h.sosRepo.CreateEvent(event); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membuat kejadian SOS"})
		return
	}

	// Fetch all emergency contacts to notify them
	contacts, err := h.contactRepo.GetContacts(userID)
	if err == nil && len(contacts) > 0 {
		title := "EMERGENCY: BUTUH BANTUAN SEGERA!"
		triggerLabel := "Manual"
		if req.TriggerType == "auto" {
			title = "EMERGENCY: BENTURAN/KECELAKAAN TERDETEKSI!"
			triggerLabel = "Sensor Otomatis"
		}
		body := fmt.Sprintf("%s mengalami keadaan darurat (%s)! Segera periksa lokasi.", user.Name, triggerLabel)

		dataPayload := map[string]string{
			"sos_id":      event.SosID,
			"type":        "sos_alert",
			"user_name":   user.Name,
			"user_phone":  user.PhoneNumber,
			"latitude":    fmt.Sprintf("%f", req.Latitude),
			"longitude":   fmt.Sprintf("%f", req.Longitude),
			"trigger":     req.TriggerType,
		}

		// Send push notification to all emergency contacts
		for _, contact := range contacts {
			if contact.FcmToken != nil && *contact.FcmToken != "" {
				go func(token string) {
					_ = h.notifier.SendPush(token, title, body, dataPayload)
				}(*contact.FcmToken)
			}
		}
	}

	c.JSON(http.StatusCreated, event)
}

// ResolveSos finishes or cancels an active SOS
func (h *SosHandler) ResolveSos(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sosID := c.Param("id")
	if sosID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID SOS diperlukan"})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required,oneof=resolved false_alarm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.sosRepo.ResolveEvent(sosID, userID, req.Status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status SOS berhasil diperbarui", "status": req.Status})
}

// GetActiveSos checks if the current user has an active SOS event
func (h *SosHandler) GetActiveSos(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	activeEvent, err := h.sosRepo.GetActiveEvent(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memeriksa status SOS aktif"})
		return
	}

	if activeEvent == nil {
		c.JSON(http.StatusOK, gin.H{"active": false, "sos_id": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"active": true, "sos_id": activeEvent.SosID, "event": activeEvent})
}

// TrackLocation streams and appends new real-time GPS tracking coordinates
func (h *SosHandler) TrackLocation(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sosID := c.Param("id")
	if sosID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID SOS diperlukan"})
		return
	}

	var req model.TrackLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify the SOS belongs to the user and is active
	detail, err := h.sosRepo.GetEventDetail(sosID)
	if err != nil || detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Kejadian SOS tidak ditemukan"})
		return
	}
	if detail.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak: Anda bukan pemilik SOS ini"})
		return
	}
	if detail.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Kejadian SOS sudah tidak aktif"})
		return
	}

	tracking := &model.SosTracking{
		SosID:     sosID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}

	if err := h.sosRepo.AddTrackingPoint(tracking); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal merekam data pelacakan"})
		return
	}

	c.JSON(http.StatusCreated, tracking)
}

// GetSosDetail gets detailed information of a specific SOS
func (h *SosHandler) GetSosDetail(c *gin.Context) {
	sosID := c.Param("id")
	if sosID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID SOS diperlukan"})
		return
	}

	detail, err := h.sosRepo.GetEventDetail(sosID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil detail SOS"})
		return
	}
	if detail == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Detail SOS tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// GetSentHistory returns the SOS events triggered by the user
func (h *SosHandler) GetSentHistory(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	events, err := h.sosRepo.GetSentHistory(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil riwayat SOS dikirim"})
		return
	}

	c.JSON(http.StatusOK, events)
}

// GetReceivedHistory returns the SOS events received by the user
func (h *SosHandler) GetReceivedHistory(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	events, err := h.sosRepo.GetReceivedHistory(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil riwayat SOS diterima"})
		return
	}

	c.JSON(http.StatusOK, events)
}

// AcknowledgeSos marks that a receiver/contact has read/opened the SOS
func (h *SosHandler) AcknowledgeSos(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	sosID := c.Param("id")
	if sosID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID SOS diperlukan"})
		return
	}

	if err := h.sosRepo.AcknowledgeEvent(sosID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengirimkan sinyal balik SOS"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sinyal balik SOS berhasil terkirim"})
}
