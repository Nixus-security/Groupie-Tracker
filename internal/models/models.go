// Package models contient toutes les structures de données de l'application
package models

import (
	"sync"
	"time"
)

// ============================================================================
// CONSTANTES DU JEU
// ============================================================================

const (
	// NbrsManche définit le nombre de manches pour le Petit Bac
	NbrsManche = 9

	// BlindTestDefaultTime temps par manche en secondes pour le Blind Test
	BlindTestDefaultTime = 37
)

// ============================================================================
// UTILISATEUR
// ============================================================================

// User représente un utilisateur enregistré
type User struct {
	ID           int64     `json:"id"`
	Pseudo       string    `json:"pseudo"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Ne jamais exposer en JSON
	CreatedAt    time.Time `json:"created_at"`
}

// Session représente une session utilisateur active
type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ============================================================================
// SALLES DE JEU
// ============================================================================

// GameType représente le type de jeu
type GameType string

const (
	GameTypeBlindTest GameType = "blindtest"
	GameTypePetitBac  GameType = "petitbac"
)

// RoomStatus représente l'état d'une salle
type RoomStatus string

const (
	RoomStatusWaiting  RoomStatus = "waiting"  // En attente de joueurs
	RoomStatusPlaying  RoomStatus = "playing"  // Partie en cours
	RoomStatusFinished RoomStatus = "finished" // Partie terminée
)

// Room représente une salle de jeu
type Room struct {
	ID        string              `json:"id"`
	Code      string              `json:"code"`      // Code pour rejoindre
	Name      string              `json:"name"`      // Nom de la salle
	HostID    int64               `json:"host_id"`   // Créateur de la salle
	GameType  GameType            `json:"game_type"` // Type de jeu
	Status    RoomStatus          `json:"status"`
	Players   map[int64]*Player   `json:"players"` // Joueurs dans la salle
	Config    GameConfig          `json:"config"`  // Configuration du jeu
	CreatedAt time.Time           `json:"created_at"`
	Mutex     sync.RWMutex        `json:"-"` // Pour accès concurrent
}

// Player représente un joueur dans une salle
type Player struct {
	UserID    int64  `json:"user_id"`
	Pseudo    string `json:"pseudo"`
	Score     int    `json:"score"`
	IsHost    bool   `json:"is_host"`
	IsReady   bool   `json:"is_ready"`
	Connected bool   `json:"connected"`
}

// GameConfig configuration générale d'une partie
type GameConfig struct {
	// Blind Test
	Playlist     string `json:"playlist,omitempty"`      // Rock, Rap, Pop
	TimePerRound int    `json:"time_per_round,omitempty"` // Temps par manche

	// Petit Bac
	Categories    []string `json:"categories,omitempty"`     // Catégories actives
	NbRounds      int      `json:"nb_rounds,omitempty"`      // Nombre de manches
	UsedLetters   []string `json:"used_letters,omitempty"`   // Lettres déjà utilisées
}

// IsRoomReady vérifie si une salle est prête à démarrer
func IsRoomReady(r *Room) bool {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	// Minimum 2 joueurs
	if len(r.Players) < 2 {
		return false
	}

	// Tous les joueurs doivent être prêts et connectés
	for _, player := range r.Players {
		if !player.IsReady || !player.Connected {
			return false
		}
	}

	return true
}

// ============================================================================
// BLIND TEST
// ============================================================================

// SpotifyTrack représente une piste Spotify
type SpotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	PreviewURL string `json:"preview_url"` // URL de prévisualisation 30s
	ImageURL   string `json:"image_url"`
}

// ============================================================================
// PETIT BAC
// ============================================================================

// DefaultPetitBacCategories catégories par défaut du Petit Bac musical
var DefaultPetitBacCategories = []string{
	"artiste",
	"album",
	"groupe",
	"instrument",
	"featuring",
}

// PetitBacCategory catégorie personnalisée
type PetitBacCategory struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// WEBSOCKET MESSAGES
// ============================================================================

// WSMessageType types de messages WebSocket
type WSMessageType string

const (
	// Messages généraux
	WSTypeError       WSMessageType = "error"
	WSTypePing        WSMessageType = "ping"
	WSTypePong        WSMessageType = "pong"
	
	// Messages de salle
	WSTypeJoinRoom    WSMessageType = "join_room"
	WSTypeLeaveRoom   WSMessageType = "leave_room"
	WSTypePlayerJoined WSMessageType = "player_joined"
	WSTypePlayerLeft  WSMessageType = "player_left"
	WSTypePlayerReady WSMessageType = "player_ready"
	WSTypeRoomUpdate  WSMessageType = "room_update"
	WSTypeStartGame   WSMessageType = "start_game"
	
	// Messages Blind Test
	WSTypeBTNewRound    WSMessageType = "bt_new_round"
	WSTypeBTAnswer      WSMessageType = "bt_answer"
	WSTypeBTResult      WSMessageType = "bt_result"
	WSTypeBTScores      WSMessageType = "bt_scores"
	WSTypeBTGameEnd     WSMessageType = "bt_game_end"
	
	// Messages Petit Bac
	WSTypePBNewRound    WSMessageType = "pb_new_round"
	WSTypePBAnswer      WSMessageType = "pb_answer"
	WSTypePBVote        WSMessageType = "pb_vote"
	WSTypePBVoteResult  WSMessageType = "pb_vote_result"
	WSTypePBScores      WSMessageType = "pb_scores"
	WSTypePBGameEnd     WSMessageType = "pb_game_end"
	WSTypePBStopRound   WSMessageType = "pb_stop_round"
)

// WSMessage message WebSocket générique
type WSMessage struct {
	Type    WSMessageType `json:"type"`
	Payload interface{}   `json:"payload,omitempty"`
	Error   string        `json:"error,omitempty"`
}