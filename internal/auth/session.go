// Package auth - session.go
// Gère les sessions utilisateur avec stockage SQLite
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
	// SessionCookieName nom du cookie de session
	SessionCookieName = "session_id"
	
	// SessionDuration durée de validité d'une session (24h)
	SessionDuration = 24 * time.Hour
)

var (
	ErrSessionNotFound = errors.New("session non trouvée")
	ErrSessionExpired  = errors.New("session expirée")
)

// SessionManager gère les sessions utilisateur
type SessionManager struct {
	db *sql.DB
}

// NewSessionManager crée un nouveau gestionnaire de sessions
func NewSessionManager() *SessionManager {
	return &SessionManager{
		db: database.GetDB(),
	}
}

// CreateSession crée une nouvelle session pour un utilisateur
func (sm *SessionManager) CreateSession(userID int64) (*models.Session, error) {
	// Générer un ID de session unique
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresAt := now.Add(SessionDuration)

	// Supprimer les anciennes sessions de cet utilisateur (optionnel, une seule session)
	_, err = sm.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}

	// Créer la nouvelle session
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

// GetSession récupère une session par son ID
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

	// Vérifier si la session a expiré
	if time.Now().After(session.ExpiresAt) {
		sm.DeleteSession(sessionID)
		return nil, ErrSessionExpired
	}

	return &session, nil
}

// DeleteSession supprime une session
func (sm *SessionManager) DeleteSession(sessionID string) error {
	_, err := sm.db.Exec("DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}

// DeleteUserSessions supprime toutes les sessions d'un utilisateur
func (sm *SessionManager) DeleteUserSessions(userID int64) error {
	_, err := sm.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// CleanExpiredSessions nettoie les sessions expirées (à appeler périodiquement)
func (sm *SessionManager) CleanExpiredSessions() error {
	_, err := sm.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

// ExtendSession prolonge une session existante
func (sm *SessionManager) ExtendSession(sessionID string) error {
	newExpiry := time.Now().Add(SessionDuration)
	_, err := sm.db.Exec(
		"UPDATE sessions SET expires_at = ? WHERE id = ?",
		newExpiry, sessionID,
	)
	return err
}

// ============================================================================
// FONCTIONS HTTP (COOKIES)
// ============================================================================

// SetSessionCookie définit le cookie de session dans la réponse HTTP
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, session *models.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,                    // Protection XSS
		Secure:   false,                   // Mettre true en production HTTPS
		SameSite: http.SameSiteLaxMode,    // Protection CSRF
	})
}

// GetSessionFromRequest récupère la session depuis la requête HTTP
func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (*models.Session, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return sm.GetSession(cookie.Value)
}

// ClearSessionCookie supprime le cookie de session
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

// GetUserFromRequest récupère l'utilisateur connecté depuis la requête
func (sm *SessionManager) GetUserFromRequest(r *http.Request) (*models.User, error) {
	session, err := sm.GetSessionFromRequest(r)
	if err != nil {
		return nil, err
	}

	authService := NewService()
	return authService.GetUserByID(session.UserID)
}

// ============================================================================
// FONCTIONS UTILITAIRES
// ============================================================================

// generateSessionID génère un ID de session aléatoire et sécurisé
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
