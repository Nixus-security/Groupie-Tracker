package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"html/template"
	"net/http"
	"time"

	"music-platform/internal/config"
	"music-platform/internal/database"
	"music-platform/internal/middleware"
	"music-platform/internal/utils"
	"music-platform/internal/websocket"
)

var (
	templates *template.Template
	hub       *websocket.Hub
	cfg       *config.Config
)

func Setup(t *template.Template, h *websocket.Hub, c *config.Config) {
	templates = t
	hub = h
	cfg = c
}

func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderTemplate(w, "login.html", nil)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")
	password := r.FormValue("password")

	if identifier == "" || password == "" {
		renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Veuillez remplir tous les champs",
		})
		return
	}

	user, err := database.GetUserByIdentifier(identifier)
	if err != nil {
		renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Erreur serveur",
		})
		return
	}

	if user == nil {
		renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Identifiant ou mot de passe incorrect",
		})
		return
	}

	if !utils.CheckPassword(password, user.PasswordHash) {
		renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Identifiant ou mot de passe incorrect",
		})
		return
	}

	token, err := generateToken()
	if err != nil {
		renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Erreur serveur",
		})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := database.CreateSession(user.ID, token, expiresAt); err != nil {
		renderTemplate(w, "login.html", map[string]interface{}{
			"Error": "Erreur serveur",
		})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.SessionName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/lobby", http.StatusSeeOther)
}

func Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		renderTemplate(w, "register.html", nil)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	pseudo := r.FormValue("pseudo")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	data := map[string]interface{}{
		"Pseudo": pseudo,
		"Email":  email,
	}

	if pseudo == "" || email == "" || password == "" || confirmPassword == "" {
		data["Error"] = "Veuillez remplir tous les champs"
		renderTemplate(w, "register.html", data)
		return
	}

	if err := utils.ValidatePseudo(pseudo); err != nil {
		data["Error"] = err.Error()
		renderTemplate(w, "register.html", data)
		return
	}

	if err := utils.ValidateEmail(email); err != nil {
		data["Error"] = err.Error()
		renderTemplate(w, "register.html", data)
		return
	}

	if err := utils.ValidatePassword(password); err != nil {
		data["Error"] = err.Error()
		renderTemplate(w, "register.html", data)
		return
	}

	if password != confirmPassword {
		data["Error"] = "Les mots de passe ne correspondent pas"
		renderTemplate(w, "register.html", data)
		return
	}

	exists, err := database.PseudoExists(pseudo)
	if err != nil {
		data["Error"] = "Erreur serveur"
		renderTemplate(w, "register.html", data)
		return
	}
	if exists {
		data["Error"] = "Ce pseudo est déjà utilisé"
		renderTemplate(w, "register.html", data)
		return
	}

	exists, err = database.EmailExists(email)
	if err != nil {
		data["Error"] = "Erreur serveur"
		renderTemplate(w, "register.html", data)
		return
	}
	if exists {
		data["Error"] = "Cet email est déjà utilisé"
		renderTemplate(w, "register.html", data)
		return
	}

	hash, err := utils.HashPassword(password)
	if err != nil {
		data["Error"] = "Erreur serveur"
		renderTemplate(w, "register.html", data)
		return
	}

	userID, err := database.CreateUser(pseudo, email, hash)
	if err != nil {
		data["Error"] = "Erreur lors de la création du compte"
		renderTemplate(w, "register.html", data)
		return
	}

	token, err := generateToken()
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := database.CreateSession(userID, token, expiresAt); err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.SessionName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/lobby", http.StatusSeeOther)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cfg.SessionName)
	if err == nil {
		database.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.SessionName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
	}
}

func WebSocket(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non autorisé", http.StatusUnauthorized)
		return
	}

	dbUser := &database.User{
		ID:     user.ID,
		Pseudo: user.Pseudo,
		Email:  user.Email,
	}

	websocket.ServeWs(hub, w, r, dbUser)
}