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
		"SELECT user_id, name, email, password, phone_number, fcm_token, created_at FROM users WHERE email=$1",
		email,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) Create(u *model.User) error {
	row := r.db.QueryRowx(
		`INSERT INTO users (name, email, password, phone_number)
         VALUES ($1, $2, $3, $4)
         RETURNING user_id, created_at`,
		u.Name, u.Email, u.Password, u.PhoneNumber,
	)
	return row.Scan(&u.UserID, &u.CreatedAt)
}