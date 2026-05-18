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
	row := r.db.QueryRowx(
		`INSERT INTO medical_profiles (user_id, blood_type, medical_notes)
         VALUES ($1, $2, $3)
         ON CONFLICT (medical_id) DO UPDATE 
         SET blood_type = EXCLUDED.blood_type, medical_notes = EXCLUDED.medical_notes
         RETURNING medical_id`,
		p.UserID, p.BloodType, p.MedicalNotes,
	)
	
	// Wait, is there a unique constraint or primary key conflict we should handle?
	// The table has a PRIMARY KEY `medical_id` DEFAULT gen_random_uuid().
	// But to do upsert properly based on user_id, since user_id is 1-to-1:
	// We can check if a record already exists for the user_id. If yes, update it. If not, insert it.
	// That is simpler and doesn't rely on complex unique constraints if they are not defined.
	// Let's implement it with a simple exists check or INSERT ON CONFLICT.
	// Wait, the table definition in migration is:
	// CREATE TABLE IF NOT EXISTS medical_profiles (
	//     medical_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	//     user_id       UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
	//     blood_type     VARCHAR(5),
	//     medical_notes TEXT
	// );
	// There is no UNIQUE constraint on user_id in the migration!
	// So we should do a check in Go: if a profile already exists, UPDATE it, otherwise INSERT.
	// Let's do exactly that to prevent multiple rows per user!
	
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
	
	err = row.Scan(&p.MedicalID)
	return err
}
