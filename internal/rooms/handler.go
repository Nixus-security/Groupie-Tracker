// Package rooms - handler.go
// Gère les requêtes HTTP pour les salles de jeu
package rooms

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"groupie-tracker/internal/auth"
	"groupie-tracker/internal/models"
)

// Handler gère les requêtes HTTP liées aux salles
type Handler struct {
	manager        *Manager
	sessionManager *auth.SessionManager
	templates      map[string]*template.Template
	templateDir    string
}

// NewHandler crée un nouveau handler de salles
func NewHandler(templateDir string) *Handler {
	h := &Handler{
		manager:        GetManager(),
		sessionManager: auth.NewSessionManager(),
		templates:      make(map[string]*template.Template),
		templateDir:    templateDir,
	}

	// Charger les templates individuellement
	h.loadTemplates()

	return h
}

// loadTemplates charge tous les templates
func (h *Handler) loadTemplates() {
	templateFiles := []string{
		"rooms.html",
		"room.html",
		"create_room.html",
		"join_room.html",
		"index.html",
	}

	for _, file := range templateFiles {
		path := filepath.Join(h.templateDir, file)
		tmpl, err := template.ParseFiles(path)
		if err != nil {
			log.Printf("[Rooms] Erreur chargement template %s: %v", file, err)
			continue
		}
		// Utiliser le nom sans extension comme clé
		name := strings.TrimSuffix(file, ".html")
		h.templates[name] = tmpl
		log.Printf("[Rooms] Template chargé: %s", name)
	}
}

// GetManager retourne le manager de salles
func (h *Handler) GetManager() *Manager {
	return h.manager
}

// ============================================================================
// PAGES HTML
// ============================================================================

// HandleLobby affiche la page du lobby
func (h *Handler) HandleLobby(w http.ResponseWriter, r *http.Request) {
	user, _ := h.sessionManager.GetUserFromRequest(r)

	// Si pas connecté, rediriger vers login
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	rooms := h.manager.GetAllRooms()

	data := map[string]interface{}{
		"Title": "Salles de jeu",
		"User":  user,
		"Rooms": rooms,
	}

	tmpl, ok := h.templates["rooms"]
	if ok && tmpl != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[Rooms] Erreur template lobby: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	} else {
		// Fallback JSON si pas de templates
		log.Printf("[Rooms] Template 'rooms' non trouvé, fallback JSON")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}

// HandleRoom affiche la page d'une salle
func (h *Handler) HandleRoom(w http.ResponseWriter, r *http.Request) {
	// Extraire l'ID de la salle de l'URL
	path := strings.TrimPrefix(r.URL.Path, "/room/")
	roomID := strings.Split(path, "/")[0]

	if roomID == "" {
		http.Redirect(w, r, "/lobby", http.StatusSeeOther)
		return
	}

	// Chercher par code d'abord, puis par ID
	room, err := h.manager.GetRoomByCode(roomID)
	if err != nil {
		room, err = h.manager.GetRoom(roomID)
		if err != nil {
			http.Redirect(w, r, "/lobby?error=room_not_found", http.StatusSeeOther)
			return
		}
	}

	user, err := h.sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/login?redirect=/room/"+roomID, http.StatusSeeOther)
		return
	}

	// Faire rejoindre le joueur s'il n'est pas déjà dans la salle
	room.Mutex.RLock()
	_, inRoom := room.Players[user.ID]
	room.Mutex.RUnlock()

	if !inRoom {
		_, err = h.manager.JoinRoom(room.ID, user.ID, user.Pseudo)
		if err != nil {
			http.Redirect(w, r, "/lobby?error="+err.Error(), http.StatusSeeOther)
			return
		}
	}

	// Vérifier si l'utilisateur est l'hôte
	isHost := room.HostID == user.ID

	data := map[string]interface{}{
		"Title":  room.Name,
		"User":   user,
		"Room":   room,
		"IsHost": isHost,
	}

	tmpl, ok := h.templates["room"]
	if ok && tmpl != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[Rooms] Erreur template room: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}

// ============================================================================
// API JSON
// ============================================================================

// HandleCreateRoom gère la création d'une salle (API)
func (h *Handler) HandleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.sessionManager.GetUserFromRequest(r)
	if err != nil {
		jsonError(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Parser le formulaire
	if err := r.ParseForm(); err != nil {
		jsonError(w, "Données invalides", http.StatusBadRequest)
		return
	}

	// Récupérer le nom de la salle
	roomName := strings.TrimSpace(r.FormValue("room_name"))
	if roomName == "" {
		roomName = strings.TrimSpace(r.FormValue("name"))
	}
	gameTypeStr := r.FormValue("game_type")

	// Valider le nom de la salle
	if len(roomName) < 3 || len(roomName) > 50 {
		roomName = user.Pseudo + "'s Room"
	}

	// Déterminer le type de jeu
	var gameType models.GameType
	switch gameTypeStr {
	case "blindtest":
		gameType = models.GameTypeBlindTest
	case "petitbac":
		gameType = models.GameTypePetitBac
	default:
		gameType = models.GameTypeBlindTest
	}

	// Créer la salle avec le nom personnalisé
	room, err := h.manager.CreateRoom(roomName, user.ID, user.Pseudo, gameType)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirection vers la salle si c'est une requête de formulaire
	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
		return
	}

	jsonSuccess(w, map[string]interface{}{
		"room_id": room.ID,
		"code":    room.Code,
		"name":    room.Name,
	})
}

// HandleJoinRoom gère la jonction à une salle par code (API)
func (h *Handler) HandleJoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.sessionManager.GetUserFromRequest(r)
	if err != nil {
		jsonError(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Essayer de parser comme JSON d'abord
	var code string
	contentType := r.Header.Get("Content-Type")
	
	if strings.Contains(contentType, "application/json") {
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Données invalides", http.StatusBadRequest)
			return
		}
		code = req.Code
	} else {
		// Formulaire
		if err := r.ParseForm(); err != nil {
			jsonError(w, "Données invalides", http.StatusBadRequest)
			return
		}
		code = r.FormValue("code")
	}

	code = strings.ToUpper(strings.TrimSpace(code))

	// Trouver la salle par code
	room, err := h.manager.GetRoomByCode(code)
	if err != nil {
		if strings.Contains(contentType, "application/json") {
			jsonError(w, "Salle non trouvée", http.StatusNotFound)
		} else {
			http.Redirect(w, r, "/lobby?error=Salle+non+trouvée", http.StatusSeeOther)
		}
		return
	}

	// Rejoindre la salle
	_, err = h.manager.JoinRoom(room.ID, user.ID, user.Pseudo)
	if err != nil {
		if strings.Contains(contentType, "application/json") {
			jsonError(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Redirect(w, r, "/lobby?error="+err.Error(), http.StatusSeeOther)
		}
		return
	}

	// Redirection si formulaire
	if !strings.Contains(contentType, "application/json") {
		http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
		return
	}

	jsonSuccess(w, map[string]interface{}{
		"room_id": room.ID,
	})
}

// HandleLeaveRoom gère le départ d'une salle (API)
func (h *Handler) HandleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.sessionManager.GetUserFromRequest(r)
	if err != nil {
		jsonError(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	var req struct {
		RoomID string `json:"room_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Données invalides", http.StatusBadRequest)
		return
	}

	if err := h.manager.LeaveRoom(req.RoomID, user.ID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	jsonSuccess(w, nil)
}

// HandleRestartRoom redémarre une partie (API)
func (h *Handler) HandleRestartRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.sessionManager.GetUserFromRequest(r)
	if err != nil {
		jsonError(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Extraire l'ID de la salle
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	roomID := strings.TrimSuffix(path, "/restart")

	// Vérifier que l'utilisateur est l'hôte
	if !h.manager.IsHost(roomID, user.ID) {
		jsonError(w, "Seul l'hôte peut relancer la partie", http.StatusForbidden)
		return
	}

	// Remettre la salle en attente
	if err := h.manager.UpdateRoomStatus(roomID, models.RoomStatusWaiting); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remettre les scores à zéro
	h.manager.ResetPlayerScores(roomID)

	// Remettre tous les joueurs en "non prêt"
	room, _ := h.manager.GetRoom(roomID)
	if room != nil {
		room.Mutex.Lock()
		for _, player := range room.Players {
			player.IsReady = false
		}
		room.Mutex.Unlock()
	}

	jsonSuccess(w, nil)
}

// HandleGetRooms retourne la liste des salles (API)
func (h *Handler) HandleGetRooms(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	var roomsList []*models.Room
	if status != "" {
		roomsList = h.manager.GetRoomsByStatus(models.RoomStatus(status))
	} else {
		roomsList = h.manager.GetAllRooms()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rooms":   roomsList,
	})
}

// ============================================================================
// FONCTIONS UTILITAIRES JSON
// ============================================================================

func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

func jsonSuccess(w http.ResponseWriter, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if data == nil {
		data = make(map[string]interface{})
	}
	data["success"] = true
	json.NewEncoder(w).Encode(data)
}