package algorithms

import (
	"math"
	"testing"
	"time"
)

func approxEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

func TestRetrievability(t *testing.T) {
	tests := []struct {
		elapsed   int
		stability float64
		want      float64
	}{
		{0, 1.0, 1.0},
		{1, 1.0, 0.9},
		{10, 1.0, 0.5468},
		{0, 5.0, 1.0},
		{5, 5.0, 0.9},
	}
	for _, tt := range tests {
		got := Retrievability(tt.elapsed, tt.stability)
		if !approxEqual(got, tt.want, 0.01) {
			t.Errorf("Retrievability(%d, %.1f) = %.4f, want ~%.4f", tt.elapsed, tt.stability, got, tt.want)
		}
	}
}

func TestInitialStability(t *testing.T) {
	if s := InitialStability(Again); !approxEqual(s, defaultWeights[0], 0.001) {
		t.Errorf("InitialStability(Again) = %f, want %f", s, defaultWeights[0])
	}
	if s := InitialStability(Good); !approxEqual(s, defaultWeights[2], 0.001) {
		t.Errorf("InitialStability(Good) = %f, want %f", s, defaultWeights[2])
	}
}

func TestInitialDifficulty(t *testing.T) {
	d := InitialDifficulty(Good)
	if d < 1 || d > 10 {
		t.Errorf("InitialDifficulty(Good) = %f, want in [1,10]", d)
	}
}

func TestReviewCard(t *testing.T) {
	now := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	card := NewFSRSCard()

	card = ReviewCard(card, Good, now)
	if card.State != Review {
		t.Errorf("after Good on new card, state = %s, want Review", card.State)
	}
	if card.Stability <= 0 {
		t.Errorf("stability should be positive, got %f", card.Stability)
	}
	if card.ScheduledDays < 1 {
		t.Errorf("scheduled_days should be >= 1, got %d", card.ScheduledDays)
	}
	if card.Reps != 1 {
		t.Errorf("reps = %d, want 1", card.Reps)
	}

	next := now.AddDate(0, 0, card.ScheduledDays)
	card = ReviewCard(card, Again, next)
	if card.State != Relearning {
		t.Errorf("after Again on review card, state = %s, want Relearning", card.State)
	}
	if card.Lapses != 1 {
		t.Errorf("lapses = %d, want 1", card.Lapses)
	}
}

func TestNextInterval(t *testing.T) {
	interval := NextInterval(1.0, 0.9)
	if interval < 1 {
		t.Errorf("interval for stability=1 should be >= 1, got %d", interval)
	}
	interval5 := NextInterval(5.0, 0.9)
	if interval5 <= interval {
		t.Errorf("higher stability should give longer interval: %d vs %d", interval5, interval)
	}
}
