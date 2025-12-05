package spotify
package websocket

type MessageType string

const (
	TypeUserJoined      MessageType = "user_joined"
	TypeUserLeft        MessageType = "user_left"
	TypeChat            MessageType = "chat"
	TypePlayerReady     MessageType = "player_ready"
	TypeGameStarting    MessageType = "game_starting"
	TypeRoundStart      MessageType = "round_start"
	TypeRoundEnd        MessageType = "round_end"
	TypeAnswerSubmitted MessageType = "answer_submitted"
	TypeVotingStart     MessageType = "voting_start"
	TypeVoteUpdate      MessageType = "vote_update"
	TypeShowAnswers     MessageType = "show_answers"
	TypeGameEnd         MessageType = "game_end"
	TypeError           MessageType = "error"
	TypePong            MessageType = "pong"
)

type UserJoinedMessage struct {
	Type   MessageType `json:"type"`
	UserID int64       `json:"user_id"`
	Pseudo string      `json:"pseudo"`
	Count  int         `json:"count"`
}

type UserLeftMessage struct {
	Type   MessageType `json:"type"`
	UserID int64       `json:"user_id"`
	Pseudo string      `json:"pseudo"`
	Count  int         `json:"count"`
}

type ChatMessage struct {
	Type    MessageType `json:"type"`
	UserID  int64       `json:"user_id"`
	Pseudo  string      `json:"pseudo"`
	Message string      `json:"message"`
}

type RoundStartMessage struct {
	Type        MessageType `json:"type"`
	Round       int         `json:"round"`
	TotalRounds int         `json:"total_rounds"`
	Time        int         `json:"time"`
	PreviewURL  string      `json:"preview_url,omitempty"`
	Letter      string      `json:"letter,omitempty"`
	Categories  []int64     `json:"categories,omitempty"`
}

type RoundEndMessage struct {
	Type   MessageType    `json:"type"`
	Round  int            `json:"round"`
	Scores map[int64]int  `json:"scores"`
	Track  *TrackInfo     `json:"track,omitempty"`
}

type TrackInfo struct {
	Name   string `json:"name"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Image  string `json:"image"`
}

type AnswerSubmittedMessage struct {
	Type         MessageType `json:"type"`
	UserID       int64       `json:"user_id"`
	Pseudo       string      `json:"pseudo"`
	IsCorrect    bool        `json:"is_correct,omitempty"`
	Points       int         `json:"points,omitempty"`
	ResponseTime int64       `json:"response_time,omitempty"`
}

type VotingStartMessage struct {
	Type  MessageType `json:"type"`
	Round int         `json:"round"`
}

type VoteUpdateMessage struct {
	Type         MessageType `json:"type"`
	AnswerID     int64       `json:"answer_id"`
	VotesValid   int         `json:"votes_valid"`
	VotesInvalid int         `json:"votes_invalid"`
	IsValidated  bool        `json:"is_validated"`
}

type ShowAnswersMessage struct {
	Type    MessageType   `json:"type"`
	Answers []AnswerInfo  `json:"answers"`
}

type AnswerInfo struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"user_id"`
	Pseudo       string `json:"pseudo"`
	CategoryID   int64  `json:"category_id"`
	CategoryName string `json:"category_name"`
	Answer       string `json:"answer"`
	VotesValid   int    `json:"votes_valid"`
	VotesInvalid int    `json:"votes_invalid"`
	IsValidated  bool   `json:"is_validated"`
}

type GameEndMessage struct {
	Type       MessageType      `json:"type"`
	Scoreboard []ScoreboardItem `json:"scoreboard"`
}

type ScoreboardItem struct {
	Rank       int    `json:"rank"`
	UserID     int64  `json:"user_id"`
	Pseudo     string `json:"pseudo"`
	TotalScore int    `json:"total_score"`
}

type ErrorMessage struct {
	Type    MessageType `json:"type"`
	Message string      `json:"message"`
}

func NewError(message string) ErrorMessage {
	return ErrorMessage{
		Type:    TypeError,
		Message: message,
	}
}