package cluedelay

import (
	"errors"
	"testing"
	"time"

	"github.com/bcspragu/Codenames/codenames"
)

const gID = codenames.GameID("game_0")

// fakeClock lets tests advance time without sleeping.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestTracker(delay time.Duration) (*Tracker, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)}
	tr := New(delay)
	tr.now = clk.now
	return tr, clk
}

func TestHoldWaitsOutRemainderOfPhase(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start(gID)
	clk.advance(2 * time.Second) // A suspiciously quick spymaster.

	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}

	if got, want := releaseAt.Sub(clk.now()), 58*time.Second; got != want {
		t.Errorf("clue held for %v, want %v", got, want)
	}
}

func TestHoldReleasesImmediatelyAfterDelayElapsed(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start(gID)
	clk.advance(90 * time.Second) // A spymaster who thought it over.

	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}

	if !releaseAt.Before(clk.now()) {
		t.Errorf("release time %v is not in the past (now %v), so a slow clue would be held", releaseAt, clk.now())
	}
}

func TestHoldWithoutStartHoldsFullDelay(t *testing.T) {
	// A phase we have no record of (the process restarted mid-game) must hold
	// for the full delay rather than releasing early.
	tr, clk := newTestTracker(60 * time.Second)

	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}

	if got, want := releaseAt.Sub(clk.now()), 60*time.Second; got != want {
		t.Errorf("clue held for %v, want %v", got, want)
	}
}

func TestHoldRejectsSecondClueWhilePending(t *testing.T) {
	tr, _ := newTestTracker(60 * time.Second)

	tr.Start(gID)
	if _, _, err := tr.Hold(gID); err != nil {
		t.Fatalf("first Hold() = %v, want no error", err)
	}

	if _, _, err := tr.Hold(gID); !errors.Is(err, ErrPending) {
		t.Errorf("second Hold() = %v, want ErrPending", err)
	}

	// Once the first clue is out of the way, the slot is free again.
	tr.Done(gID)
	if _, _, err := tr.Hold(gID); err != nil {
		t.Errorf("Hold() after Done() = %v, want no error", err)
	}
}

func TestStartResetsTheClock(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start(gID)
	clk.advance(30 * time.Second)
	if _, _, err := tr.Hold(gID); err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}
	tr.Done(gID)

	// The next team's phase starts fresh, so their clue gets the full delay.
	tr.Start(gID)
	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}
	if got, want := releaseAt.Sub(clk.now()), 60*time.Second; got != want {
		t.Errorf("clue held for %v, want %v", got, want)
	}
}

func TestHoldsAreTrackedPerGame(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start("game_0")
	clk.advance(45 * time.Second)
	tr.Start("game_1")

	if _, _, err := tr.Hold("game_0"); err != nil {
		t.Fatalf("Hold(game_0) = %v, want no error", err)
	}
	releaseAt, _, err := tr.Hold("game_1")
	if err != nil {
		t.Fatalf("Hold(game_1) = %v, want no error", err)
	}
	if got, want := releaseAt.Sub(clk.now()), 60*time.Second; got != want {
		t.Errorf("game_1's clue held for %v, want %v — a pending clue in another game leaked in", got, want)
	}
}

func TestDisabledGameDoesNotHold(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start(gID)
	if on := tr.SetEnabled(gID, false); on {
		t.Error("SetEnabled(false) reported the hold still on")
	}
	if tr.Enabled(gID) {
		t.Error("Enabled() is true after switching the hold off")
	}

	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}
	if releaseAt.After(clk.now()) {
		t.Errorf("release time %v is in the future for a game with the hold off", releaseAt)
	}
}

func TestSwitchingOffCutsAPendingHoldShort(t *testing.T) {
	tr, _ := newTestTracker(60 * time.Second)

	tr.Start(gID)
	_, cut, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}

	select {
	case <-cut:
		t.Fatal("hold was cut short before anything switched it off")
	default:
	}

	tr.SetEnabled(gID, false)
	select {
	case <-cut:
		// The waiting clue is free to go out.
	default:
		t.Error("switching the hold off did not cut the pending hold short")
	}

	// Switching it off twice must not close the channel twice.
	tr.SetEnabled(gID, false)
	tr.Done(gID)
}

func TestSettingSurvivesTheNextTurn(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start(gID)
	tr.SetEnabled(gID, false)
	tr.Done(gID)

	// A new clue phase must not quietly switch the hold back on.
	tr.Start(gID)
	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}
	if releaseAt.After(clk.now()) {
		t.Errorf("release time %v is in the future — the setting was lost across turns", releaseAt)
	}
	if tr.Enabled(gID) {
		t.Error("Enabled() is true after a new turn began")
	}
}

func TestHoldCanBeSwitchedBackOn(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start(gID)
	tr.SetEnabled(gID, false)
	if on := tr.SetEnabled(gID, true); !on {
		t.Error("SetEnabled(true) reported the hold still off")
	}

	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}
	if got, want := releaseAt.Sub(clk.now()), 60*time.Second; got != want {
		t.Errorf("clue held for %v, want %v", got, want)
	}
}

func TestSettingsAreTrackedPerGame(t *testing.T) {
	tr, clk := newTestTracker(60 * time.Second)

	tr.Start("game_0")
	tr.Start("game_1")
	tr.SetEnabled("game_0", false)

	if tr.Enabled("game_0") {
		t.Error("game_0 still holding after being switched off")
	}
	if !tr.Enabled("game_1") {
		t.Error("switching game_0 off also switched off game_1")
	}
	releaseAt, _, err := tr.Hold("game_1")
	if err != nil {
		t.Fatalf("Hold(game_1) = %v, want no error", err)
	}
	if got, want := releaseAt.Sub(clk.now()), 60*time.Second; got != want {
		t.Errorf("game_1's clue held for %v, want %v", got, want)
	}
}

func TestZeroDelayCannotBeSwitchedOn(t *testing.T) {
	// A server with no delay configured has no duration to hold for, so the
	// per-game switch can't invent one.
	tr, _ := newTestTracker(0)

	if on := tr.SetEnabled(gID, true); on {
		t.Error("SetEnabled(true) reported the hold on despite a zero delay")
	}
	if tr.Enabled(gID) {
		t.Error("Enabled() is true despite a zero delay")
	}
}

func TestZeroDelayReleasesImmediately(t *testing.T) {
	tr, clk := newTestTracker(0)

	tr.Start(gID)
	releaseAt, _, err := tr.Hold(gID)
	if err != nil {
		t.Fatalf("Hold() = %v, want no error", err)
	}
	if releaseAt.After(clk.now()) {
		t.Errorf("release time %v is in the future with a zero delay", releaseAt)
	}
}
