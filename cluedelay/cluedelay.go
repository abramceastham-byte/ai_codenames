// Package cluedelay puts a floor on how long a spymaster's clue phase lasts.
//
// A clue is withheld from the operatives until the phase has been running for
// at least the configured delay, no matter how quickly the spymaster submitted
// it. The point is that response latency shouldn't be a tell: an AI spymaster
// that answers in two seconds and a human who agonizes for fifty both hand
// their clue over at the same moment. A spymaster who takes longer than the
// delay isn't held at all — the clock has already run out by the time they
// submit.
//
// The hold can be turned off per game while a game is in progress, for running
// held and unheld games side by side. Turning it off cuts short a clue that's
// mid-hold rather than letting it sit out the rest of its wait.
//
// The tracker only tracks time and the single in-flight clue slot; committing
// and broadcasting the clue at release time is the caller's job.
package cluedelay

import (
	"errors"
	"sync"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

// DefaultDelay is the minimum time from the start of a team's clue phase to
// the moment their clue reaches the operatives.
const DefaultDelay = 60 * time.Second

// ErrPending reports that a clue for this game is already waiting out its
// delay. Only one clue can be in flight per game, since giving one ends the
// spymaster's turn.
var ErrPending = errors.New("cluedelay: a clue is already waiting to be released")

// Tracker records when each game's clue phase began and whether that game is
// holding clues.
type Tracker struct {
	delay time.Duration
	now   func() time.Time

	mu    sync.Mutex
	games map[codenames.GameID]*gameState
}

type gameState struct {
	startedAt time.Time
	// pending is true between Hold and Done — a clue is waiting to go out.
	pending bool
	// cut is closed to release the pending clue ahead of its time. Nil once
	// closed, so it can't be closed twice.
	cut chan struct{}
	// off records that this game has had its hold switched off. It survives
	// Start, so the setting sticks for the rest of the game.
	off bool
}

// New returns a Tracker that holds clues for delay past the start of the clue
// phase. A delay of zero or less disables holding entirely, and no game can
// switch it back on — there'd be no duration to hold for.
func New(delay time.Duration) *Tracker {
	return &Tracker{
		delay: delay,
		now:   time.Now,
		games: make(map[codenames.GameID]*gameState),
	}
}

// Delay returns the configured minimum clue phase length. Zero means this
// server never holds clues.
func (t *Tracker) Delay() time.Duration {
	return t.delay
}

// Start marks the beginning of a clue phase for gID: the game starting, or a
// turn ending and handing the board to the other team's spymaster. It must be
// called before the message that tells the spymaster it's their turn goes out,
// so an AI that replies instantly is still measured from the right instant.
func (t *Tracker) Start(gID codenames.GameID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state(gID).startedAt = t.now()
}

// Hold claims the game's clue slot and reports when the clue may be released,
// along with a channel that's closed if the hold is cut short. The returned
// time is in the past if the phase has already outlasted the delay or the game
// isn't holding clues, in which case the caller should release right away.
//
// A phase we have no record of — the process restarted mid-game — is treated
// as starting now, which holds the clue for the full delay. Guessing late is
// the safe direction: it costs the spymaster a wait, where guessing early
// would leak how fast they answered.
func (t *Tracker) Hold(gID codenames.GameID) (time.Time, <-chan struct{}, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s := t.state(gID)
	if s.startedAt.IsZero() {
		s.startedAt = t.now()
	}
	if s.pending {
		return time.Time{}, nil, ErrPending
	}
	s.pending = true
	s.cut = make(chan struct{})

	return s.startedAt.Add(t.gameDelay(s)), s.cut, nil
}

// Done frees the clue slot claimed by Hold, whether the clue was released on
// time, cut short, or abandoned.
func (t *Tracker) Done(gID codenames.GameID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if s, ok := t.games[gID]; ok {
		s.pending = false
		s.cut = nil
	}
}

// Enabled reports whether gID is currently holding clues.
func (t *Tracker) Enabled(gID codenames.GameID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.delay <= 0 {
		return false
	}
	s, ok := t.games[gID]
	return !ok || !s.off
}

// SetEnabled turns the hold on or off for a single game, and reports the state
// it ended up in — which is false regardless if this server has no delay
// configured. Switching it off releases a clue that's mid-hold immediately,
// rather than leaving the spymaster's clue stuck behind a wait nobody wants
// anymore.
func (t *Tracker) SetEnabled(gID codenames.GameID, on bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	s := t.state(gID)
	s.off = !on
	if !on && s.pending && s.cut != nil {
		close(s.cut)
		s.cut = nil
	}

	return on && t.delay > 0
}

// Clear drops all state for gID, for when the game is over.
func (t *Tracker) Clear(gID codenames.GameID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.games, gID)
}

// state returns gID's state, creating it if this is the first we've heard of
// the game. Callers must hold mu.
func (t *Tracker) state(gID codenames.GameID) *gameState {
	s, ok := t.games[gID]
	if !ok {
		s = &gameState{}
		t.games[gID] = s
	}
	return s
}

// gameDelay is how long s should hold a clue. Callers must hold mu.
func (t *Tracker) gameDelay(s *gameState) time.Duration {
	if s.off {
		return 0
	}
	return t.delay
}
