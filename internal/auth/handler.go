// Package auth - handler.go
// Handlers HTTP pour inscription, connexion, déconnexion
package auth

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
)

// Handler gère les requêtes HTTP d'authentification
type Handler struct {
	service        *Service
	sessionManager *SessionManager
	templates      *template.Template
}

// NewHandler crée une nouvelle instance du handler
func NewHandler(templatesDir string) *Handler {
	// Fonctions personnalisées pour les templates
	funcMap := template.FuncMap{
		"slice": func(s string, start, end int) string {
			if start >= len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"eq": func(a, b interface{}) bool {
			return a == b
		},
	}

	// Charger les templates
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		log.Printf("⚠️ Erreur chargement templates auth: %v", err)
	}

	return &Handler{
		service:        NewService(),
		sessionManager: NewSessionManager(),
		templates:      tmpl,
	}
}

// RegisterRoutes enregistre les routes d'authentification
func (h *Handler) RegisterRoutes(mux *http.ServeMux, authMiddleware *Middleware) {
	// Pages (GET)
	mux.Handle("/register", authMiddleware.RedirectIfAuth(http.HandlerFunc(h.RegisterPage)))
	mux.Handle("/login", authMiddleware.RedirectIfAuth(http.HandlerFunc(h.LoginPage)))
	mux.Handle("/logout", http.HandlerFunc(h.Logout))

	// API (POST)
	mux.HandleFunc("/api/auth/register", h.APIRegister)
	mux.HandleFunc("/api/auth/login", h.APILogin)
	mux.HandleFunc("/api/auth/logout", h.APILogout)
	mux.Handle("/api/auth/me", authMiddleware.RequireAuthAPI(http.HandlerFunc(h.APIMe)))
}

// ============================================================================
// PAGES HTML
// ============================================================================

// RegisterPage affiche la page d'inscription
func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.handleRegisterForm(w, r)
		return
	}

	data := map[string]interface{}{
		"Title": "Inscription",
		"Error": r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "register.html", data); err != nil {
			log.Printf("❌ Erreur template register: %v", err)
			h.renderBasicRegisterPage(w, data)
		}
	} else {
		h.renderBasicRegisterPage(w, data)
	}
}

// LoginPage affiche la page de connexion
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.handleLoginForm(w, r)
		return
	}

	data := map[string]interface{}{
		"Title":    "Connexion",
		"Error":    r.URL.Query().Get("error"),
		"Redirect": r.URL.Query().Get("redirect"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "login.html", data); err != nil {
			log.Printf("❌ Erreur template login: %v", err)
			h.renderBasicLoginPage(w, data)
		}
	} else {
		h.renderBasicLoginPage(w, data)
	}
}

// Logout déconnecte l'utilisateur
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session, err := h.sessionManager.GetSessionFromRequest(r)
	if err == nil {
		h.sessionManager.DeleteSession(session.ID)
	}
	h.sessionManager.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ============================================================================
// HANDLERS DE FORMULAIRES
// ============================================================================

func (h *Handler) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	pseudo := r.FormValue("pseudo")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")
	
	// Support pour les deux noms de champ possibles
	if confirmPassword == "" {
		confirmPassword = r.FormValue("confirm_password")
	}

	// Vérifier que les mots de passe correspondent
	if password != confirmPassword {
		http.Redirect(w, r, "/register?error=Les+mots+de+passe+ne+correspondent+pas", http.StatusSeeOther)
		return
	}

	// Créer l'utilisateur
	user, err := h.service.Register(pseudo, email, password)
	if err != nil {
		http.Redirect(w, r, "/register?error="+err.Error(), http.StatusSeeOther)
		return
	}

	// Créer une session
	session, err := h.sessionManager.CreateSession(user.ID)
	if err != nil {
		http.Redirect(w, r, "/login?error=Erreur+création+session", http.StatusSeeOther)
		return
	}

	h.sessionManager.SetSessionCookie(w, session)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	identifier := r.FormValue("identifier") // pseudo ou email
	password := r.FormValue("password")
	redirect := r.FormValue("redirect")

	user, err := h.service.Login(identifier, password)
	if err != nil {
		http.Redirect(w, r, "/login?error=Identifiants+invalides", http.StatusSeeOther)
		return
	}

	session, err := h.sessionManager.CreateSession(user.ID)
	if err != nil {
		http.Redirect(w, r, "/login?error=Erreur+création+session", http.StatusSeeOther)
		return
	}

	h.sessionManager.SetSessionCookie(w, session)

	// Rediriger vers la page demandée ou l'accueil
	if redirect != "" && redirect != "/login" && redirect != "/register" {
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// ============================================================================
// API JSON
// ============================================================================

// RegisterRequest structure de la requête d'inscription
type RegisterRequest struct {
	Pseudo          string `json:"pseudo"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// LoginRequest structure de la requête de connexion
type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// AuthResponse structure de la réponse d'authentification
type AuthResponse struct {
	Success bool         `json:"success"`
	User    *UserDTO     `json:"user,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// UserDTO structure utilisateur pour l'API (sans données sensibles)
type UserDTO struct {
	ID     int64  `json:"id"`
	Pseudo string `json:"pseudo"`
	Email  string `json:"email"`
}

// APIRegister gère l'inscription via API JSON
func (h *Handler) APIRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "JSON invalide"})
		return
	}

	if req.Password != req.ConfirmPassword {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Les mots de passe ne correspondent pas"})
		return
	}

	user, err := h.service.Register(req.Pseudo, req.Email, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: err.Error()})
		return
	}

	session, err := h.sessionManager.CreateSession(user.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Erreur création session"})
		return
	}

	h.sessionManager.SetSessionCookie(w, session)

	json.NewEncoder(w).Encode(AuthResponse{
		Success: true,
		User: &UserDTO{
			ID:     user.ID,
			Pseudo: user.Pseudo,
			Email:  user.Email,
		},
	})
}

// APILogin gère la connexion via API JSON
func (h *Handler) APILogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Méthode non autorisée"})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "JSON invalide"})
		return
	}

	user, err := h.service.Login(req.Identifier, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Identifiants invalides"})
		return
	}

	session, err := h.sessionManager.CreateSession(user.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Erreur création session"})
		return
	}

	h.sessionManager.SetSessionCookie(w, session)

	json.NewEncoder(w).Encode(AuthResponse{
		Success: true,
		User: &UserDTO{
			ID:     user.ID,
			Pseudo: user.Pseudo,
			Email:  user.Email,
		},
	})
}

// APILogout gère la déconnexion via API
func (h *Handler) APILogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	session, err := h.sessionManager.GetSessionFromRequest(r)
	if err == nil {
		h.sessionManager.DeleteSession(session.ID)
	}
	h.sessionManager.ClearSessionCookie(w)

	json.NewEncoder(w).Encode(AuthResponse{Success: true})
}

// APIMe retourne l'utilisateur connecté
func (h *Handler) APIMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	user := GetUserFromContext(r.Context())
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(AuthResponse{Success: false, Error: "Non authentifié"})
		return
	}

	json.NewEncoder(w).Encode(AuthResponse{
		Success: true,
		User: &UserDTO{
			ID:     user.ID,
			Pseudo: user.Pseudo,
			Email:  user.Email,
		},
	})
}

// ============================================================================
// TEMPLATES DE SECOURS (si les fichiers HTML ne sont pas trouvés)
// ============================================================================

func (h *Handler) renderBasicRegisterPage(w http.ResponseWriter, data map[string]interface{}) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Inscription - Groupie Tracker</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="auth-page">
        <div class="auth-container">
            <div class="auth-card">
                <div class="auth-logo">
                    <span class="logo-icon">🎶</span>
                    <h1>Groupie Tracker</h1>
                    <p>Créer votre compte</p>
                </div>
                {{if .Error}}<div class="alert alert-danger">⚠️ {{.Error}}</div>{{end}}
                <form method="POST" action="/register">
                    <div class="form-group">
                        <label class="form-label" for="pseudo">Pseudo (avec majuscule)</label>
                        <input type="text" class="form-control" id="pseudo" name="pseudo" required minlength="3">
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="email">Email</label>
                        <input type="email" class="form-control" id="email" name="email" required>
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="password">Mot de passe (min 12 car.)</label>
                        <input type="password" class="form-control" id="password" name="password" required minlength="12">
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="confirmPassword">Confirmer le mot de passe</label>
                        <input type="password" class="form-control" id="confirmPassword" name="confirmPassword" required>
                    </div>
                    <button type="submit" class="btn btn-primary btn-lg btn-block">S'inscrire</button>
                </form>
                <div class="auth-footer">
                    <p>Déjà inscrit ? <a href="/login">Se connecter</a></p>
                </div>
            </div>
        </div>
    </div>
</body>
</html>`

	tmpl, _ := template.New("register").Parse(html)
	tmpl.Execute(w, data)
}

func (h *Handler) renderBasicLoginPage(w http.ResponseWriter, data map[string]interface{}) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Connexion - Groupie Tracker</title>
    <link rel="stylesheet" href="/static/css/style.css">
</head>
<body>
    <div class="auth-page">
        <div class="auth-container">
            <div class="auth-card">
                <div class="auth-logo">
                    <span class="logo-icon">🎵</span>
                    <h1>Groupie Tracker</h1>
                    <p>Connexion à votre compte</p>
                </div>
                {{if .Error}}<div class="alert alert-danger">⚠️ {{.Error}}</div>{{end}}
                <form method="POST" action="/login">
                    <input type="hidden" name="redirect" value="{{.Redirect}}">
                    <div class="form-group">
                        <label class="form-label" for="identifier">Pseudo ou Email</label>
                        <input type="text" class="form-control" id="identifier" name="identifier" required>
                    </div>
                    <div class="form-group">
                        <label class="form-label" for="password">Mot de passe</label>
                        <input type="password" class="form-control" id="password" name="password" required>
                    </div>
                    <button type="submit" class="btn btn-primary btn-lg btn-block">Se connecter</button>
                </form>
                <div class="auth-footer">
                    <p>Pas encore inscrit ? <a href="/register">S'inscrire</a></p>
                </div>
            </div>
        </div>
    </div>
</body>
</html>`

	tmpl, _ := template.New("login").Parse(html)
	tmpl.Execute(w, data)
}
