package models

import "sort"

var scoreboardActualPointInGame = make(map[int64]map[int64]int)

type Score struct {
	ID          int64  `json:"id"`
	GameID      int64  `json:"game_id"`
	UserID      int64  `json:"user_id"`
	Points      int    `json:"points"`
	RoundNumber int    `json:"round_number"`
	Pseudo      string `json:"pseudo"`
}

type ScoreboardEntry struct {
	Rank       int    `json:"rank"`
	UserID     int64  `json:"user_id"`
	Pseudo     string `json:"pseudo"`
	TotalScore int    `json:"total_score"`
}

type Scoreboard struct {
	GameID   int64             `json:"game_id"`
	GameType string            `json:"game_type"`
	Status   string            `json:"status"`
	Entries  []ScoreboardEntry `json:"entries"`
}

func InitGameScoreboard(gameID int64) {
	scoreboardActualPointInGame[gameID] = make(map[int64]int)
}

func UpdatePlayerScore(gameID, userID int64, points int) {
	if scoreboardActualPointInGame[gameID] == nil {
		scoreboardActualPointInGame[gameID] = make(map[int64]int)
	}
	scoreboardActualPointInGame[gameID][userID] += points
}

func SetPlayerScore(gameID, userID int64, points int) {
	if scoreboardActualPointInGame[gameID] == nil {
		scoreboardActualPointInGame[gameID] = make(map[int64]int)
	}
	scoreboardActualPointInGame[gameID][userID] = points
}

func GetPlayerScore(gameID, userID int64) int {
	if scoreboardActualPointInGame[gameID] == nil {
		return 0
	}
	return scoreboardActualPointInGame[gameID][userID]
}

func GetGameScores(gameID int64) map[int64]int {
	if scoreboardActualPointInGame[gameID] == nil {
		return make(map[int64]int)
	}
	scores := make(map[int64]int)
	for k, v := range scoreboardActualPointInGame[gameID] {
		scores[k] = v
	}
	return scores
}

func ClearGameScoreboard(gameID int64) {
	delete(scoreboardActualPointInGame, gameID)
}

func BuildScoreboard(gameID int64, players []Player, gameType, status string) *Scoreboard {
	scores := GetGameScores(gameID)

	entries := make([]ScoreboardEntry, 0, len(players))
	for _, p := range players {
		entries = append(entries, ScoreboardEntry{
			UserID:     p.ID,
			Pseudo:     p.Pseudo,
			TotalScore: scores[p.ID],
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TotalScore > entries[j].TotalScore
	})

	for i := range entries {
		entries[i].Rank = i + 1
	}

	return &Scoreboard{
		GameID:   gameID,
		GameType: gameType,
		Status:   status,
		Entries:  entries,
	}
}
