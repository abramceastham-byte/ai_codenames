package msgs

import (
	"encoding/json"

	"github.com/bcspragu/Codenames/codenames"
)

type Player struct {
	PlayerID codenames.PlayerID `json:"player_id"`
	Name     string             `json:"name"`
	Team     codenames.Team     `json:"team"`
	Role     codenames.Role     `json:"role"`
}

type jsonRoleAssigned RoleAssigned
type RoleAssigned struct {
	Players []*Player `json:"players"`
}

func (ra *RoleAssigned) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonRoleAssigned
		Action string `json:"action"`
	}{jsonRoleAssigned(*ra), "ROLE_ASSIGNED"})
}

type jsonGameStart GameStart
type GameStart struct {
	Game    *codenames.Game `json:"game"`
	Players []*Player       `json:"players"`
}

func (gs *GameStart) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonGameStart
		Action string `json:"action"`
	}{jsonGameStart(*gs), "GAME_START"})
}

type jsonClueGiven ClueGiven
type ClueGiven struct {
	Clue *codenames.Clue `json:"clue"`
	Team codenames.Team  `json:"team"`
	Game *codenames.Game `json:"game"`
}

func (cg *ClueGiven) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonClueGiven
		Action string `json:"action"`
	}{jsonClueGiven(*cg), "CLUE_GIVEN"})
}

type jsonPlayerVote PlayerVote
type PlayerVote struct {
	PlayerID  codenames.PlayerID `json:"player_id"`
	Guess     string             `json:"guess"`
	Confirmed bool               `json:"confirmed"`
}

func (pv *PlayerVote) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonPlayerVote
		Action string `json:"action"`
	}{jsonPlayerVote(*pv), "PLAYER_VOTE"})
}

type jsonGuessGiven GuessGiven
type GuessGiven struct {
	Guess           string          `json:"guess"`
	Team            codenames.Team  `json:"team"`
	CanKeepGuessing bool            `json:"can_keep_guessing"`
	RevealedCard    *codenames.Card `json:"card"`
	Game            *codenames.Game `json:"game"`
}

func (gg *GuessGiven) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonGuessGiven
		Action string `json:"action"`
	}{jsonGuessGiven(*gg), "GUESS_GIVEN"})
}

type jsonGameEnd GameEnd
type GameEnd struct {
	WinningTeam codenames.Team  `json:"winning_team"`
	Game        *codenames.Game `json:"game"`
}

func (ge *GameEnd) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonGameEnd
		Action string `json:"action"`
	}{jsonGameEnd(*ge), "GAME_END"})
}

type jsonBothCluesRevealed BothCluesRevealed
type BothCluesRevealed struct {
	RedClue  *codenames.Clue `json:"red_clue"`
	BlueClue *codenames.Clue `json:"blue_clue"`
	Game     *codenames.Game `json:"game"`
}

func (b *BothCluesRevealed) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonBothCluesRevealed
		Action string `json:"action"`
	}{jsonBothCluesRevealed(*b), "BOTH_CLUES_REVEALED"})
}

type jsonTuringVoteCast TuringVoteCast
type TuringVoteCast struct {
	PlayerID        codenames.PlayerID `json:"player_id"`
	SuspectedAITeam codenames.Team     `json:"suspected_ai_team"`
}

func (t *TuringVoteCast) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonTuringVoteCast
		Action string `json:"action"`
	}{jsonTuringVoteCast(*t), "TURING_VOTE_CAST"})
}

type jsonTuringResult TuringResult
type TuringResult struct {
	ActualAITeam  codenames.Team  `json:"actual_ai_team"`
	VotesRedIsAI  int             `json:"votes_red_is_ai"`
	VotesBlueIsAI int             `json:"votes_blue_is_ai"`
	Game          *codenames.Game `json:"game"`
}

func (t *TuringResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		jsonTuringResult
		Action string `json:"action"`
	}{jsonTuringResult(*t), "TURING_RESULT"})
}
