// Package auth - middleware.go
// Middleware pour protéger les routes nécessitant une authentification
package auth

import (
	"context"
	"net/http"

	"groupie-tracker/internal/models"
)

// ContextKey type pour les clés de contexte
type ContextKey string

const (
	// UserContextKey clé pour stocker l'utilisateur dans le contexte
	UserContextKey ContextKey = "user"
	
	// SessionContextKey clé pour stocker la session dans le contexte
	SessionContextKey ContextKey = "session"
)

// Middleware structure du middleware d'authentification
type Middleware struct {
	sessionManager *SessionManager
}

// NewMiddleware crée une nouvelle instance du middleware
func NewMiddleware() *Middleware {
	return &Middleware{
		sessionManager: NewSessionManager(),
	}
}

// RequireAuth middleware qui vérifie que l'utilisateur est connecté
// Redirige vers /login si non connecté
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.sessionManager.GetUserFromRequest(r)
		if err != nil {
			// Rediriger vers la page de connexion
			http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
			return
		}

		// Récupérer la session
		session, _ := m.sessionManager.GetSessionFromRequest(r)

		// Ajouter l'utilisateur et la session au contexte
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		ctx = context.WithValue(ctx, SessionContextKey, session)

		// Prolonger la session automatiquement
		m.sessionManager.ExtendSession(session.ID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthAPI middleware pour les API (retourne JSON au lieu de rediriger)
func (m *Middleware) RequireAuthAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.sessionManager.GetUserFromRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Non authentifié"}`))
			return
		}

		session, _ := m.sessionManager.GetSessionFromRequest(r)

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		ctx = context.WithValue(ctx, SessionContextKey, session)

		m.sessionManager.ExtendSession(session.ID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth middleware qui charge l'utilisateur s'il est connecté, mais ne bloque pas
func (m *Middleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.sessionManager.GetUserFromRequest(r)
		if err == nil && user != nil {
			session, _ := m.sessionManager.GetSessionFromRequest(r)
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			ctx = context.WithValue(ctx, SessionContextKey, session)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// RedirectIfAuth middleware qui redirige vers l'accueil si déjà connecté
// Utile pour les pages login/register
func (m *Middleware) RedirectIfAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.sessionManager.GetUserFromRequest(r)
		if err == nil && user != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// FONCTIONS HELPER POUR RÉCUPÉRER L'UTILISATEUR DU CONTEXTE
// ============================================================================

// GetUserFromContext récupère l'utilisateur depuis le contexte
func GetUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}

// GetSessionFromContext récupère la session depuis le contexte
func GetSessionFromContext(ctx context.Context) *models.Session {
	session, ok := ctx.Value(SessionContextKey).(*models.Session)
	if !ok {
		return nil
	}
	return session
}

// IsAuthenticated vérifie si l'utilisateur est authentifié
func IsAuthenticated(ctx context.Context) bool {
	return GetUserFromContext(ctx) != nil
}
