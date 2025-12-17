package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"groupie-tracker/internal/database"
	"groupie-tracker/internal/models"
)

const (
	SessionCookieName = "session_id"
	SessionDuration   = 24 * time.Hour
)

var (
	ErrSessionNotFound = errors.New("session non trouvée")
	ErrSessionExpired  = errors.New("session expirée")
)

type SessionManager struct {
	db *sql.DB
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		db: database.GetDB(),
	}
}

func (sm *SessionManager) CreateSession(userID int64) (*models.Session, error) {
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresAt := now.Add(SessionDuration)

	_, err = sm.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}

	_, err = sm.db.Exec(
		"INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		sessionID, userID, now, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	return &models.Session{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

func (sm *SessionManager) GetSession(sessionID string) (*models.Session, error) {
	var session models.Session
	query := "SELECT id, user_id, created_at, expires_at FROM sessions WHERE id = ?"
	err := sm.db.QueryRow(query, sessionID).Scan(
		&session.ID, &session.UserID, &session.CreatedAt, &session.ExpiresAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}

	if time.Now().After(session.ExpiresAt) {
		sm.DeleteSession(sessionID)
		return nil, ErrSessionExpired
	}

	return &session, nil
}

func (sm *SessionManager) DeleteSession(sessionID string) error {
	_, err := sm.db.Exec("DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}

func (sm *SessionManager) ExtendSession(sessionID string) error {
	newExpiry := time.Now().Add(SessionDuration)
	_, err := sm.db.Exec(
		"UPDATE sessions SET expires_at = ? WHERE id = ?",
		newExpiry, sessionID,
	)
	return err
}

func (sm *SessionManager) CleanExpiredSessions() error {
	_, err := sm.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, session *models.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (*models.Session, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sm.GetSession(cookie.Value)
}

func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func (sm *SessionManager) GetUserFromRequest(r *http.Request) (*models.User, error) {
	session, err := sm.GetSessionFromRequest(r)
	if err != nil {
		return nil, err
	}

	authService := NewService()
	return authService.GetUserByID(session.UserID)
}

func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}