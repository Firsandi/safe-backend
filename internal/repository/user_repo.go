package repository

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"safe-backend/internal/model"
)

type UserRepository struct {
	db *sqlx.DB
}

type EmailVerificationTiming struct {
	OtpExpiresInSeconds      int `db:"otp_expires_in_seconds"`
	ResendAvailableInSeconds int `db:"resend_available_in_seconds"`
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := r.db.Get(&user,
		"SELECT user_id, name, email, password, phone_number, COALESCE(profile_image, '') AS profile_image, fcm_token, COALESCE(email_verified, false) AS email_verified, created_at FROM users WHERE email=$1",
		email,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id string) (*model.User, error) {
	var user model.User
	err := r.db.Get(&user,
		"SELECT user_id, name, email, password, phone_number, COALESCE(profile_image, '') AS profile_image, fcm_token, COALESCE(email_verified, false) AS email_verified, created_at FROM users WHERE user_id=$1",
		id,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Create(u *model.User) error {
	row := r.db.QueryRowx(
		`INSERT INTO users (name, email, password, phone_number, profile_image, email_verified)
         VALUES ($1, $2, $3, $4, $5, $6)
         RETURNING user_id, created_at`,
		u.Name, u.Email, u.Password, u.PhoneNumber, "", u.EmailVerified,
	)
	return row.Scan(&u.UserID, &u.CreatedAt)
}

func (r *UserRepository) PrepareEmailForRegistration(email string) error {
	existingUser, err := r.FindByEmail(email)
	if err != nil {
		return nil
	}
	if existingUser.EmailVerified {
		return nil
	}

	hasActiveOtp, err := r.HasActiveEmailVerificationToken(existingUser.UserID)
	if err != nil {
		return err
	}
	if hasActiveOtp {
		return nil
	}

	return r.DeleteUnverifiedUserByEmail(email)
}

func (r *UserRepository) DeleteExpiredUnverifiedUserByEmail(email string) error {
	_, err := r.db.Exec(
		`DELETE FROM users
         WHERE email=$1
           AND email_verified=false
           AND NOT EXISTS (
             SELECT 1 FROM email_verification_tokens
             WHERE user_id=users.user_id
               AND used_at IS NULL
               AND expires_at > NOW()
               AND created_at > NOW() - INTERVAL '5 minutes'
           )`,
		email,
	)
	return err
}

func (r *UserRepository) DeleteUnverifiedUserByEmail(email string) error {
	_, err := r.db.Exec("DELETE FROM users WHERE email=$1 AND email_verified=false", email)
	return err
}

func (r *UserRepository) HasActiveEmailVerificationToken(userID string) (bool, error) {
	var exists bool
	err := r.db.Get(&exists,
		`SELECT EXISTS (
           SELECT 1 FROM email_verification_tokens
           WHERE user_id=$1
             AND used_at IS NULL
             AND expires_at > NOW()
             AND created_at > NOW() - INTERVAL '5 minutes'
         )`,
		userID,
	)
	return exists, err
}

func (r *UserRepository) CanResendEmailVerificationToken(userID string) (bool, error) {
	var lastCreatedAt sql.NullTime
	err := r.db.Get(&lastCreatedAt,
		`SELECT created_at
         FROM email_verification_tokens
         WHERE user_id=$1
         ORDER BY created_at DESC
         LIMIT 1`,
		userID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, err
	}
	if !lastCreatedAt.Valid {
		return true, nil
	}
	return lastCreatedAt.Time.Before(time.Now().Add(-3 * time.Minute)), nil
}

func (r *UserRepository) GetEmailVerificationTiming(email string) (*EmailVerificationTiming, error) {
	var timing EmailVerificationTiming
	err := r.db.Get(&timing,
		`SELECT
           GREATEST(
             0,
             FLOOR(EXTRACT(EPOCH FROM (
               LEAST(evt.expires_at, evt.created_at + INTERVAL '5 minutes') - NOW()
             )))
           )::int AS otp_expires_in_seconds,
           GREATEST(
             0,
             FLOOR(EXTRACT(EPOCH FROM (
               evt.created_at + INTERVAL '3 minutes' - NOW()
             )))
           )::int AS resend_available_in_seconds
         FROM email_verification_tokens evt
         JOIN users u ON u.user_id = evt.user_id
         WHERE u.email=$1 AND u.email_verified=false AND evt.used_at IS NULL
         ORDER BY evt.created_at DESC
         LIMIT 1`,
		email,
	)
	if err != nil {
		return nil, err
	}
	return &timing, nil
}

func (r *UserRepository) SaveEmailVerificationToken(userID string, token string) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(
		"UPDATE email_verification_tokens SET used_at=NOW() WHERE user_id=$1 AND used_at IS NULL",
		userID,
	); err != nil {
		return err
	}

	_, err = tx.Exec(
		`INSERT INTO email_verification_tokens (user_id, token, expires_at)
         VALUES ($1, $2, NOW() + INTERVAL '5 minutes')`,
		userID, token,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *UserRepository) VerifyEmailByToken(email string, token string) (*model.User, error) {
	tx, err := r.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var userID string
	err = tx.Get(&userID,
		`SELECT evt.user_id
         FROM email_verification_tokens evt
         JOIN users u ON u.user_id = evt.user_id
         WHERE u.email=$1
           AND evt.token=$2
           AND evt.used_at IS NULL
           AND evt.expires_at > NOW()
           AND evt.created_at > NOW() - INTERVAL '5 minutes'`,
		email, token,
	)
	if err != nil {
		return nil, err
	}

	if _, err = tx.Exec("UPDATE users SET email_verified=true WHERE user_id=$1", userID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec("UPDATE email_verification_tokens SET used_at=NOW() WHERE token=$1", token); err != nil {
		return nil, err
	}

	var user model.User
	err = tx.Get(&user,
		"SELECT user_id, name, email, password, phone_number, COALESCE(profile_image, '') AS profile_image, fcm_token, COALESCE(email_verified, false) AS email_verified, created_at FROM users WHERE user_id=$1",
		userID,
	)
	if err != nil {
		return nil, err
	}

	return &user, tx.Commit()
}

func (r *UserRepository) MarkEmailVerified(userID string) error {
	_, err := r.db.Exec("UPDATE users SET email_verified=true WHERE user_id=$1", userID)
	return err
}

func (r *UserRepository) UpdateFcmToken(userID string, token string) error {
	_, err := r.db.Exec("UPDATE users SET fcm_token=$1 WHERE user_id=$2", token, userID)
	return err
}

func (r *UserRepository) UpdateProfile(userID string, name string, phoneNumber string, profileImage string) error {
	_, err := r.db.Exec("UPDATE users SET name=$1, phone_number=$2, profile_image=$3 WHERE user_id=$4", name, phoneNumber, profileImage, userID)
	return err
}

func (r *UserRepository) UpdateLocation(userID string, lat float64, lng float64) error {
	_, err := r.db.Exec("UPDATE users SET latitude=$1, longitude=$2, location_updated_at=NOW() WHERE user_id=$3", lat, lng, userID)
	return err
}
