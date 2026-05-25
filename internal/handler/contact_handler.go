package handler

import (
	"fmt"
	"net/http"
	"strings"
	"safe-backend/internal/model"
	"safe-backend/internal/repository"
	"safe-backend/internal/service"

	"github.com/gin-gonic/gin"
)

type EmergencyContactHandler struct {
	repo     *repository.EmergencyContactRepository
	userRepo *repository.UserRepository
	notifier service.NotificationService
}

func NewEmergencyContactHandler(
	repo *repository.EmergencyContactRepository,
	userRepo *repository.UserRepository,
	notifier service.NotificationService,
) *EmergencyContactHandler {
	return &EmergencyContactHandler{
		repo:     repo,
		userRepo: userRepo,
		notifier: notifier,
	}
}

// SearchUsers handles search queries by phone or email
func (h *EmergencyContactHandler) SearchUsers(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query pencarian tidak boleh kosong"})
		return
	}

	// Normalisasi nomor telepon Indonesia (+62 / 0 / 62)
	normalized := query
	if strings.HasPrefix(query, "0") {
		normalized = "+62" + query[1:]
	} else if strings.HasPrefix(query, "62") {
		normalized = "+" + query
	}

	users, err := h.repo.SearchUsers(query, normalized, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mencari pengguna"})
		return
	}

	if users == nil {
		users = []model.ContactResponseDTO{}
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// AddContact adds another user as an emergency contact
func (h *EmergencyContactHandler) AddContact(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req model.AddContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.AddContact(userID, req.TargetUserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Push Notification in background
	go func() {
		sender, errSender := h.userRepo.FindByID(userID)
		target, errTarget := h.userRepo.FindByID(req.TargetUserID)
		if errSender == nil && errTarget == nil && target.FcmToken != nil && *target.FcmToken != "" {
			title := "Permintaan Kontak Darurat"
			body := fmt.Sprintf("%s ingin menambahkan Anda sebagai kontak darurat.", sender.Name)
			dataPayload := map[string]string{
				"type":        "contact_request",
				"sender_id":   userID,
				"sender_name": sender.Name,
			}
			_ = h.notifier.SendPush(*target.FcmToken, title, body, dataPayload)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Permintaan kontak darurat berhasil dikirim"})
}

// ListContacts returns outgoing accepted and pending contacts
func (h *EmergencyContactHandler) ListContacts(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	contacts, err := h.repo.GetContactsForFlutter(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil daftar kontak darurat"})
		return
	}

	if contacts == nil {
		contacts = []model.ContactResponseDTO{}
	}

	c.JSON(http.StatusOK, gin.H{"contacts": contacts})
}

// ListPendingRequests returns incoming pending contact requests
func (h *EmergencyContactHandler) ListPendingRequests(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	requests, err := h.repo.GetPendingRequestsForFlutter(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil daftar permintaan masuk"})
		return
	}

	if requests == nil {
		requests = []model.ContactResponseDTO{}
	}

	c.JSON(http.StatusOK, gin.H{"requests": requests})
}

// AcceptRequest accepts a pending contact request
func (h *EmergencyContactHandler) AcceptRequest(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	relationID := c.Param("id")
	if relationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID Permintaan diperlukan"})
		return
	}

	// Fetch requesterID to send push notification
	requesterID, _ := h.repo.GetRequesterID(relationID)

	if err := h.repo.RespondToRequest(relationID, userID, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Push Notification back to requester in background
	if requesterID != "" {
		go func() {
			receiver, errReceiver := h.userRepo.FindByID(userID)
			requester, errRequester := h.userRepo.FindByID(requesterID)
			if errReceiver == nil && errRequester == nil && requester.FcmToken != nil && *requester.FcmToken != "" {
				title := "Permintaan Kontak Diterima"
				body := fmt.Sprintf("%s menyetujui permintaan kontak darurat Anda.", receiver.Name)
				dataPayload := map[string]string{
					"type":           "contact_accepted",
					"responder_id":   userID,
					"responder_name": receiver.Name,
				}
				_ = h.notifier.SendPush(*requester.FcmToken, title, body, dataPayload)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permintaan kontak darurat diterima"})
}

// RejectRequest rejects/deletes a pending contact request
func (h *EmergencyContactHandler) RejectRequest(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	relationID := c.Param("id")
	if relationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID Permintaan diperlukan"})
		return
	}

	if err := h.repo.RespondToRequest(relationID, userID, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Permintaan kontak darurat ditolak"})
}

// DeleteContact removes/disconnects an emergency contact
func (h *EmergencyContactHandler) DeleteContact(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	contactID := c.Param("id")
	if contactID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID Kontak diperlukan"})
		return
	}

	if err := h.repo.DeleteContact(userID, contactID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Kontak darurat berhasil dihapus"})
}
