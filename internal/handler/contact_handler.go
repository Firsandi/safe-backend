package handler

import (
	"net/http"
	"strings"
	"safe-backend/internal/model"
	"safe-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

type EmergencyContactHandler struct {
	repo *repository.EmergencyContactRepository
}

func NewEmergencyContactHandler(repo *repository.EmergencyContactRepository) *EmergencyContactHandler {
	return &EmergencyContactHandler{repo: repo}
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

	if err := h.repo.RespondToRequest(relationID, userID, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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
