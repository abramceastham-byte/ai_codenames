package botproto

// Move log lines, as written by cmd/match and read by cmd/replay. A log is
// JSONL: one LogMatchStart, then LogEvents (per turn: a clue, its guesses,
// any forfeit, and a turn_end), then one LogResult.

// Log line types, in the "type" field of every line.
const (
	LogMatchStartType = "match_start"
	LogClue           = "clue"
	LogGuess          = "guess"
	LogForfeit        = "forfeit"
	LogTurnEnd        = "turn_end"
	LogResultType     = "result"
)

// LogMatchStart is the first line of a move log.
type LogMatchStart struct {
	Type          string `json:"type"`
	Time          string `json:"time"`
	Seed          int64  `json:"seed"`
	StartingTeam  Team   `json:"starting_team"`
	MoveTimeoutMS int64  `json:"move_timeout_ms"`
	MaxTurns      int    `json:"max_turns"`
	Red           Player `json:"red"`
	Blue          Player `json:"blue"`
	// Board is the starting board, with every card's type.
	Board []Card `json:"board"`
}

// LogEvent is one clue, guess, forfeit, or turn_end line.
type LogEvent struct {
	Type     string   `json:"type"`
	Turn     int      `json:"turn"`
	Team     Team     `json:"team"`
	Role     Role     `json:"role,omitempty"`
	Bot      string   `json:"bot,omitempty"`
	Word     string   `json:"word,omitempty"`
	Count    int      `json:"count,omitempty"`
	Revealed CardType `json:"revealed,omitempty"`
	End      TurnEnd  `json:"end,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}

// LogResult is the last line of a move log. Winner is empty for a draw.
type LogResult struct {
	Type       string `json:"type"`
	Winner     Team   `json:"winner"`
	WinnerName string `json:"winner_name"`
	Reason     string `json:"reason"`
	Tiebreak   string `json:"tiebreak,omitempty"`
	Turns      int    `json:"turns"`
	Forfeits   int    `json:"forfeits"`
	FinalBoard []Card `json:"final_board"`
}
