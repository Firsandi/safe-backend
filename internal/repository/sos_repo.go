package repository

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"safe-backend/internal/model"
)

type SosRepository struct {
	db *sqlx.DB
}

func NewSosRepository(db *sqlx.DB) *SosRepository {
	return &SosRepository{db: db}
}

// CreateEvent registers a new SOS incident
func (r *SosRepository) CreateEvent(e *model.SosEvent) error {
	row := r.db.QueryRowx(
		`INSERT INTO sos_events (user_id, trigger_type, status, initial_latitude, initial_longitude, medical_snapshot)
         VALUES ($1, $2, $3, $4, $5, $6)
         RETURNING sos_id, created_at`,
		e.UserID, e.TriggerType, "active", e.InitialLatitude, e.InitialLongitude, e.MedicalSnapshot,
	)
	return row.Scan(&e.SosID, &e.CreatedAt)
}

// ResolveEvent resolves or cancels an active SOS event
func (r *SosRepository) ResolveEvent(sosID, userID string, status string) error {
	if status != "resolved" && status != "false_alarm" {
		return errors.New("status penyelesaian tidak valid")
	}

	res, err := r.db.Exec(
		"UPDATE sos_events SET status=$1 WHERE sos_id=$2 AND user_id=$3 AND status='active'",
		status, sosID, userID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return errors.New("kejadian SOS tidak ditemukan atau sudah dinonaktifkan")
	}
	return nil
}

// GetActiveEvent gets the active SOS event for a user if one exists
func (r *SosRepository) GetActiveEvent(userID string) (*model.SosEvent, error) {
	var event model.SosEvent
	err := r.db.Get(&event, "SELECT sos_id, user_id, trigger_type, status, initial_latitude, initial_longitude, medical_snapshot, created_at FROM sos_events WHERE user_id=$1 AND status='active' LIMIT 1", userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// GetEventDetail gets the complete details including tracking history
func (r *SosRepository) GetEventDetail(sosID string) (*model.SosEventDetailDTO, error) {
	var detail model.SosEventDetailDTO
	err := r.db.Get(&detail, `
		SELECT 
			s.sos_id, 
			s.user_id, 
			u.name AS user_name,
			u.phone_number AS user_phone,
			s.trigger_type, 
			s.status, 
			s.initial_latitude, 
			s.initial_longitude, 
			s.medical_snapshot, 
			s.created_at
		FROM sos_events s
		JOIN users u ON s.user_id = u.user_id
		WHERE s.sos_id = $1
	`, sosID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Fetch tracking points
	var trackingPoints []model.SosTracking
	err = r.db.Select(&trackingPoints, "SELECT tracking_id, sos_id, latitude, longitude, recorded_at FROM sos_tracking WHERE sos_id=$1 ORDER BY recorded_at ASC", sosID)
	if err != nil {
		return nil, err
	}
	detail.TrackingPoints = trackingPoints

	// Fetch responders
	var responders []model.SosAcknowledgement
	err = r.db.Select(&responders, `
		SELECT 
			sa.acknowledgement_id, 
			sa.sos_id, 
			sa.responder_id, 
			u.name AS responder_name, 
			sa.acknowledged_at 
		FROM sos_acknowledgements sa
		JOIN users u ON sa.responder_id = u.user_id
		WHERE sa.sos_id=$1 
		ORDER BY sa.acknowledged_at ASC`, 
		sosID,
	)
	if err != nil {
		responders = []model.SosAcknowledgement{}
	}
	detail.Responders = responders

	return &detail, nil
}

// GetSentHistory returns the history of SOS events triggered by the user
func (r *SosRepository) GetSentHistory(userID string) ([]model.SosEvent, error) {
	var events []model.SosEvent
	err := r.db.Select(&events, "SELECT sos_id, user_id, trigger_type, status, initial_latitude, initial_longitude, medical_snapshot, created_at FROM sos_events WHERE user_id=$1 ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	return events, nil
}

// GetReceivedHistory returns the SOS events triggered by users who registered current user as emergency contact
func (r *SosRepository) GetReceivedHistory(userID string) ([]model.SosEventDetailDTO, error) {
	var events []model.SosEventDetailDTO
	err := r.db.Select(&events, `
		SELECT 
			s.sos_id, 
			s.user_id, 
			u.name AS user_name,
			u.phone_number AS user_phone,
			s.trigger_type, 
			s.status, 
			s.initial_latitude, 
			s.initial_longitude, 
			s.medical_snapshot, 
			s.created_at
		FROM sos_events s
		JOIN users u ON s.user_id = u.user_id
		JOIN emergency_contacts c ON s.user_id = c.requester_id
		WHERE c.receiver_id = $1 AND c.status = 'accepted'
		
		UNION ALL
		
		SELECT 
			s.sos_id, 
			s.user_id, 
			u.name AS user_name,
			u.phone_number AS user_phone,
			s.trigger_type, 
			s.status, 
			s.initial_latitude, 
			s.initial_longitude, 
			s.medical_snapshot, 
			s.created_at
		FROM sos_events s
		JOIN users u ON s.user_id = u.user_id
		JOIN emergency_contacts c ON s.user_id = c.receiver_id
		WHERE c.requester_id = $1 AND c.status = 'accepted'
		
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}

	// Fetch tracking points for active events (if any)
	for i := range events {
		var trackingPoints []model.SosTracking
		err = r.db.Select(&trackingPoints, "SELECT tracking_id, sos_id, latitude, longitude, recorded_at FROM sos_tracking WHERE sos_id=$1 ORDER BY recorded_at ASC", events[i].SosID)
		if err == nil {
			events[i].TrackingPoints = trackingPoints
		} else {
			events[i].TrackingPoints = []model.SosTracking{}
		}
	}

	return events, nil
}

// AddTrackingPoint registers a new real-time GPS tracking point
func (r *SosRepository) AddTrackingPoint(t *model.SosTracking) error {
	row := r.db.QueryRowx(
		`INSERT INTO sos_tracking (sos_id, latitude, longitude)
         VALUES ($1, $2, $3)
         RETURNING tracking_id, recorded_at`,
		t.SosID, t.Latitude, t.Longitude,
	)
	return row.Scan(&t.TrackingID, &t.RecordedAt)
}

// AcknowledgeEvent registers that a contact has seen/read the SOS incident
func (r *SosRepository) AcknowledgeEvent(sosID, responderID string) error {
	_, err := r.db.Exec(`
		INSERT INTO sos_acknowledgements (sos_id, responder_id)
		VALUES ($1, $2)
		ON CONFLICT (sos_id, responder_id) DO NOTHING`,
		sosID, responderID,
	)
	return err
}
