package operative

import "testing"

func TestAllowBonus(t *testing.T) {
	tests := []struct {
		name           string
		oursLeft       int
		theirsLeft     int
		margin         float64
		assassinInTop3 bool
		want           bool
	}{
		{
			name:       "not behind (deficit < 2): never take the bonus",
			oursLeft:   3,
			theirsLeft: 2,
			margin:     0.9, // even a great margin doesn't matter here
			want:       false,
		},
		{
			name:       "tied: never take the bonus",
			oursLeft:   3,
			theirsLeft: 3,
			margin:     0.9,
			want:       false,
		},
		{
			name:       "exactly deficit 2, opponent not close to winning, margin above 0.20 threshold",
			oursLeft:   5,
			theirsLeft: 3,
			margin:     0.21,
			want:       true,
		},
		{
			name:       "exactly deficit 2, opponent not close to winning, margin at 0.20 does not clear it",
			oursLeft:   5,
			theirsLeft: 3,
			margin:     0.20,
			want:       false,
		},
		{
			name:       "behind but opponent not close to winning, weak margin: pass",
			oursLeft:   5,
			theirsLeft: 3,
			margin:     0.10,
			want:       false,
		},
		{
			name:       "opponent about to win (theirsLeft<=2): lower 0.05 bar applies",
			oursLeft:   4,
			theirsLeft: 2,
			margin:     0.06,
			want:       true,
		},
		{
			name:       "opponent about to win, margin at 0.05 does not clear it",
			oursLeft:   4,
			theirsLeft: 2,
			margin:     0.05,
			want:       false,
		},
		{
			name:       "opponent about to win, theirsLeft=1, tiny positive margin is enough",
			oursLeft:   6,
			theirsLeft: 1,
			margin:     0.051,
			want:       true,
		},
		{
			name:       "opponent about to win, theirsLeft=0 edge case, still gated by margin",
			oursLeft:   6,
			theirsLeft: 0,
			margin:     0.01,
			want:       false,
		},
		{
			name:           "assassin in top 3 hard-blocks even a great margin and losing position",
			oursLeft:       6,
			theirsLeft:     1,
			margin:         0.9,
			assassinInTop3: true,
			want:           false,
		},
		{
			name:           "assassin in top 3 hard-blocks even when not behind",
			oursLeft:       2,
			theirsLeft:     2,
			margin:         0.9,
			assassinInTop3: true,
			want:           false,
		},
		{
			name:       "large deficit, opponent far from winning, strong margin: take it",
			oursLeft:   6,
			theirsLeft: 2,
			margin:     0.25,
			want:       true,
		},
		{
			name:       "negative margin (a non-team word actually scores higher): never take it",
			oursLeft:   6,
			theirsLeft: 1,
			margin:     -0.1,
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := AllowBonus(tc.oursLeft, tc.theirsLeft, tc.margin, tc.assassinInTop3)
			if got != tc.want {
				t.Errorf("AllowBonus(oursLeft=%d, theirsLeft=%d, margin=%v, assassinInTop3=%v) = %v, want %v",
					tc.oursLeft, tc.theirsLeft, tc.margin, tc.assassinInTop3, got, tc.want)
			}
		})
	}
}
