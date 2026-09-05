package llm

import (
	"fmt"
	"strings"

	"github.com/bcspragu/Codenames/codenames"
)

// summaryWidth is the inner width of a summary block, chosen to stay readable
// in a default 80-column terminal once log's timestamp prefix is accounted for.
const summaryWidth = 68

// summaryBlock accumulates the lines of one boxed terminal summary. Callers
// build it up with the add* helpers and finish with String, which is what gets
// handed to a single log.Printf — one call, so a block never interleaves with
// another goroutine's output the way successive Printf lines would.
type summaryBlock struct {
	title string
	lines []string
}

func newSummaryBlock(title string) *summaryBlock {
	return &summaryBlock{title: title}
}

// addField writes a "LABEL  value" row, aligning values into a common column.
func (s *summaryBlock) addField(label, value string) {
	s.lines = append(s.lines, fmt.Sprintf("%-9s %s", label, value))
}

// addBlank writes a spacer row, used to separate sections.
func (s *summaryBlock) addBlank() {
	s.lines = append(s.lines, "")
}

// addRaw writes a pre-formatted line verbatim, truncating rather than
// wrapping. Use it for lines whose internal spacing is load-bearing (column
// alignment), since addWrapped re-flows through strings.Fields and would
// collapse that spacing to single spaces.
func (s *summaryBlock) addRaw(indent, text string) {
	s.lines = append(s.lines, indent+truncate(text, summaryWidth-len([]rune(indent))))
}

// addSection writes a section heading.
func (s *summaryBlock) addSection(name string) {
	s.lines = append(s.lines, name)
}

// addWrapped writes text indented under a section heading, wrapping at the
// block width so a long rationale stays inside the box instead of blowing
// past it.
func (s *summaryBlock) addWrapped(indent, text string) {
	for _, line := range wrapText(text, summaryWidth-len(indent)) {
		s.lines = append(s.lines, indent+line)
	}
}

// String renders the accumulated lines as a box. It leads with a newline so
// the block starts on its own line rather than trailing log's timestamp
// prefix, which would push the top border out of alignment with the rest.
func (s *summaryBlock) String() string {
	var b strings.Builder
	b.WriteString("\n┌─ " + s.title + " ")
	if pad := summaryWidth - len([]rune(s.title)) - 3; pad > 0 {
		b.WriteString(strings.Repeat("─", pad))
	}
	b.WriteString("\n")
	for _, line := range s.lines {
		b.WriteString("│ " + line + "\n")
	}
	b.WriteString("└" + strings.Repeat("─", summaryWidth))
	return b.String()
}

// wrapText greedily wraps text to width columns, never splitting a word. It
// counts runes rather than bytes so a rationale containing non-ASCII (curly
// quotes and em dashes both show up in model output) wraps at the width it
// actually occupies on screen.
func wrapText(text string, width int) []string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil
	}
	if width < 8 {
		width = 8
	}

	var (
		lines []string
		cur   string
	)
	for _, f := range fields {
		switch {
		case cur == "":
			cur = f
		case len([]rune(cur))+1+len([]rune(f)) <= width:
			cur += " " + f
		default:
			lines = append(lines, cur)
			cur = f
		}
	}
	return append(lines, cur)
}

// truncate shortens s to at most n runes, marking the cut with an ellipsis so
// a trimmed value never reads as though the model actually said that much.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// spymasterSummary renders the terminal block for a clue the spymaster just
// committed to: the clue itself, each target with the model's own short
// reason, and its assassin check.
func spymasterSummary(teamName string, p *clueParse) string {
	b := newSummaryBlock("SPYMASTER · " + strings.ToUpper(teamName))
	b.addField("CLUE", fmt.Sprintf("%s (%d)", strings.ToUpper(p.Clue.Word), p.Clue.Count))
	b.addBlank()

	b.addSection("TARGETS")
	for _, t := range p.Targets {
		why := t.Why
		if why == "" {
			why = "(no reason given)"
		}
		// Word on its own line, reason indented under it — a two-column
		// layout would either truncate the reason or wrap it into the word
		// column once a target runs long.
		b.addRaw("  ▸ ", strings.ToUpper(t.Word))
		b.addWrapped("      ", why)
	}

	b.addBlank()
	b.addSection("ASSASSIN")
	b.addWrapped("  ", truncate(p.Assassin, 240))
	return b.String()
}

// operativeSummary renders the terminal block for a guess decision: the clue
// as the operative received it, each candidate with its confidence and why,
// and the decision the thresholds produced from them.
func operativeSummary(teamName string, c *codenames.Clue, res *GuessResult) string {
	title := "OPERATIVE"
	if teamName != "" {
		title += " · " + strings.ToUpper(teamName)
	}
	b := newSummaryBlock(title)
	b.addField("CLUE", fmt.Sprintf("%s (%d)", strings.ToUpper(c.Word), c.Count))
	if res.GuessesThisTurn >= c.Count {
		b.addField("GUESS #", fmt.Sprintf("bonus (all %d already guessed)", c.Count))
	} else {
		b.addField("GUESS #", fmt.Sprintf("%d of %d", res.GuessesThisTurn+1, c.Count))
	}
	b.addBlank()

	b.addSection("CANDIDATES")
	for i, cand := range res.Candidates {
		marker := "    "
		if i == 0 {
			// The top candidate is the only one the threshold is ever
			// applied to, so it's worth picking out of the list.
			marker = "  ▸ "
		}
		conf := fmt.Sprintf("%.2f", cand.Confidence)
		if cand.Confidence < cand.RawConfidence {
			conf = fmt.Sprintf("%.2f (capped from %.2f)", cand.Confidence, cand.RawConfidence)
		}
		b.addRaw(marker, fmt.Sprintf("%-12s %-24s %s", strings.ToUpper(cand.Word), conf, cand.LinkType))
		b.addWrapped("      ", cand.Reasoning)
	}

	b.addBlank()
	riskiest := res.RiskiestBoardWord
	if riskiest == "" {
		riskiest = "(none named)"
	}
	if res.TopCandidateIsRiskiest {
		riskiest += "  ← top candidate is this word"
	}
	b.addField("RISKIEST", riskiest)

	// Say which threshold applied and why, so a pass never looks arbitrary:
	// the same confidence passes or guesses depending on which of the three
	// rules below was in force.
	var rule string
	switch {
	case res.MustGuess:
		rule = "mandated first guess, no threshold"
	case res.GuessesThisTurn >= res.ClueNumber:
		rule = fmt.Sprintf("bonus guess, threshold %.2f", res.ThresholdApplied)
	default:
		rule = fmt.Sprintf("threshold %.2f", res.ThresholdApplied)
	}

	decision := "PASS — ends the turn"
	if res.Guess != "" && res.Guess != codenames.PassGuess {
		decision = "GUESS " + strings.ToUpper(res.Guess)
	}
	b.addField("DECISION", decision+"  ("+rule+")")
	return b.String()
}
