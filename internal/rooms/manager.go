// Package rooms gère les salles de jeu
package rooms

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"groupie-tracker/internal/models"
)

var (
	ErrRoomNotFound     = errors.New("salle non trouvée")
	ErrRoomFull         = errors.New("salle pleine")
	ErrAlreadyInRoom    = errors.New("vous êtes déjà dans cette salle")
	ErrNotInRoom        = errors.New("vous n'êtes pas dans cette salle")
	ErrNotHost          = errors.New("vous n'êtes pas l'hôte de cette salle")
	ErrGameInProgress   = errors.New("une partie est déjà en cours")
	ErrInvalidGameType  = errors.New("type de jeu invalide")
)

const (
	// MaxPlayersPerRoom nombre maximum de joueurs par salle
	MaxPlayersPerRoom = 8
	
	// RoomCodeLength longueur du code de salle
	RoomCodeLength = 6
)

// Manager gère toutes les salles en mémoire
type Manager struct {
	rooms map[string]*models.Room // Code -> Room
	mutex sync.RWMutex
}

// instance singleton du manager
var (
	managerInstance *Manager
	managerOnce     sync.Once
)

// GetManager retourne l'instance singleton du manager
func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			rooms: make(map[string]*models.Room),
		}
	})
	return managerInstance
}

// CreateRoom crée une nouvelle salle
func (m *Manager) CreateRoom(hostID int64, hostPseudo string, name string, gameType models.GameType) (*models.Room, error) {
	// Valider le type de jeu
	if gameType != models.GameTypeBlindTest && gameType != models.GameTypePetitBac {
		return nil, ErrInvalidGameType
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Générer un code unique
	code := m.generateUniqueCode()

	// Générer un ID unique
	roomID, err := generateID()
	if err != nil {
		return nil, err
	}

	// Configuration par défaut selon le type de jeu
	config := models.GameConfig{}
	if gameType == models.GameTypeBlindTest {
		config.Playlist = "Pop" // Par défaut
		config.TimePerRound = models.BlindTestDefaultTime
	} else {
		config.Categories = models.DefaultPetitBacCategories
		config.NbRounds = models.NbrsManche
		config.UsedLetters = []string{}
	}

	room := &models.Room{
		ID:        roomID,
		Code:      code,
		Name:      name,
		HostID:    hostID,
		GameType:  gameType,
		Status:    models.RoomStatusWaiting,
		Players:   make(map[int64]*models.Player),
		Config:    config,
		CreatedAt: time.Now(),
	}

	// Ajouter l'hôte comme premier joueur
	room.Players[hostID] = &models.Player{
		UserID:    hostID,
		Pseudo:    hostPseudo,
		Score:     0,
		IsHost:    true,
		IsReady:   false,
		Connected: true,
	}

	m.rooms[code] = room

	return room, nil
}

// GetRoom récupère une salle par son code
func (m *Manager) GetRoom(code string) (*models.Room, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	room, exists := m.rooms[code]
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

// GetRoomByID récupère une salle par son ID
func (m *Manager) GetRoomByID(id string) (*models.Room, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, room := range m.rooms {
		if room.ID == id {
			return room, nil
		}
	}
	return nil, ErrRoomNotFound
}

// JoinRoom permet à un joueur de rejoindre une salle
func (m *Manager) JoinRoom(code string, userID int64, pseudo string) (*models.Room, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	room, exists := m.rooms[code]
	if !exists {
		return nil, ErrRoomNotFound
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	// Vérifier si la partie n'est pas déjà en cours
	if room.Status == models.RoomStatusPlaying {
		return nil, ErrGameInProgress
	}

	// Vérifier si le joueur n'est pas déjà dans la salle
	if _, exists := room.Players[userID]; exists {
		// Reconnecter le joueur
		room.Players[userID].Connected = true
		return room, nil
	}

	// Vérifier si la salle n'est pas pleine
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

	return room, nil
}

// LeaveRoom permet à un joueur de quitter une salle
func (m *Manager) LeaveRoom(code string, userID int64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	room, exists := m.rooms[code]
	if !exists {
		return ErrRoomNotFound
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if _, exists := room.Players[userID]; !exists {
		return ErrNotInRoom
	}

	// Si c'est l'hôte qui part et qu'il y a d'autres joueurs
	if room.HostID == userID && len(room.Players) > 1 {
		// Transférer le rôle d'hôte
		delete(room.Players, userID)
		for id, player := range room.Players {
			player.IsHost = true
			room.HostID = id
			break
		}
	} else {
		delete(room.Players, userID)
	}

	// Supprimer la salle si elle est vide
	if len(room.Players) == 0 {
		delete(m.rooms, code)
	}

	return nil
}

// SetPlayerReady définit le statut "prêt" d'un joueur
func (m *Manager) SetPlayerReady(code string, userID int64, ready bool) error {
	room, err := m.GetRoom(code)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	player, exists := room.Players[userID]
	if !exists {
		return ErrNotInRoom
	}

	player.IsReady = ready
	return nil
}

// UpdateRoomConfig met à jour la configuration d'une salle
func (m *Manager) UpdateRoomConfig(code string, userID int64, config models.GameConfig) error {
	room, err := m.GetRoom(code)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	// Seul l'hôte peut modifier la config
	if room.HostID != userID {
		return ErrNotHost
	}

	if room.Status != models.RoomStatusWaiting {
		return ErrGameInProgress
	}

	room.Config = config
	return nil
}

// StartGame démarre la partie
func (m *Manager) StartGame(code string, userID int64) error {
	room, err := m.GetRoom(code)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if room.HostID != userID {
		return ErrNotHost
	}

	if !models.IsRoomReady(room) {
		return errors.New("tous les joueurs ne sont pas prêts")
	}

	room.Status = models.RoomStatusPlaying
	return nil
}

// EndGame termine la partie
func (m *Manager) EndGame(code string) error {
	room, err := m.GetRoom(code)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	room.Status = models.RoomStatusFinished
	return nil
}

// ResetRoom remet la salle en attente
func (m *Manager) ResetRoom(code string, userID int64) error {
	room, err := m.GetRoom(code)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if room.HostID != userID {
		return ErrNotHost
	}

	room.Status = models.RoomStatusWaiting
	
	// Réinitialiser les scores et états des joueurs
	for _, player := range room.Players {
		player.Score = 0
		player.IsReady = false
	}

	// Réinitialiser les lettres utilisées pour le Petit Bac
	room.Config.UsedLetters = []string{}

	return nil
}

// GetPlayerCount retourne le nombre de joueurs dans une salle
func (m *Manager) GetPlayerCount(code string) int {
	room, err := m.GetRoom(code)
	if err != nil {
		return 0
	}

	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	return len(room.Players)
}

// GetAllRooms retourne toutes les salles (pour debug/admin)
func (m *Manager) GetAllRooms() []*models.Room {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	rooms := make([]*models.Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

// DisconnectPlayer marque un joueur comme déconnecté
func (m *Manager) DisconnectPlayer(code string, userID int64) {
	room, err := m.GetRoom(code)
	if err != nil {
		return
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if player, exists := room.Players[userID]; exists {
		player.Connected = false
	}
}

// UpdatePlayerScore met à jour le score d'un joueur
func (m *Manager) UpdatePlayerScore(code string, userID int64, points int) {
	room, err := m.GetRoom(code)
	if err != nil {
		return
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if player, exists := room.Players[userID]; exists {
		player.Score += points
	}
}

// ============================================================================
// FONCTIONS UTILITAIRES
// ============================================================================

// generateUniqueCode génère un code de salle unique
func (m *Manager) generateUniqueCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Sans I, O, 0, 1 pour éviter confusion
	for {
		code := make([]byte, RoomCodeLength)
		rand.Read(code)
		for i := range code {
			code[i] = charset[int(code[i])%len(charset)]
		}
		codeStr := string(code)
		
		// Vérifier que le code n'existe pas déjà
		if _, exists := m.rooms[codeStr]; !exists {
			return codeStr
		}
	}
}

// generateID génère un ID unique pour une salle
func generateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
