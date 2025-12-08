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

// BlindTestGame représente une partie de Blind Test en cours
type BlindTestGame struct {
	RoomID       string           `json:"room_id"`
	CurrentRound int              `json:"current_round"`
	TotalRounds  int              `json:"total_rounds"`
	CurrentTrack *SpotifyTrack    `json:"current_track,omitempty"`
	StartTime    time.Time        `json:"start_time"`
	Scores       map[int64]int    `json:"scores"` // UserID -> Score
	Answers      map[int64]string `json:"answers"` // Réponses de la manche
	Status       string           `json:"status"`
	Mutex        sync.RWMutex     `json:"-"`
}

// SpotifyTrack représente une piste Spotify
type SpotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	PreviewURL string `json:"preview_url"` // URL de prévisualisation 30s
	ImageURL   string `json:"image_url"`
}

// BlindTestAnswer réponse d'un joueur au Blind Test
type BlindTestAnswer struct {
	UserID    int64     `json:"user_id"`
	Answer    string    `json:"answer"`
	Timestamp time.Time `json:"timestamp"`
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

// PetitBacGame représente une partie de Petit Bac en cours
type PetitBacGame struct {
	RoomID       string                      `json:"room_id"`
	CurrentRound int                         `json:"current_round"`
	TotalRounds  int                         `json:"total_rounds"`
	CurrentLetter string                     `json:"current_letter"`
	UsedLetters  []string                    `json:"used_letters"`
	Categories   []string                    `json:"categories"`
	Answers      map[int64]*PetitBacAnswer   `json:"answers"` // UserID -> Réponses
	Votes        map[string]map[int64]bool   `json:"votes"`   // Category -> UserID -> Vote
	Scores       map[int64]int               `json:"scores"`
	Status       string                      `json:"status"`
	StartTime    time.Time                   `json:"start_time"`
	Mutex        sync.RWMutex                `json:"-"`
}

// PetitBacAnswer réponses d'un joueur pour une manche
type PetitBacAnswer struct {
	UserID   int64             `json:"user_id"`
	Answers  map[string]string `json:"answers"` // Category -> Réponse
	Submitted bool             `json:"submitted"`
}

// PetitBacVote vote pour valider une réponse
type PetitBacVote struct {
	VoterID    int64  `json:"voter_id"`
	TargetID   int64  `json:"target_id"`
	Category   string `json:"category"`
	IsValid    bool   `json:"is_valid"`
}

// PetitBacCategory catégorie personnalisée
type PetitBacCategory struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// SCOREBOARD
// ============================================================================

// Scoreboard tableau des scores
type Scoreboard struct {
	RoomID                    string         `json:"room_id"`
	GameType                  GameType       `json:"game_type"`
	ScoreboardActualPointInGame map[int64]int `json:"scoreboard_actual_point_in_game"`
	FinalScores               map[int64]int  `json:"final_scores"`
}

// ScoreEntry entrée du classement
type ScoreEntry struct {
	UserID int64  `json:"user_id"`
	Pseudo string `json:"pseudo"`
	Score  int    `json:"score"`
	Rank   int    `json:"rank"`
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

// WSJoinRoomPayload payload pour rejoindre une salle
type WSJoinRoomPayload struct {
	RoomCode string `json:"room_code"`
}

// WSPlayerReadyPayload payload pour signaler qu'on est prêt
type WSPlayerReadyPayload struct {
	Ready bool `json:"ready"`
}

// WSBlindTestAnswerPayload payload pour une réponse Blind Test
type WSBlindTestAnswerPayload struct {
	Answer string `json:"answer"`
}

// WSPetitBacAnswerPayload payload pour les réponses Petit Bac
type WSPetitBacAnswerPayload struct {
	Answers map[string]string `json:"answers"`
}

// WSPetitBacVotePayload payload pour un vote Petit Bac
type WSPetitBacVotePayload struct {
	TargetUserID int64  `json:"target_user_id"`
	Category     string `json:"category"`
	IsValid      bool   `json:"is_valid"`
}
