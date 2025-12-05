package models

import "time"

type Game struct {
	ID           int64     `json:"id"`
	RoomID       int64     `json:"room_id"`
	GameType     string    `json:"game_type"`
	CurrentRound int       `json:"current_round"`
	TotalRounds  int       `json:"total_rounds"`
	Status       string    `json:"status"`
	UsedLetters  string    `json:"used_letters"`
	Config       string    `json:"config"`
	CreatedAt    time.Time `json:"created_at"`
}

type GameState struct {
	Game          *Game        `json:"game"`
	CurrentTrack  *SpotifyTrack `json:"current_track,omitempty"`
	CurrentLetter string        `json:"current_letter,omitempty"`
	TimeRemaining int           `json:"time_remaining"`
	Players       []Player      `json:"players"`
}

type SpotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	PreviewURL string `json:"preview_url"`
	ImageURL   string `json:"image_url"`
}

type BlindTestAnswer struct {
	UserID       int64  `json:"user_id"`
	Answer       string `json:"answer"`
	IsCorrect    bool   `json:"is_correct"`
	ResponseTime int64  `json:"response_time"`
	Points       int    `json:"points"`
}

type RoundResult struct {
	RoundNumber int                 `json:"round_number"`
	Answers     []BlindTestAnswer   `json:"answers,omitempty"`
	Track       *SpotifyTrack       `json:"track,omitempty"`
	Letter      string              `json:"letter,omitempty"`
}

func (g *Game) IsLastRound() bool {
	return g.CurrentRound >= g.TotalRounds
}

func (g *Game) NextRound() bool {
	if g.IsLastRound() {
		return false
	}
	g.CurrentRound++
	return true
}

func (g *Game) AddUsedLetter(letter string) {
	g.UsedLetters += letter
}

func (g *Game) IsLetterUsed(letter string) bool {
	for _, l := range g.UsedLetters {
		if string(l) == letter {
			return true
		}
	}
	return false
}

func (g *Game) GetAvailableLetters() []string {
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var available []string

	for _, letter := range alphabet {
		if !g.IsLetterUsed(string(letter)) {
			available = append(available, string(letter))
		}
	}

	return available
}
