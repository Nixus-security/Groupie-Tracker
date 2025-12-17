package auth

import (
	"context"
	"net/http"

	"groupie-tracker/internal/models"
)

type ContextKey string

const (
	UserContextKey    ContextKey = "user"
	SessionContextKey ContextKey = "session"
)

type Middleware struct {
	sessionManager *SessionManager
}

func NewMiddleware() *Middleware {
	return &Middleware{
		sessionManager: NewSessionManager(),
	}
}

func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.sessionManager.GetUserFromRequest(r)
		if err != nil {
			http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
			return
		}

		session, _ := m.sessionManager.GetSessionFromRequest(r)

		ctx := context.WithValue(r.Context(), UserContextKey, user)
		ctx = context.WithValue(ctx, SessionContextKey, session)

		m.sessionManager.ExtendSession(session.ID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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

func GetUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}

func GetSessionFromContext(ctx context.Context) *models.Session {
	session, ok := ctx.Value(SessionContextKey).(*models.Session)
	if !ok {
		return nil
	}
	return session
}

func IsAuthenticated(ctx context.Context) bool {
	return GetUserFromContext(ctx) != nil
}