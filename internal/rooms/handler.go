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
	templates      *template.Template
}

// NewHandler crée un nouveau handler de salles
func NewHandler(templateDir string) *Handler {
	// Charger les templates
	tmpl, err := template.ParseGlob(filepath.Join(templateDir, "*.html"))
	if err != nil {
		log.Printf("[Rooms] Erreur chargement templates: %v", err)
	}

	// Charger les partials
	if tmpl != nil {
		partials, err := template.ParseGlob(filepath.Join(templateDir, "partials", "*.html"))
		if err == nil && partials != nil {
			for _, t := range partials.Templates() {
				tmpl.AddParseTree(t.Name(), t.Tree)
			}
		}
	}

	return &Handler{
		manager:        GetManager(),
		sessionManager: auth.NewSessionManager(),
		templates:      tmpl,
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
		"User":  user,
		"Rooms": rooms,
	}

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "lobby", data); err != nil {
			log.Printf("[Rooms] Erreur template lobby: %v", err)
			http.Error(w, "Erreur interne", http.StatusInternalServerError)
		}
	} else {
		// Fallback JSON si pas de templates
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

	room, err := h.manager.GetRoom(roomID)
	if err != nil {
		http.Redirect(w, r, "/lobby?error=room_not_found", http.StatusSeeOther)
		return
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
		_, err = h.manager.JoinRoom(roomID, user.ID, user.Pseudo)
		if err != nil {
			http.Redirect(w, r, "/lobby?error="+err.Error(), http.StatusSeeOther)
			return
		}
	}

	// Vérifier si l'utilisateur est l'hôte
	isHost := room.HostID == user.ID

	data := map[string]interface{}{
		"User":   user,
		"Room":   room,
		"IsHost": isHost,
	}

	if h.templates != nil {
		if err := h.templates.ExecuteTemplate(w, "room", data); err != nil {
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

	// Récupérer le nom de la salle (NOUVEAU: nom personnalisé, pas le pseudo)
	roomName := strings.TrimSpace(r.FormValue("room_name"))
	gameTypeStr := r.FormValue("game_type")

	// Valider le nom de la salle
	if len(roomName) < 3 || len(roomName) > 50 {
		// Si pas de nom valide, utiliser un nom par défaut
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

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Données invalides", http.StatusBadRequest)
		return
	}

	// Trouver la salle par code
	room, err := h.manager.GetRoomByCode(req.Code)
	if err != nil {
		jsonError(w, "Salle non trouvée", http.StatusNotFound)
		return
	}

	// Rejoindre la salle
	_, err = h.manager.JoinRoom(room.ID, user.ID, user.Pseudo)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
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