package models

import (
	"encoding/json"
	"time"
)

type Room struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	CreatorID int64     `json:"creator_id"`
	GameType  string    `json:"game_type"`
	Status    string    `json:"status"`
	Config    string    `json:"config"`
	CreatedAt time.Time `json:"created_at"`
	Players   []Player  `json:"players,omitempty"`
}

type Player struct {
	ID     int64  `json:"id"`
	Pseudo string `json:"pseudo"`
}

type BlindTestConfig struct {
	Playlist     string `json:"playlist"`
	TimePerRound int    `json:"time_per_round"`
	TotalRounds  int    `json:"total_rounds"`
	MinPlayers   int    `json:"min_players"`
}

type PetitBacConfig struct {
	TimePerRound int     `json:"time_per_round"`
	TotalRounds  int     `json:"total_rounds"`
	Categories   []int64 `json:"categories"`
	MinPlayers   int     `json:"min_players"`
}

func DefaultBlindTestConfig() BlindTestConfig {
	return BlindTestConfig{
		Playlist:     "Pop",
		TimePerRound: 37,
		TotalRounds:  10,
		MinPlayers:   2,
	}
}

func DefaultPetitBacConfig() PetitBacConfig {
	return PetitBacConfig{
		TimePerRound: 60,
		TotalRounds:  9,
		Categories:   []int64{1, 2, 3, 4, 5},
		MinPlayers:   2,
	}
}

func IsRoomReady(r Room) bool {
	if r.Status != "waiting" {
		return false
	}

	if len(r.Players) == 0 {
		return false
	}

	switch r.GameType {
	case "blindtest":
		var cfg BlindTestConfig
		if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
			return false
		}
		return len(r.Players) >= cfg.MinPlayers

	case "petitbac":
		var cfg PetitBacConfig
		if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
			return false
		}
		return len(r.Players) >= cfg.MinPlayers
	}

	return false
}

func (r *Room) GetBlindTestConfig() (*BlindTestConfig, error) {
	var cfg BlindTestConfig
	if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Room) GetPetitBacConfig() (*PetitBacConfig, error) {
	var cfg PetitBacConfig
	if err := json.Unmarshal([]byte(r.Config), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Room) SetConfig(cfg interface{}) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	r.Config = string(data)
	return nil
}
