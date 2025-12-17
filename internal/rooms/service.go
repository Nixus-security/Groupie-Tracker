package rooms

import (
	"database/sql"
	"encoding/json"

	"groupie-tracker/internal/database"
	"groupie-tracker/internal/models"
)

type PersistenceService struct {
	db      *sql.DB
	manager *Manager
}

func NewPersistenceService() *PersistenceService {
	return &PersistenceService{
		db:      database.GetDB(),
		manager: GetManager(),
	}
}

func (s *PersistenceService) SaveRoom(room *models.Room) error {
	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	configJSON, err := json.Marshal(room.Config)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO rooms (id, code, name, host_id, game_type, status, config, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, room.ID, room.Code, room.Name, room.HostID, room.GameType, room.Status, string(configJSON), room.CreatedAt)

	return err
}

func (s *PersistenceService) SaveRoomPlayers(room *models.Room) error {
	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	_, err := s.db.Exec("DELETE FROM room_players WHERE room_id = ?", room.ID)
	if err != nil {
		return err
	}

	for _, player := range room.Players {
		_, err := s.db.Exec(`
			INSERT INTO room_players (room_id, user_id, score, is_host)
			VALUES (?, ?, ?, ?)
		`, room.ID, player.UserID, player.Score, player.IsHost)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *PersistenceService) SaveGameScores(room *models.Room, roundScores map[int64][]int) error {
	room.Mutex.RLock()
	defer room.Mutex.RUnlock()

	for userID, player := range room.Players {
		roundScoresJSON, _ := json.Marshal(roundScores[userID])
		
		_, err := s.db.Exec(`
			INSERT INTO game_scores (room_id, user_id, game_type, score, round_scores)
			VALUES (?, ?, ?, ?, ?)
		`, room.ID, userID, room.GameType, player.Score, string(roundScoresJSON))
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *PersistenceService) GetUserGameHistory(userID int64, limit int) ([]GameHistoryEntry, error) {
	query := `
		SELECT gs.room_id, r.name, gs.game_type, gs.score, gs.round_scores, gs.created_at
		FROM game_scores gs
		JOIN rooms r ON gs.room_id = r.id
		WHERE gs.user_id = ?
		ORDER BY gs.created_at DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []GameHistoryEntry
	for rows.Next() {
		var entry GameHistoryEntry
		var roundScoresJSON string
		
		err := rows.Scan(&entry.RoomID, &entry.RoomName, &entry.GameType, &entry.Score, &roundScoresJSON, &entry.PlayedAt)
		if err != nil {
			return nil, err
		}
		
		json.Unmarshal([]byte(roundScoresJSON), &entry.RoundScores)
		history = append(history, entry)
	}

	return history, nil
}

type GameHistoryEntry struct {
	RoomID      string          `json:"room_id"`
	RoomName    string          `json:"room_name"`
	GameType    models.GameType `json:"game_type"`
	Score       int             `json:"score"`
	RoundScores []int           `json:"round_scores"`
	PlayedAt    string          `json:"played_at"`
}

func (s *PersistenceService) GetLeaderboard(gameType models.GameType, limit int) ([]LeaderboardEntry, error) {
	query := `
		SELECT u.id, u.pseudo, SUM(gs.score) as total_score, COUNT(gs.id) as games_played
		FROM game_scores gs
		JOIN users u ON gs.user_id = u.id
		WHERE gs.game_type = ?
		GROUP BY u.id
		ORDER BY total_score DESC
		LIMIT ?
	`

	rows, err := s.db.Query(query, gameType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaderboard []LeaderboardEntry
	rank := 1
	for rows.Next() {
		var entry LeaderboardEntry
		err := rows.Scan(&entry.UserID, &entry.Pseudo, &entry.TotalScore, &entry.GamesPlayed)
		if err != nil {
			return nil, err
		}
		entry.Rank = rank
		rank++
		leaderboard = append(leaderboard, entry)
	}

	return leaderboard, nil
}

type LeaderboardEntry struct {
	Rank        int    `json:"rank"`
	UserID      int64  `json:"user_id"`
	Pseudo      string `json:"pseudo"`
	TotalScore  int    `json:"total_score"`
	GamesPlayed int    `json:"games_played"`
}

func (s *PersistenceService) CleanOldRooms() error {
	_, err := s.db.Exec(`
		DELETE FROM rooms 
		WHERE status = 'finished' 
		AND created_at < datetime('now', '-1 day')
	`)
	return err
}