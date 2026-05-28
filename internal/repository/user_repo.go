package repository

import (
	"github.com/jmoiron/sqlx"
	"safe-backend/internal/model"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var user model.User
	err := r.db.Get(&user,
		"SELECT user_id, name, email, password, phone_number, COALESCE(profile_image, '') AS profile_image, fcm_token, created_at, latitude AS last_latitude, longitude AS last_longitude, location_updated_at AS last_location_update FROM users WHERE email=$1",
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
		"SELECT user_id, name, email, password, phone_number, COALESCE(profile_image, '') AS profile_image, fcm_token, created_at, latitude AS last_latitude, longitude AS last_longitude, location_updated_at AS last_location_update FROM users WHERE user_id=$1",
		id,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Create(u *model.User) error {
	row := r.db.QueryRowx(
		`INSERT INTO users (name, email, password, phone_number, profile_image)
         VALUES ($1, $2, $3, $4, $5)
         RETURNING user_id, created_at`,
		u.Name, u.Email, u.Password, u.PhoneNumber, "",
	)
	return row.Scan(&u.UserID, &u.CreatedAt)
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
