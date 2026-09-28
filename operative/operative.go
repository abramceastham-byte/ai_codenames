// Package operative holds deterministic (non-LLM, non-prompt) decision logic
// for the operative role — currently just the bonus-guess gate. Kept
// separate from llm/w2v so this stays pure, dependency-free, and trivially
// unit-testable: no board, no embeddings, no network calls.
package operative

// AllowBonus decides whether the operative should take the optional bonus
// guess (the (N+1)th guess after a clue for N words), given the current
// score gap and how risky the best remaining candidates look.
//
//   - oursLeft:  words our team still has left to find (after this clue).
//   - theirsLeft: words the opposing team still has left to find.
//   - margin:    score gap between our team's best remaining candidate and
//     the best non-team candidate (opponent, bystander, or assassin), from
//     the existing similarity ranking for the clue just given. A small or
//     negative margin means a non-team word is competitive with — or beats —
//     our best guess, i.e. the bonus guess is a real gamble.
//   - assassinInTop3: true if the assassin word is among the 3 highest-
//     scoring candidates overall (team and non-team combined) for this clue.
//     This is a signal margin alone can't carry: margin only compares against
//     whichever non-team word scores highest, so an assassin sitting at rank
//     3 behind a higher-scoring bystander wouldn't move margin at all, but
//     still means one wrong step lands on it. The literal 3-argument
//     "allowBonus(oursLeft, theirsLeft int, margin float64)" spec can't
//     express this hard block on its own — it needed a 4th input, so it's
//     included explicitly rather than trying to encode it as an adjustment
//     to margin.
//
// Callers must compute margin and assassinInTop3 from a fully-visible board
// (i.e. the spymaster's view). An operative's own view has every unrevealed
// card's Agent zeroed to UnknownAgent (see codenames.Revealed), so it cannot
// legitimately compute either value itself — doing so would mean using
// hidden information a human operative structurally cannot see.
func AllowBonus(oursLeft, theirsLeft int, margin float64, assassinInTop3 bool) bool {
	// Hard block first: no score situation justifies a guess that's likely
	// the assassin, regardless of how far behind we are.
	if assassinInTop3 {
		return false
	}

	deficit := oursLeft - theirsLeft
	if deficit < 2 {
		// Not meaningfully behind — no need to gamble on a bonus guess.
		return false
	}

	if theirsLeft <= 2 {
		// They're about to win: a much smaller edge is worth the gamble.
		return margin > 0.05
	}

	// Behind, but the opponent isn't imminently winning — only take a bonus
	// guess that's an obvious pick, not a coin flip.
	return margin > 0.20
}
