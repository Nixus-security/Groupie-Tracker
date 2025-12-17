package models

import (
	"sync"
	"time"
)

const (
	NbrsManche           = 9
	BlindTestDefaultTime = 37
)

type User struct {
	ID           int64     `json:"id"`
	Pseudo       string    `json:"pseudo"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type GameType string

const (
	GameTypeBlindTest GameType = "blindtest"
	GameTypePetitBac  GameType = "petitbac"
)

type RoomStatus string

const (
	RoomStatusWaiting  RoomStatus = "waiting"
	RoomStatusPlaying  RoomStatus = "playing"
	RoomStatusFinished RoomStatus = "finished"
)

type RoomStatusInfo struct {
	Label string
	Icon  string
	Color string
}

func (s RoomStatus) GetStatusInfo() RoomStatusInfo {
	switch s {
	case RoomStatusWaiting:
		return RoomStatusInfo{
			Label: "En attente",
			Icon:  "icon-hourglass",
			Color: "status-waiting",
		}
	case RoomStatusPlaying:
		return RoomStatusInfo{
			Label: "En cours",
			Icon:  "icon-play",
			Color: "status-playing",
		}
	case RoomStatusFinished:
		return RoomStatusInfo{
			Label: "Terminée",
			Icon:  "icon-check",
			Color: "status-finished",
		}
	default:
		return RoomStatusInfo{
			Label: "Inconnu",
			Icon:  "icon-question",
			Color: "status-unknown",
		}
	}
}

func (s RoomStatus) String() string {
	return s.GetStatusInfo().Label
}

type Room struct {
	ID        string            `json:"id"`
	Code      string            `json:"code"`
	Name      string            `json:"name"`
	HostID    int64             `json:"host_id"`
	GameType  GameType          `json:"game_type"`
	Status    RoomStatus        `json:"status"`
	Players   map[int64]*Player `json:"players"`
	Config    GameConfig        `json:"config"`
	CreatedAt time.Time         `json:"created_at"`
	Mutex     sync.RWMutex      `json:"-"`
}

func (r *Room) PlayerCount() int {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()
	return len(r.Players)
}

type Player struct {
	UserID    int64  `json:"user_id"`
	Pseudo    string `json:"pseudo"`
	Score     int    `json:"score"`
	IsHost    bool   `json:"is_host"`
	IsReady   bool   `json:"is_ready"`
	Connected bool   `json:"connected"`
}

type GameConfig struct {
	Playlist     string   `json:"playlist,omitempty"`
	TimePerRound int      `json:"time_per_round,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	NbRounds     int      `json:"nb_rounds,omitempty"`
	UsedLetters  []string `json:"used_letters,omitempty"`
}

func IsRoomReady(r *Room) bool {
	r.Mutex.RLock()
	defer r.Mutex.RUnlock()

	if len(r.Players) < 1 {
		return false
	}

	for _, player := range r.Players {
		if !player.IsReady || !player.Connected {
			return false
		}
	}

	return true
}

type SpotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	PreviewURL string `json:"preview_url"`
	ImageURL   string `json:"image_url"`
}

var DefaultPetitBacCategories = []string{
	"artiste",
	"album",
	"groupe",
	"instrument",
	"featuring",
}

type PetitBacCategory struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type WSMessageType string

const (
	WSTypeError WSMessageType = "error"
	WSTypePing  WSMessageType = "ping"
	WSTypePong  WSMessageType = "pong"

	WSTypeJoinRoom     WSMessageType = "join_room"
	WSTypeLeaveRoom    WSMessageType = "leave_room"
	WSTypePlayerJoined WSMessageType = "player_joined"
	WSTypePlayerLeft   WSMessageType = "player_left"
	WSTypePlayerReady  WSMessageType = "player_ready"
	WSTypeRoomUpdate   WSMessageType = "room_update"
	WSTypeStartGame    WSMessageType = "start_game"

	WSTypeBTPreload   WSMessageType = "bt_preload"
	WSTypeBTNewRound  WSMessageType = "bt_new_round"
	WSTypeBTAnswer    WSMessageType = "bt_answer"
	WSTypeBTResult    WSMessageType = "bt_result"
	WSTypeBTReveal    WSMessageType = "bt_reveal"
	WSTypeBTScores    WSMessageType = "bt_scores"
	WSTypeBTGameEnd   WSMessageType = "bt_game_end"
	WSTypeTimeUpdate  WSMessageType = "time_update"
	WSTypePlayerFound WSMessageType = "player_found"

	WSTypePBNewRound      WSMessageType = "pb_new_round"
	WSTypePBAnswer        WSMessageType = "pb_answer"
	WSTypePBSubmitAnswers WSMessageType = "submit_answers"
	WSTypePBVote          WSMessageType = "pb_vote"
	WSTypePBSubmitVotes   WSMessageType = "submit_votes"
	WSTypePBVoteResult    WSMessageType = "pb_vote_result"
	WSTypePBScores        WSMessageType = "pb_scores"
	WSTypePBGameEnd       WSMessageType = "pb_game_end"
	WSTypePBStopRound     WSMessageType = "stop_round"
)

type WSMessage struct {
	Type    WSMessageType `json:"type"`
	Payload interface{}   `json:"payload,omitempty"`
	Error   string        `json:"error,omitempty"`
}