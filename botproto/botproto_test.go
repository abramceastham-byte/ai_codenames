package botproto

import "testing"

func TestValidateClue(t *testing.T) {
	board := []Card{
		{Word: "snowman", Type: CardRed},
		{Word: "air", Type: CardBlue, Revealed: true},
		{Word: "Africa", Type: CardAssassin},
	}
	tests := []struct {
		clue    string
		count   int
		wantErr bool
	}{
		{"winter", 2, false},
		{"Winter", 9, false},
		{"airplane", 1, false}, // overlaps "air", but it's revealed
		{"", 1, true},
		{"ice cream", 1, true},
		{"x-ray", 1, true},
		{"café", 1, true},
		{"winter", 0, true},
		{"winter", 10, true},
		{"snow", 1, true},
		{"SNOWMAN", 1, true},
		{"snowmen", 1, false},
		{"africans", 1, true},
		{"abcdefghijklmnopqrstuvwxyzabcde", 1, true},
	}
	for _, tt := range tests {
		err := ValidateClue(SpymasterResponse{Clue: tt.clue, Count: tt.count}, board)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateClue(%q, %d) = %v, wantErr %t", tt.clue, tt.count, err, tt.wantErr)
		}
	}
}
