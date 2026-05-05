// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package engine

import (
	"math"
	"testing"
)

// TestDecide_TierTable exercises the 9 cells of the (tier, trend)
// table from docs/regulation-design/06-fade-controller.md §4.3.
// Score is sampled at the middle of each tier (0.15, 0.5, 0.85) to
// stay clear of boundary effects (boundaries are tested separately).
func TestDecide_TierTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		score float64
		trend AutonomyTrend
		want  FadeParams
	}{
		// Low tier (score < 0.3)
		{
			name:  "Low/declining stays Low",
			score: 0.15, trend: AutonomyTrendDeclining,
			want: FadeParams{HintLevelFull, WebhookFrequencyDaily, ZPDAggressivenessGentle, true},
		},
		{
			name:  "Low/stable stays Low",
			score: 0.15, trend: AutonomyTrendStable,
			want: FadeParams{HintLevelFull, WebhookFrequencyDaily, ZPDAggressivenessGentle, true},
		},
		{
			name:  "Low/improving moves up to Mid",
			score: 0.15, trend: AutonomyTrendImproving,
			want: FadeParams{HintLevelPartial, WebhookFrequencyWeekly, ZPDAggressivenessNormal, true},
		},

		// Mid tier (0.3 <= score < 0.7)
		{
			name:  "Mid/declining falls to Low",
			score: 0.5, trend: AutonomyTrendDeclining,
			want: FadeParams{HintLevelFull, WebhookFrequencyDaily, ZPDAggressivenessGentle, true},
		},
		{
			name:  "Mid/stable stays Mid",
			score: 0.5, trend: AutonomyTrendStable,
			want: FadeParams{HintLevelPartial, WebhookFrequencyWeekly, ZPDAggressivenessNormal, true},
		},
		{
			name:  "Mid/improving moves up to High",
			score: 0.5, trend: AutonomyTrendImproving,
			want: FadeParams{HintLevelNone, WebhookFrequencyOff, ZPDAggressivenessPush, false},
		},

		// High tier (score >= 0.7)
		{
			name:  "High/declining falls to Mid",
			score: 0.85, trend: AutonomyTrendDeclining,
			want: FadeParams{HintLevelPartial, WebhookFrequencyWeekly, ZPDAggressivenessNormal, true},
		},
		{
			name:  "High/stable stays High",
			score: 0.85, trend: AutonomyTrendStable,
			want: FadeParams{HintLevelNone, WebhookFrequencyOff, ZPDAggressivenessPush, false},
		},
		{
			name:  "High/improving stays High (clamp)",
			score: 0.85, trend: AutonomyTrendImproving,
			want: FadeParams{HintLevelNone, WebhookFrequencyOff, ZPDAggressivenessPush, false},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Decide(tc.score, tc.trend)
			if got != tc.want {
				t.Fatalf("Decide(%v, %q) = %+v, want %+v", tc.score, tc.trend, got, tc.want)
			}
		})
	}
}

// TestDecide_ScoreBoundaries verifies the inclusive-at-lower-bound
// behaviour of the tier classifier and the clamp on out-of-range
// inputs.
func TestDecide_ScoreBoundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		score    float64
		wantHint HintLevel
	}{
		{"score=0.0 -> Low", 0.0, HintLevelFull},
		{"score=0.299 -> Low", 0.299, HintLevelFull},
		{"score=0.30 -> Mid", 0.30, HintLevelPartial},
		{"score=0.5 -> Mid", 0.5, HintLevelPartial},
		{"score=0.699 -> Mid", 0.699, HintLevelPartial},
		{"score=0.70 -> High", 0.70, HintLevelNone},
		{"score=1.0 -> High", 1.0, HintLevelNone},
		{"score=-0.1 (clamp) -> Low", -0.1, HintLevelFull},
		{"score=1.5 (clamp) -> High", 1.5, HintLevelNone},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Decide(tc.score, AutonomyTrendStable)
			if got.HintLevel != tc.wantHint {
				t.Fatalf("Decide(%v, stable).HintLevel = %q, want %q",
					tc.score, got.HintLevel, tc.wantHint)
			}
		})
	}
}

// TestDecide_NaNScore documents the NaN fallback (degrades to Mid).
func TestDecide_NaNScore(t *testing.T) {
	t.Parallel()
	got := Decide(math.NaN(), AutonomyTrendStable)
	if got.HintLevel != HintLevelPartial {
		t.Fatalf("NaN score should degrade to Mid (partial hints), got %q", got.HintLevel)
	}
}

// TestDecide_TrendDefensive verifies that unknown / mistyped trend
// strings are treated as "stable" (no movement). Case-sensitive: the
// canonical lower-case forms are the only ones that move the tier.
func TestDecide_TrendDefensive(t *testing.T) {
	t.Parallel()

	defensiveTrends := []AutonomyTrend{
		"",
		"garbage",
		"IMPROVING", // case-sensitive — not recognised
		"Improving",
		"stable ", // trailing space
		"declining\n",
	}

	for _, tr := range defensiveTrends {
		tr := tr
		t.Run(string(tr), func(t *testing.T) {
			t.Parallel()
			// Mid tier base — defensive trend should keep it Mid.
			got := Decide(0.5, tr)
			want := FadeParams{HintLevelPartial, WebhookFrequencyWeekly, ZPDAggressivenessNormal, true}
			if got != want {
				t.Fatalf("Decide(0.5, %q) = %+v, want %+v (defensive trend should be stable)",
					tr, got, want)
			}
		})
	}
}

// TestDecide_Pure verifies determinism and absence of state: the
// function must return the same FadeParams for the same inputs across
// repeated calls.
func TestDecide_Pure(t *testing.T) {
	t.Parallel()

	first := Decide(0.5, AutonomyTrendImproving)
	for i := 0; i < 100; i++ {
		got := Decide(0.5, AutonomyTrendImproving)
		if got != first {
			t.Fatalf("Decide is not deterministic: got %+v on iter %d, want %+v",
				got, i, first)
		}
	}
}

// TestDecide_Monotonic asserts that, holding trend constant, as the
// score rises through the tiers, HintLevel can only relax (full ->
// partial -> none) and never re-tighten. This is the property the
// integration test will rely on.
func TestDecide_Monotonic(t *testing.T) {
	t.Parallel()

	hintRank := map[HintLevel]int{
		HintLevelFull:    2,
		HintLevelPartial: 1,
		HintLevelNone:    0,
	}

	for _, trend := range []AutonomyTrend{AutonomyTrendDeclining, AutonomyTrendStable, AutonomyTrendImproving} {
		var lastRank = 999
		for s := 0.0; s <= 1.0; s += 0.05 {
			fp := Decide(s, trend)
			r := hintRank[fp.HintLevel]
			if r > lastRank {
				t.Fatalf("non-monotonic for trend=%q at score=%v: hint=%q rank=%d > lastRank=%d",
					trend, s, fp.HintLevel, r, lastRank)
			}
			lastRank = r
		}
	}
}
