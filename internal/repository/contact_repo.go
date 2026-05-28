package repository

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"safe-backend/internal/model"
)

type EmergencyContactRepository struct {
	db *sqlx.DB
}

func NewEmergencyContactRepository(db *sqlx.DB) *EmergencyContactRepository {
	return &EmergencyContactRepository{db: db}
}

// SendRequest sends an emergency contact request by receiver's email
func (r *EmergencyContactRepository) SendRequest(requesterID, receiverEmail string) error {
	// Find receiver user
	var receiver model.User
	err := r.db.Get(&receiver, "SELECT user_id FROM users WHERE email=$1", receiverEmail)
	if err == sql.ErrNoRows {
		return errors.New("pengguna dengan email tersebut tidak ditemukan")
	}
	if err != nil {
		return err
	}

	if receiver.UserID == requesterID {
		return errors.New("tidak dapat menambahkan diri sendiri sebagai kontak darurat")
	}

	// Check if relationship already exists
	var count int
	err = r.db.Get(&count, 
		"SELECT COUNT(*) FROM emergency_contacts WHERE (requester_id=$1 AND receiver_id=$2) OR (requester_id=$2 AND receiver_id=$1)", 
		requesterID, receiver.UserID,
	)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("hubungan kontak darurat sudah ada atau sedang menunggu persetujuan")
	}

	// Insert pending relationship
	_, err = r.db.Exec(
		"INSERT INTO emergency_contacts (requester_id, receiver_id, status) VALUES ($1, $2, 'pending')",
		requesterID, receiver.UserID,
	)
	return err
}

// RespondToRequest accepts or rejects a pending request
func (r *EmergencyContactRepository) RespondToRequest(relationID, receiverID string, accept bool) error {
	if accept {
		res, err := r.db.Exec(
			"UPDATE emergency_contacts SET status='accepted' WHERE contact_id=$1 AND receiver_id=$2 AND status='pending'",
			relationID, receiverID,
		)
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return errors.New("permintaan kontak darurat tidak ditemukan atau sudah diproses")
		}
		return nil
	} else {
		res, err := r.db.Exec(
			"DELETE FROM emergency_contacts WHERE contact_id=$1 AND receiver_id=$2 AND status='pending'",
			relationID, receiverID,
		)
		if err != nil {
			return err
		}
		rows, _ := res.RowsAffected()
		if rows == 0 {
			return errors.New("permintaan kontak darurat tidak ditemukan atau sudah diproses")
		}
		return nil
	}
}

// GetRequesterID fetches the requester_id of a specific contact relation
func (r *EmergencyContactRepository) GetRequesterID(relationID string) (string, error) {
	var requesterID string
	err := r.db.Get(&requesterID, "SELECT requester_id FROM emergency_contacts WHERE contact_id=$1", relationID)
	return requesterID, err
}

// GetContacts returns a list of contacts who have accepted the user's request (User sends SOS to them)
func (r *EmergencyContactRepository) GetContacts(userID string) ([]model.ContactRequestDTO, error) {
	var contacts []model.ContactRequestDTO
	// Query emergency contacts where current user is requester OR receiver and status is accepted
	err := r.db.Select(&contacts, `
		SELECT 
			c.contact_id, 
			c.requester_id, 
			c.receiver_id, 
			c.status,
			u.name AS name,
			u.email AS email,
			u.phone_number AS phone_number,
			u.fcm_token AS fcm_token
		FROM emergency_contacts c
		JOIN users u ON c.receiver_id = u.user_id
		WHERE c.requester_id = $1 AND c.status = 'accepted'
		
		UNION ALL
		
		SELECT 
			c.contact_id, 
			c.requester_id, 
			c.receiver_id, 
			c.status,
			u.name AS name,
			u.email AS email,
			u.phone_number AS phone_number,
			u.fcm_token AS fcm_token
		FROM emergency_contacts c
		JOIN users u ON c.requester_id = u.user_id
		WHERE c.receiver_id = $1 AND c.status = 'accepted'
	`, userID)
	if err != nil {
		return nil, err
	}
	return contacts, nil
}

// GetPendingRequests returns all pending incoming requests (Other users who want to add the current user)
func (r *EmergencyContactRepository) GetPendingRequests(userID string) ([]model.ContactRequestDTO, error) {
	var requests []model.ContactRequestDTO
	err := r.db.Select(&requests, `
		SELECT 
			c.contact_id, 
			c.requester_id, 
			c.receiver_id, 
			c.status,
			u.name AS name,
			u.email AS email,
			u.phone_number AS phone_number,
			u.fcm_token AS fcm_token
		FROM emergency_contacts c
		JOIN users u ON c.requester_id = u.user_id
		WHERE c.receiver_id = $1 AND c.status = 'pending'
	`, userID)
	if err != nil {
		return nil, err
	}
	return requests, nil
}

// GetContactsForFlutter returns a list of contacts with status mapped to UI expectations ('Tersambung' or 'Menunggu Konfirmasi')
func (r *EmergencyContactRepository) GetContactsForFlutter(userID string) ([]model.ContactResponseDTO, error) {
	var contacts []model.ContactResponseDTO
	err := r.db.Select(&contacts, `
		SELECT 
			c.contact_id AS id, 
			u.user_id AS user_id, 
			u.name AS name, 
			u.phone_number AS phone_number,
			COALESCE(u.profile_image, '') AS profile_image,
			CASE 
				WHEN c.status = 'accepted' THEN 'Tersambung'
				ELSE 'Menunggu Konfirmasi'
			END AS status,
			u.latitude AS last_latitude,
			u.longitude AS last_longitude,
			u.location_updated_at AS last_location_update
		FROM emergency_contacts c
		JOIN users u ON c.receiver_id = u.user_id
		WHERE c.requester_id = $1
		
		UNION ALL
		
		SELECT 
			c.contact_id AS id, 
			u.user_id AS user_id, 
			u.name AS name, 
			u.phone_number AS phone_number,
			COALESCE(u.profile_image, '') AS profile_image,
			'Tersambung' AS status,
			u.latitude AS last_latitude,
			u.longitude AS last_longitude,
			u.location_updated_at AS last_location_update
		FROM emergency_contacts c
		JOIN users u ON c.requester_id = u.user_id
		WHERE c.receiver_id = $1 AND c.status = 'accepted'
	`, userID)
	if err != nil {
		return nil, err
	}
	return contacts, nil
}

// GetPendingRequestsForFlutter returns incoming contact requests for Flutter
func (r *EmergencyContactRepository) GetPendingRequestsForFlutter(userID string) ([]model.ContactResponseDTO, error) {
	var requests []model.ContactResponseDTO
	err := r.db.Select(&requests, `
		SELECT 
			c.contact_id AS id, 
			u.user_id AS user_id, 
			u.name AS name, 
			u.phone_number AS phone_number,
			COALESCE(u.profile_image, '') AS profile_image,
			'Menunggu Konfirmasi' AS status,
			u.latitude AS last_latitude,
			u.longitude AS last_longitude,
			u.location_updated_at AS last_location_update
		FROM emergency_contacts c
		JOIN users u ON c.requester_id = u.user_id
		WHERE c.receiver_id = $1 AND c.status = 'pending'
	`, userID)
	if err != nil {
		return nil, err
	}
	return requests, nil
}

// SearchUsers searches the users database by email or phone number
func (r *EmergencyContactRepository) SearchUsers(query, normalized, currentUserID string) ([]model.ContactResponseDTO, error) {
	var users []model.ContactResponseDTO
	err := r.db.Select(&users, `
		SELECT 
			u.user_id AS id, 
			u.user_id AS user_id, 
			u.name, 
			u.phone_number, 
			COALESCE(u.profile_image, '') AS profile_image,
			COALESCE(
				CASE 
					WHEN ec.status = 'accepted' THEN 'Tersambung'
					WHEN ec.status = 'pending' THEN 'Menunggu Konfirmasi'
					ELSE ''
				END, 
				''
			) AS status,
			u.latitude AS last_latitude,
			u.longitude AS last_longitude,
			u.location_updated_at AS last_location_update
		FROM users u
		LEFT JOIN emergency_contacts ec ON 
			(ec.requester_id = $3 AND ec.receiver_id = u.user_id) OR
			(ec.requester_id = u.user_id AND ec.receiver_id = $3)
		WHERE (u.email = $1 OR u.phone_number = $1 OR u.phone_number = $2) AND u.user_id != $3
	`, query, normalized, currentUserID)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// AddContact adds a pending emergency contact by target user id
func (r *EmergencyContactRepository) AddContact(requesterID, targetUserID string) error {
	if requesterID == targetUserID {
		return errors.New("tidak dapat menambahkan diri sendiri sebagai kontak darurat")
	}

	var count int
	err := r.db.Get(&count, 
		"SELECT COUNT(*) FROM emergency_contacts WHERE (requester_id=$1 AND receiver_id=$2) OR (requester_id=$2 AND receiver_id=$1)", 
		requesterID, targetUserID,
	)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("hubungan kontak darurat sudah ada atau sedang menunggu persetujuan")
	}

	_, err = r.db.Exec(
		"INSERT INTO emergency_contacts (requester_id, receiver_id, status) VALUES ($1, $2, 'pending')",
		requesterID, targetUserID,
	)
	return err
}

// DeleteContact removes an emergency contact relationship
func (r *EmergencyContactRepository) DeleteContact(userID string, contactID string) error {
	res, err := r.db.Exec(
		"DELETE FROM emergency_contacts WHERE contact_id=$1 AND (requester_id=$2 OR receiver_id=$2)",
		contactID, userID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return errors.New("hubungan kontak darurat tidak ditemukan")
	}
	return nil
}
