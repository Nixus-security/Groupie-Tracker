package models

const nbrs_manche = 9

type Category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

type PetitBacAnswer struct {
	ID            int64  `json:"id"`
	GameID        int64  `json:"game_id"`
	UserID        int64  `json:"user_id"`
	RoundNumber   int    `json:"round_number"`
	CategoryID    int64  `json:"category_id"`
	Letter        string `json:"letter"`
	Answer        string `json:"answer"`
	VotesValid    int    `json:"votes_valid"`
	VotesInvalid  int    `json:"votes_invalid"`
	IsValidated   bool   `json:"is_validated"`
	PointsAwarded int    `json:"points_awarded"`
	Pseudo        string `json:"pseudo,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
}

type PetitBacSubmission struct {
	UserID  int64            `json:"user_id"`
	Answers map[int64]string `json:"answers"`
}

type PetitBacVote struct {
	AnswerID int64 `json:"answer_id"`
	IsValid  bool  `json:"is_valid"`
}

type RoundAnswers struct {
	Letter   string                      `json:"letter"`
	Round    int                         `json:"round"`
	Answers  map[int64][]PetitBacAnswer  `json:"answers"`
}

func GetNbrsManche() int {
	return nbrs_manche
}

func DefaultCategories() []Category {
	return []Category{
		{ID: 1, Name: "Artiste", IsDefault: true},
		{ID: 2, Name: "Album", IsDefault: true},
		{ID: 3, Name: "Groupe", IsDefault: true},
		{ID: 4, Name: "Instrument", IsDefault: true},
		{ID: 5, Name: "Featuring", IsDefault: true},
	}
}

func CalculatePetitBacPoints(answer PetitBacAnswer, allAnswers []PetitBacAnswer) int {
	if answer.Answer == "" || !answer.IsValidated {
		return 0
	}

	count := 0
	normalized := normalizeAnswer(answer.Answer)

	for _, a := range allAnswers {
		if a.CategoryID == answer.CategoryID &&
			a.RoundNumber == answer.RoundNumber &&
			a.IsValidated &&
			normalizeAnswer(a.Answer) == normalized {
			count++
		}
	}

	if count > 1 {
		return 1
	}
	return 2
}

func normalizeAnswer(s string) string {
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else if r >= 'a' && r <= 'z' {
			result += string(r)
		}
	}
	return result
}

func IsAnswerValidated(votesValid, votesInvalid, totalPlayers int) bool {
	totalVotes := votesValid + votesInvalid
	if totalVotes == 0 {
		return false
	}

	threshold := float64(totalVotes) * 2.0 / 3.0
	return float64(votesValid) >= threshold
}
