// Package rooms - Gestionnaire des salles de jeu
package rooms

import (
	"encoding/json"
	"fmt"
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
	templateDir string
}

// NewHandler crée un nouveau gestionnaire de salles
func NewHandler(templateDir string) *Handler {
	return &Handler{
		templateDir: templateDir,
	}
}

// HandleLobby affiche le lobby avec la liste des salles
func (h *Handler) HandleLobby(w http.ResponseWriter, r *http.Request) {
	// Vérifier l'authentification
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		// Rediriger vers la page de connexion si non authentifié
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	// Récupérer toutes les salles
	manager := GetManager()
	rooms := manager.GetAllRooms()

	// Préparer les données pour le template
	data := map[string]interface{}{
		"Title": "Salles de jeu",
		"User":  user,
		"Rooms": rooms,
	}

	// Charger et exécuter le template
	tmplPath := filepath.Join(h.templateDir, "rooms.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("[ROOMS] Erreur chargement template rooms.html: %v", err)
		// Fallback: servir le fichier HTML directement
		http.ServeFile(w, r, tmplPath)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ROOMS] Erreur exécution template: %v", err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}
}

// HandleRoom affiche une salle spécifique
func (h *Handler) HandleRoom(w http.ResponseWriter, r *http.Request) {
	// Vérifier l'authentification
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	// Extraire le code de la salle depuis l'URL
	code := strings.TrimPrefix(r.URL.Path, "/room/")
	if code == "" || code == "create" || code == "join" {
		http.Redirect(w, r, "/rooms", http.StatusSeeOther)
		return
	}

	// Récupérer la salle
	manager := GetManager()
	room, err := manager.GetRoom(code)
	if err != nil || room == nil {
		http.Error(w, "Salle introuvable", http.StatusNotFound)
		return
	}

	// Vérifier si l'utilisateur est dans la salle (accès direct au slice Players)
	var player *models.Player
	room.Mu.RLock()
	for _, p := range room.Players {
		if p.UserID == user.ID {
			player = p
			break
		}
	}
	room.Mu.RUnlock()

	if player == nil {
		// L'utilisateur n'est pas dans la salle, rediriger vers le lobby
		http.Redirect(w, r, "/rooms?error=not_in_room", http.StatusSeeOther)
		return
	}

	// Déterminer le template à utiliser selon le type de jeu
	var tmplFile string
	switch room.GameType {
	case models.GameTypeBlindTest:
		tmplFile = "room_blindtest.html"
	case models.GameTypePetitBac:
		tmplFile = "room_petitbac.html"
	default:
		tmplFile = "room.html"
	}

	// Préparer les données pour le template
	data := map[string]interface{}{
		"Title":  room.Name,
		"User":   user,
		"Room":   room,
		"Player": player,
	}

	// Charger et exécuter le template
	tmplPath := filepath.Join(h.templateDir, tmplFile)
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("[ROOMS] Erreur chargement template %s: %v", tmplFile, err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ROOMS] Erreur exécution template: %v", err)
		http.Error(w, "Erreur interne", http.StatusInternalServerError)
		return
	}
}

// HandleGetRooms renvoie la liste des salles en JSON
func (h *Handler) HandleGetRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	manager := GetManager()
	rooms := manager.GetAllRooms()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"rooms":   rooms,
	})
}

// HandleCreateRoom crée une nouvelle salle
func (h *Handler) HandleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	// Vérifier l'authentification
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Parser le formulaire
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erreur parsing formulaire", http.StatusBadRequest)
		return
	}

	roomName := r.FormValue("room_name")
	gameTypeStr := r.FormValue("game_type")

	// Validation
	if roomName == "" {
		http.Error(w, "Le nom de la salle est requis", http.StatusBadRequest)
		return
	}

	// Convertir le type de jeu en models.GameType
	var gameType models.GameType
	switch gameTypeStr {
	case "blindtest":
		gameType = models.GameTypeBlindTest
	case "petitbac":
		gameType = models.GameTypePetitBac
	default:
		http.Error(w, "Type de jeu invalide", http.StatusBadRequest)
		return
	}

	// Créer la salle avec la bonne signature
	// CreateRoom(name string, hostID int64, code string, gameType models.GameType)
	manager := GetManager()
	room, err := manager.CreateRoom(roomName, user.ID, "", gameType)
	if err != nil {
		log.Printf("[ROOMS] Erreur création salle: %v", err)
		http.Error(w, "Erreur création salle", http.StatusInternalServerError)
		return
	}

	log.Printf("[ROOMS] Salle créée: %s (%s) par %s", room.Code, room.Name, user.Pseudo)

	// Rediriger vers la salle
	http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
}

// HandleJoinRoom permet de rejoindre une salle
func (h *Handler) HandleJoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	// Vérifier l'authentification
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Parser le formulaire
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Erreur parsing formulaire", http.StatusBadRequest)
		return
	}

	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	if code == "" {
		http.Redirect(w, r, "/room/join?error=Code+requis", http.StatusSeeOther)
		return
	}

	// Rejoindre la salle
	manager := GetManager()
	room, err := manager.GetRoom(code)
	if err != nil || room == nil {
		http.Redirect(w, r, "/room/join?error=Salle+introuvable", http.StatusSeeOther)
		return
	}

	// Créer le joueur avec les infos de l'utilisateur
	player := &models.Player{
		UserID:  user.ID,
		Pseudo:  user.Pseudo,
		Score:   0,
		IsReady: false,
	}

	// Ajouter le joueur manuellement dans le slice Players
	room.Mu.Lock()
	
	// Vérifier si le joueur n'est pas déjà dans la salle
	playerExists := false
	for _, p := range room.Players {
		if p.UserID == user.ID {
			playerExists = true
			break
		}
	}

	if playerExists {
		room.Mu.Unlock()
		http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
		return
	}

	// Vérifier que la salle n'est pas pleine (max 8 joueurs)
	if len(room.Players) >= 8 {
		room.Mu.Unlock()
		http.Redirect(w, r, "/room/join?error=Salle+pleine", http.StatusSeeOther)
		return
	}

	room.Players = append(room.Players, player)
	room.Mu.Unlock()

	log.Printf("[ROOMS] %s a rejoint la salle %s", user.Pseudo, room.Code)

	// Rediriger vers la salle
	http.Redirect(w, r, "/room/"+room.Code, http.StatusSeeOther)
}

// HandleLeaveRoom permet de quitter une salle
func (h *Handler) HandleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	// Vérifier l'authentification
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Parser le JSON
	var req struct {
		RoomCode string `json:"room_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Erreur parsing JSON", http.StatusBadRequest)
		return
	}

	// Récupérer la salle
	manager := GetManager()
	room, err := manager.GetRoom(req.RoomCode)
	if err != nil || room == nil {
		http.Error(w, "Salle introuvable", http.StatusNotFound)
		return
	}

	// Retirer le joueur du slice Players
	room.Mu.Lock()
	for i, p := range room.Players {
		if p.UserID == user.ID {
			room.Players = append(room.Players[:i], room.Players[i+1:]...)
			break
		}
	}
	room.Mu.Unlock()

	log.Printf("[ROOMS] %s a quitté la salle %s", user.Pseudo, room.Code)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Vous avez quitté la salle",
	})
}

// HandleRestartRoom redémarre une partie terminée
func (h *Handler) HandleRestartRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	// Vérifier l'authentification
	sessionManager := auth.NewSessionManager()
	user, err := sessionManager.GetUserFromRequest(r)
	if err != nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Extraire le code depuis l'URL (/api/rooms/{code}/restart)
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	code := strings.TrimSuffix(path, "/restart")

	// Récupérer la salle
	manager := GetManager()
	room, err := manager.GetRoom(code)
	if err != nil || room == nil {
		http.Error(w, "Salle introuvable", http.StatusNotFound)
		return
	}

	// Vérifier que l'utilisateur est l'hôte
	if room.HostID != user.ID {
		http.Error(w, "Seul l'hôte peut redémarrer la partie", http.StatusForbidden)
		return
	}

	// Redémarrer la partie (réinitialiser les scores et le statut)
	room.Mu.Lock()
	room.Status = models.RoomStatusWaiting
	for _, player := range room.Players {
		player.Score = 0
		player.IsReady = false
	}
	room.Mu.Unlock()

	log.Printf("[ROOMS] Partie redémarrée dans la salle %s par %s", room.Code, user.Pseudo)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Partie redémarrée",
	})
}

