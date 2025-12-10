// Package rooms gère les salles de jeu
package rooms

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"groupie-tracker/internal/database"
	"groupie-tracker/internal/models"
)

var (
	ErrRoomNotFound    = errors.New("salle non trouvée")
	ErrRoomFull        = errors.New("salle pleine")
	ErrAlreadyInRoom   = errors.New("déjà dans cette salle")
	ErrNotHost         = errors.New("seul l'hôte peut effectuer cette action")
	ErrGameInProgress  = errors.New("une partie est déjà en cours")
	ErrInvalidRoomName = errors.New("nom de salle invalide (3-50 caractères)")
)

const (
	// MaxPlayersPerRoom nombre maximum de joueurs par salle
	MaxPlayersPerRoom = 10

	// RoomCodeLength longueur du code de salle
	RoomCodeLength = 6

	// InactiveRoomTimeout durée avant suppression d'une salle inactive
	InactiveRoomTimeout = 2 * time.Hour
)

// Manager gère toutes les salles actives en mémoire
type Manager struct {
	rooms map[string]*models.Room // Clé = ID de la salle
	codes map[string]string       // Clé = code, Valeur = ID
	mutex sync.RWMutex
	db    *sql.DB
}

var (
	managerInstance *Manager
	managerOnce     sync.Once
)

// GetManager retourne l'instance singleton du manager de salles
func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			rooms: make(map[string]*models.Room),
			codes: make(map[string]string),
			db:    database.GetDB(),
		}
		// Démarrer le nettoyage périodique des salles inactives
		go managerInstance.cleanupInactiveRooms()
	})
	return managerInstance
}

// CreateRoom crée une nouvelle salle de jeu
// roomName est le nom personnalisé de la salle (PAS le pseudo du joueur)
func (m *Manager) CreateRoom(roomName string, hostID int64, hostPseudo string, gameType models.GameType) (*models.Room, error) {
	// Valider le nom de la salle
	roomName = strings.TrimSpace(roomName)
	if len(roomName) < 3 || len(roomName) > 50 {
		return nil, ErrInvalidRoomName
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Générer un ID unique
	roomID, err := generateRoomID()
	if err != nil {
		return nil, err
	}

	// Générer un code unique pour rejoindre
	code, err := m.generateUniqueCode()
	if err != nil {
		return nil, err
	}

	// Configuration par défaut selon le type de jeu
	config := models.GameConfig{}
	switch gameType {
	case models.GameTypeBlindTest:
		config.Playlist = "Pop"
		config.TimePerRound = models.BlindTestDefaultTime
	case models.GameTypePetitBac:
		config.Categories = models.DefaultPetitBacCategories
		config.NbRounds = models.NbrsManche
		config.UsedLetters = []string{}
	}

	room := &models.Room{
		ID:       roomID,
		Code:     code,
		Name:     roomName, // Utilise le nom personnalisé, PAS le pseudo
		HostID:   hostID,
		GameType: gameType,
		Status:   models.RoomStatusWaiting,
		Players: map[int64]*models.Player{
			hostID: {
				UserID:    hostID,
				Pseudo:    hostPseudo,
				Score:     0,
				IsHost:    true,
				IsReady:   true,
				Connected: true,
			},
		},
		Config:    config,
		CreatedAt: time.Now(),
	}

	// Sauvegarder en mémoire
	m.rooms[roomID] = room
	m.codes[code] = roomID

	// Sauvegarder en base de données
	if m.db != nil {
		_, err = m.db.Exec(`
			INSERT INTO rooms (id, code, name, host_id, game_type, status, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			roomID, code, roomName, hostID, string(gameType), string(models.RoomStatusWaiting), room.CreatedAt,
		)
		if err != nil {
			log.Printf("[Rooms] Erreur sauvegarde DB: %v", err)
			// On continue quand même, la salle est en mémoire
		}
	}

	log.Printf("[Rooms] Salle créée: %s (%s) par %s, type: %s", roomName, code, hostPseudo, gameType)
	return room, nil
}

// GetRoom récupère une salle par son ID
func (m *Manager) GetRoom(roomID string) (*models.Room, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

// GetRoomByCode récupère une salle par son code
func (m *Manager) GetRoomByCode(code string) (*models.Room, error) {
	m.mutex.RLock()
	roomID, exists := m.codes[strings.ToUpper(code)]
	m.mutex.RUnlock()

	if !exists {
		return nil, ErrRoomNotFound
	}
	return m.GetRoom(roomID)
}

// JoinRoom fait rejoindre un joueur à une salle
func (m *Manager) JoinRoom(roomID string, userID int64, pseudo string) (*models.Room, error) {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return nil, err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	// Vérifier si la partie n'est pas déjà en cours
	if room.Status == models.RoomStatusPlaying {
		return nil, ErrGameInProgress
	}

	// Vérifier si le joueur est déjà dans la salle
	if _, exists := room.Players[userID]; exists {
		// Reconnecter le joueur
		room.Players[userID].Connected = true
		return room, nil
	}

	// Vérifier si la salle est pleine
	if len(room.Players) >= MaxPlayersPerRoom {
		return nil, ErrRoomFull
	}

	// Ajouter le joueur
	room.Players[userID] = &models.Player{
		UserID:    userID,
		Pseudo:    pseudo,
		Score:     0,
		IsHost:    false,
		IsReady:   false,
		Connected: true,
	}

	log.Printf("[Rooms] %s a rejoint la salle %s", pseudo, room.Name)
	return room, nil
}

// LeaveRoom fait quitter un joueur d'une salle
func (m *Manager) LeaveRoom(roomID string, userID int64) error {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return err
	}

	room.Mutex.Lock()

	player, exists := room.Players[userID]
	if !exists {
		room.Mutex.Unlock()
		return nil
	}

	pseudo := player.Pseudo
	wasHost := player.IsHost

	delete(room.Players, userID)
	log.Printf("[Rooms] %s a quitté la salle %s", pseudo, room.Name)

	// Si plus de joueurs, supprimer la salle
	if len(room.Players) == 0 {
		room.Mutex.Unlock()
		return m.DeleteRoom(roomID)
	}

	// Si l'hôte part, transférer à un autre joueur
	if wasHost {
		for id, p := range room.Players {
			p.IsHost = true
			room.HostID = id
			log.Printf("[Rooms] Nouvel hôte: %s", p.Pseudo)
			break
		}
	}

	room.Mutex.Unlock()
	return nil
}

// SetPlayerReady définit l'état "prêt" d'un joueur
func (m *Manager) SetPlayerReady(roomID string, userID int64, ready bool) error {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	player, exists := room.Players[userID]
	if !exists {
		return ErrRoomNotFound
	}

	player.IsReady = ready
	return nil
}

// UpdateRoomStatus met à jour le statut d'une salle
func (m *Manager) UpdateRoomStatus(roomID string, status models.RoomStatus) error {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	room.Status = status
	room.Mutex.Unlock()

	// Mettre à jour en base
	if m.db != nil {
		_, err = m.db.Exec("UPDATE rooms SET status = ? WHERE id = ?", string(status), roomID)
		if err != nil {
			log.Printf("[Rooms] Erreur mise à jour statut DB: %v", err)
		}
	}

	statusInfo := status.GetStatusInfo()
	log.Printf("[Rooms] Salle %s -> %s", room.Name, statusInfo.Label)
	return nil
}

// GetAllRooms retourne toutes les salles actives
func (m *Manager) GetAllRooms() []*models.Room {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	rooms := make([]*models.Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// GetRoomsByStatus retourne les salles avec un statut particulier
func (m *Manager) GetRoomsByStatus(status models.RoomStatus) []*models.Room {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var rooms []*models.Room
	for _, room := range m.rooms {
		if room.Status == status {
			rooms = append(rooms, room)
		}
	}
	return rooms
}

// DeleteRoom supprime une salle
func (m *Manager) DeleteRoom(roomID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	delete(m.codes, room.Code)
	delete(m.rooms, roomID)

	// Supprimer de la base
	if m.db != nil {
		_, err := m.db.Exec("DELETE FROM rooms WHERE id = ?", roomID)
		if err != nil {
			log.Printf("[Rooms] Erreur suppression DB: %v", err)
		}
	}

	log.Printf("[Rooms] Salle supprimée: %s", room.Name)
	return nil
}

// ResetPlayerScores remet les scores des joueurs à zéro
func (m *Manager) ResetPlayerScores(roomID string) error {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	for _, player := range room.Players {
		player.Score = 0
	}

	return nil
}

// AddPlayerScore ajoute des points au score d'un joueur
func (m *Manager) AddPlayerScore(roomID string, userID int64, points int) error {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	player, exists := room.Players[userID]
	if !exists {
		return ErrRoomNotFound
	}

	player.Score += points
	return nil
}

// GetPlayer récupère un joueur dans une salle
func (m *Manager) GetPlayer(roomID string, userID int64) (*models.Player, error) {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return nil, err
	}

	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	player, exists := room.Players[userID]
	if !exists {
		return nil, ErrRoomNotFound
	}

	return player, nil
}

// IsHost vérifie si un utilisateur est l'hôte d'une salle
func (m *Manager) IsHost(roomID string, userID int64) bool {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return false
	}

	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	return room.HostID == userID
}

// ============================================================================
// FONCTIONS UTILITAIRES
// ============================================================================

// generateRoomID génère un ID de salle unique
func generateRoomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateUniqueCode génère un code unique pour rejoindre une salle
func (m *Manager) generateUniqueCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Sans I, O, 0, 1 pour éviter confusion

	for attempts := 0; attempts < 10; attempts++ {
		bytes := make([]byte, RoomCodeLength)
		if _, err := rand.Read(bytes); err != nil {
			return "", err
		}

		code := make([]byte, RoomCodeLength)
		for i := range code {
			code[i] = charset[bytes[i]%byte(len(charset))]
		}

		codeStr := string(code)
		if _, exists := m.codes[codeStr]; !exists {
			return codeStr, nil
		}
	}

	return "", errors.New("impossible de générer un code unique")
}

// cleanupInactiveRooms nettoie périodiquement les salles inactives
func (m *Manager) cleanupInactiveRooms() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mutex.Lock()
		now := time.Now()
		toDelete := []string{}

		for id, room := range m.rooms {
			// Supprimer les salles terminées depuis plus de 30 minutes
			// ou les salles en attente depuis plus de 2 heures
			if room.Status == models.RoomStatusFinished {
				toDelete = append(toDelete, id)
			} else if room.Status == models.RoomStatusWaiting && now.Sub(room.CreatedAt) > InactiveRoomTimeout {
				toDelete = append(toDelete, id)
			}
		}

		for _, id := range toDelete {
			room := m.rooms[id]
			delete(m.codes, room.Code)
			delete(m.rooms, id)
			log.Printf("[Rooms] Salle inactive supprimée: %s", room.Name)
		}

		m.mutex.Unlock()

		if len(toDelete) > 0 {
			log.Printf("[Rooms] Nettoyage: %d salle(s) supprimée(s)", len(toDelete))
		}
	}
}

// ============================================================================
// MÉTHODES DE JEU
// ============================================================================

// StartGame démarre une partie dans une salle (met à jour le statut)
func (m *Manager) StartGame(roomID string) error {
	return m.UpdateRoomStatus(roomID, models.RoomStatusPlaying)
}

// EndGame termine une partie dans une salle (met à jour le statut)
func (m *Manager) EndGame(roomID string) error {
	return m.UpdateRoomStatus(roomID, models.RoomStatusFinished)
}

// CreateRoomLegacy crée une salle avec l'ancienne signature (compatibilité)
// Le nom de la salle sera le pseudo du joueur par défaut
func (m *Manager) CreateRoomLegacy(hostID int64, hostPseudo string, gameType models.GameType) (*models.Room, error) {
	// Utilise le pseudo comme nom de salle par défaut
	return m.CreateRoom(hostPseudo+"'s Room", hostID, hostPseudo, gameType)
}

// CreateRoomWithName est un alias explicite pour CreateRoom
func (m *Manager) CreateRoomWithName(roomName string, hostID int64, hostPseudo string, gameType models.GameType) (*models.Room, error) {
	return m.CreateRoom(roomName, hostID, hostPseudo, gameType)
}