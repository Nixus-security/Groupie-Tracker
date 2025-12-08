// Package rooms - service.go
// Service pour la persistance des salles en base de données
package rooms

import (
	"database/sql"
	"encoding/json"

	"groupie-tracker/internal/database"
	"groupie-tracker/internal/models"
)

// Service gère la persistance des salles
type Service struct {
	db      *sql.DB
	manager *Manager
}

// NewService crée une nouvelle instance du service
func NewService() *Service {
	return &Service{
		db:      database.GetDB(),
		manager: GetManager(),
	}
}

// SaveGameScores sauvegarde les scores d'une partie
func (s *Service) SaveGameScores(room *models.Room, roundScores map[int64][]int) error {
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
