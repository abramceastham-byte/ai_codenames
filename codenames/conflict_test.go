package codenames

import "testing"

func board(words ...string) []Card {
	cards := make([]Card, 0, len(words))
	for _, w := range words {
		cards = append(cards, Card{Codename: w})
	}
	return cards
}

// TestConflictingBoardWordAllows is the regression the affix test buys us:
// containment is not conflict. Every clue here contains a board word somewhere
// in the string but is not a form of it, and must be playable.
func TestConflictingBoardWordAllows(t *testing.T) {
	tests := []struct {
		boardWord string
		clues     []string
	}{
		{"ice", []string{"nice", "price", "slice", "rice", "juice", "device", "office", "justice", "practice"}},
		{"ear", []string{"bear", "hear", "year", "search", "early"}},
		{"art", []string{"start", "heart", "part", "apart"}},
		{"ring", []string{"bring", "spring", "string", "during"}},
		{"over", []string{"cover", "lover", "discover"}},
		{"one", []string{"only", "money", "bone", "phone"}},
		{"ace", []string{"space", "race", "face", "place"}},
		{"car", []string{"card", "care", "carpet", "carbon"}},
	}

	for _, tc := range tests {
		cards := board(tc.boardWord)
		for _, clue := range tc.clues {
			t.Run(tc.boardWord+"/"+clue, func(t *testing.T) {
				if word, ok := ConflictingBoardWord(clue, cards); ok {
					t.Errorf("ConflictingBoardWord(%q) flagged %q; %q is not a form of %q", clue, word, clue, tc.boardWord)
				}
			})
		}
	}
}

// TestConflictingBoardWordRejects covers the forms that really do give the
// board word away.
func TestConflictingBoardWordRejects(t *testing.T) {
	tests := []struct {
		boardWord string
		clues     []string
	}{
		{"king", []string{"king", "kings", "kingdom", "kingly", "KING", "Kingdom"}},
		{"ice", []string{"ices", "iced", "icy", "icing"}},
		{"run", []string{"running", "runner", "runs"}},
		{"city", []string{"cities", "citys"}},
		{"dance", []string{"danced", "dancing", "dancer", "dances"}},
		{"hope", []string{"hopeless", "hopeful"}},
		{"friend", []string{"friendship", "friendly", "friendless"}},
	}

	for _, tc := range tests {
		cards := board(tc.boardWord)
		for _, clue := range tc.clues {
			t.Run(tc.boardWord+"/"+clue, func(t *testing.T) {
				word, ok := ConflictingBoardWord(clue, cards)
				if !ok {
					t.Fatalf("ConflictingBoardWord(%q) allowed it; %q is a form of %q", clue, clue, tc.boardWord)
				}
				if word != tc.boardWord {
					t.Errorf("ConflictingBoardWord(%q) = %q, want %q", clue, word, tc.boardWord)
				}
			})
		}
	}
}

func TestConflictingBoardWordExactMatchWins(t *testing.T) {
	// "ice" comes first, but "police" is itself on the board and is the word
	// the error should name.
	cards := board("ice", "police", "king")
	word, ok := ConflictingBoardWord("police", cards)
	if !ok {
		t.Fatal("ConflictingBoardWord(\"police\") allowed a board word")
	}
	if word != "police" {
		t.Errorf("ConflictingBoardWord(\"police\") = %q, want \"police\"", word)
	}
}

func TestConflictingBoardWordMultiWordCards(t *testing.T) {
	cards := board("ICE CREAM", "sea_horse", "new-york")

	rejects := []string{
		"icecream", "ICE CREAM", // the joined form
		"icy", "icing", // derived from the "ice" token
		"creamy", "creams", // derived from the "cream" token
		"seahorse", "horses", "seas",
		"newyork", "yorkish",
	}
	for _, clue := range rejects {
		t.Run("reject/"+clue, func(t *testing.T) {
			if _, ok := ConflictingBoardWord(clue, cards); !ok {
				t.Errorf("ConflictingBoardWord(%q) allowed a form of a multi-word card", clue)
			}
		})
	}

	allows := []string{"dream", "screams", "coarse", "renew"}
	for _, clue := range allows {
		t.Run("allow/"+clue, func(t *testing.T) {
			if word, ok := ConflictingBoardWord(clue, cards); ok {
				t.Errorf("ConflictingBoardWord(%q) flagged %q", clue, word)
			}
		})
	}
}

// TestConflictingBoardWordIgnoresRevealed pins the official rule: a covered
// card's word is back in play as a clue.
func TestConflictingBoardWordIgnoresRevealed(t *testing.T) {
	cards := []Card{
		{Codename: "king", Revealed: true},
		{Codename: "ice"},
	}

	for _, clue := range []string{"king", "kings", "kingdom"} {
		t.Run("revealed/"+clue, func(t *testing.T) {
			if word, ok := ConflictingBoardWord(clue, cards); ok {
				t.Errorf("ConflictingBoardWord(%q) flagged revealed card %q; covered words are back in play", clue, word)
			}
		})
	}

	// The unrevealed card on the same board still blocks.
	for _, clue := range []string{"ice", "icy"} {
		t.Run("unrevealed/"+clue, func(t *testing.T) {
			if _, ok := ConflictingBoardWord(clue, cards); !ok {
				t.Errorf("ConflictingBoardWord(%q) allowed a form of an unrevealed card", clue)
			}
		})
	}

	if _, ok := SuspectedCompound("kingfisher", cards); ok {
		t.Error("SuspectedCompound flagged a revealed card")
	}
	if _, ok := SuspectedCompound("iceberg", cards); !ok {
		t.Error("SuspectedCompound missed an unrevealed card")
	}
}

func TestConflictingBoardWordEdgeCases(t *testing.T) {
	cards := board("king", "ax")

	if _, ok := ConflictingBoardWord("", cards); ok {
		t.Error("empty clue reported a conflict")
	}
	if _, ok := ConflictingBoardWord("   ", cards); ok {
		t.Error("whitespace-only clue reported a conflict")
	}
	if _, ok := ConflictingBoardWord("king", nil); ok {
		t.Error("empty board reported a conflict")
	}
	// Below minRootLen, only exact matches count.
	if _, ok := ConflictingBoardWord("axe", cards); ok {
		t.Error("two-letter card treated as a root")
	}
	if _, ok := ConflictingBoardWord("ax", cards); !ok {
		t.Error("exact match on a two-letter card was allowed")
	}
}

func TestSuspectedCompound(t *testing.T) {
	cards := board("air", "king", "ice")

	suspect := []string{"airport", "kingfisher", "iceberg", "airplane"}
	for _, clue := range suspect {
		t.Run("suspect/"+clue, func(t *testing.T) {
			if _, ok := SuspectedCompound(clue, cards); !ok {
				t.Errorf("SuspectedCompound(%q) = false, want true", clue)
			}
		})
	}

	// Derived forms are conflicts, not compounds — they're already hard
	// rejected, so flagging them again would double-count in the log.
	for _, clue := range []string{"kings", "icy", "kingdom"} {
		t.Run("not-compound/"+clue, func(t *testing.T) {
			if word, ok := SuspectedCompound(clue, cards); ok {
				t.Errorf("SuspectedCompound(%q) flagged %q, want false (it's a derived form)", clue, word)
			}
		})
	}

	// Clues with no board word inside them at all.
	for _, clue := range []string{"monarch", "frozen", "wind"} {
		t.Run("clean/"+clue, func(t *testing.T) {
			if word, ok := SuspectedCompound(clue, cards); ok {
				t.Errorf("SuspectedCompound(%q) flagged %q, want false", clue, word)
			}
		})
	}

	// The known false-positive class, asserted so the behavior is documented
	// rather than discovered later: these are logged, never blocked.
	for _, clue := range []string{"justice", "office"} {
		t.Run("known-false-positive/"+clue, func(t *testing.T) {
			if _, ok := SuspectedCompound(clue, cards); !ok {
				t.Errorf("SuspectedCompound(%q) = false; expected it to flag (and only log)", clue)
			}
			if _, ok := ConflictingBoardWord(clue, cards); ok {
				t.Errorf("ConflictingBoardWord(%q) rejected a clue that should only be logged", clue)
			}
		})
	}
}
