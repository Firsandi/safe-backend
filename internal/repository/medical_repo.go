package repository

import (
	"database/sql"
	"github.com/jmoiron/sqlx"
	"safe-backend/internal/model"
)

type MedicalProfileRepository struct {
	db *sqlx.DB
}

func NewMedicalProfileRepository(db *sqlx.DB) *MedicalProfileRepository {
	return &MedicalProfileRepository{db: db}
}

func (r *MedicalProfileRepository) GetByUserID(userID string) (*model.MedicalProfile, error) {
	var profile model.MedicalProfile
	err := r.db.Get(&profile, "SELECT medical_id, user_id, blood_type, medical_notes FROM medical_profiles WHERE user_id=$1", userID)
	if err == sql.ErrNoRows {
		return nil, nil // Return nil, nil when no profile exists yet
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *MedicalProfileRepository) Upsert(p *model.MedicalProfile) error {
	existing, err := r.GetByUserID(p.UserID)
	if err != nil {
		return err
	}
	
	if existing != nil {
		_, err := r.db.Exec(
			"UPDATE medical_profiles SET blood_type=$1, medical_notes=$2 WHERE user_id=$3",
			p.BloodType, p.MedicalNotes, p.UserID,
		)
		if err != nil {
			return err
		}
		p.MedicalID = existing.MedicalID
		return nil
	}
	
	row := r.db.QueryRowx(
		`INSERT INTO medical_profiles (user_id, blood_type, medical_notes)
         VALUES ($1, $2, $3)
         RETURNING medical_id`,
		p.UserID, p.BloodType, p.MedicalNotes,
	)
	err = row.Scan(&p.MedicalID)
	return err
}
