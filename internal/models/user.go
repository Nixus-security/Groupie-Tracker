package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Pseudo       string    `json:"pseudo"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserPublic struct {
	ID     int64  `json:"id"`
	Pseudo string `json:"pseudo"`
}

type RegisterRequest struct {
	Pseudo          string `json:"pseudo"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type Session struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (u *User) ToPublic() UserPublic {
	return UserPublic{
		ID:     u.ID,
		Pseudo: u.Pseudo,
	}
}
