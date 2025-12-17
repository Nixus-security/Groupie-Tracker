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
	MaxPlayersPerRoom   = 10
	RoomCodeLength      = 6
	InactiveRoomTimeout = 2 * time.Hour
)

type Manager struct {
	rooms map[string]*models.Room
	codes map[string]string
	mutex sync.RWMutex
	db    *sql.DB
}

var (
	managerInstance *Manager
	managerOnce     sync.Once
)

func GetManager() *Manager {
	managerOnce.Do(func() {
		managerInstance = &Manager{
			rooms: make(map[string]*models.Room),
			codes: make(map[string]string),
			db:    database.GetDB(),
		}
		go managerInstance.cleanupInactiveRooms()
	})
	return managerInstance
}

func (m *Manager) CreateRoom(roomName string, hostID int64, hostPseudo string, gameType models.GameType) (*models.Room, error) {
	roomName = strings.TrimSpace(roomName)
	if len(roomName) < 3 || len(roomName) > 50 {
		return nil, ErrInvalidRoomName
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	roomID, err := generateRoomID()
	if err != nil {
		return nil, err
	}

	code, err := m.generateUniqueCode()
	if err != nil {
		return nil, err
	}

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
		Name:     roomName,
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

	m.rooms[roomID] = room
	m.codes[code] = roomID

	if m.db != nil {
		_, err = m.db.Exec(`
			INSERT INTO rooms (id, code, name, host_id, game_type, status, created_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			roomID, code, roomName, hostID, string(gameType), string(models.RoomStatusWaiting), room.CreatedAt,
		)
		if err != nil {
			log.Printf("[Rooms] Erreur sauvegarde DB: %v", err)
		}
	}

	log.Printf("[Rooms] Salle créée: %s (%s) par %s, type: %s", roomName, code, hostPseudo, gameType)
	return room, nil
}

func (m *Manager) GetRoom(roomID string) (*models.Room, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

func (m *Manager) GetRoomByCode(code string) (*models.Room, error) {
	m.mutex.RLock()
	roomID, exists := m.codes[strings.ToUpper(code)]
	m.mutex.RUnlock()

	if !exists {
		return nil, ErrRoomNotFound
	}
	return m.GetRoom(roomID)
}

func (m *Manager) JoinRoom(roomID string, userID int64, pseudo string) (*models.Room, error) {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return nil, err
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if room.Status == models.RoomStatusPlaying {
		return nil, ErrGameInProgress
	}

	if _, exists := room.Players[userID]; exists {
		room.Players[userID].Connected = true
		return room, nil
	}

	if len(room.Players) >= MaxPlayersPerRoom {
		return nil, ErrRoomFull
	}

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

	if len(room.Players) == 0 {
		room.Mutex.Unlock()
		return m.DeleteRoom(roomID)
	}

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

func (m *Manager) UpdateRoomStatus(roomID string, status models.RoomStatus) error {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return err
	}

	room.Mutex.Lock()
	room.Status = status
	room.Mutex.Unlock()

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

func (m *Manager) GetAllRooms() []*models.Room {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	rooms := make([]*models.Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

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

func (m *Manager) DeleteRoom(roomID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	room, exists := m.rooms[roomID]
	if !exists {
		return ErrRoomNotFound
	}

	delete(m.codes, room.Code)
	delete(m.rooms, roomID)

	if m.db != nil {
		_, err := m.db.Exec("DELETE FROM rooms WHERE id = ?", roomID)
		if err != nil {
			log.Printf("[Rooms] Erreur suppression DB: %v", err)
		}
	}

	log.Printf("[Rooms] Salle supprimée: %s", room.Name)
	return nil
}

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

func (m *Manager) IsHost(roomID string, userID int64) bool {
	room, err := m.GetRoom(roomID)
	if err != nil {
		return false
	}

	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	return room.HostID == userID
}

func generateRoomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (m *Manager) generateUniqueCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

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

func (m *Manager) cleanupInactiveRooms() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mutex.Lock()
		now := time.Now()
		toDelete := []string{}

		for id, room := range m.rooms {
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

func (m *Manager) StartGame(roomID string) error {
	return m.UpdateRoomStatus(roomID, models.RoomStatusPlaying)
}

func (m *Manager) EndGame(roomID string) error {
	return m.UpdateRoomStatus(roomID, models.RoomStatusFinished)
}

func (m *Manager) CreateRoomLegacy(hostID int64, hostPseudo string, gameType models.GameType) (*models.Room, error) {
	return m.CreateRoom(hostPseudo+"'s Room", hostID, hostPseudo, gameType)
}

func (m *Manager) CreateRoomWithName(roomName string, hostID int64, hostPseudo string, gameType models.GameType) (*models.Room, error) {
	return m.CreateRoom(roomName, hostID, hostPseudo, gameType)
}