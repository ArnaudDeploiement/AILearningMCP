// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package engine

import (
	"strings"
	"testing"
	"time"

	"tutor-mcp/models"
)

// TestEstimateReviewMinutesBranches confirms both branches of estimateReviewMinutes
// are exercised: high-lapse → 12 minutes, otherwise → 8 minutes.
func TestEstimateReviewMinutesBranches(t *testing.T) {
	cases := []struct {
		name string
		cs   *models.ConceptState
		want int
	}{
		{"few lapses", &models.ConceptState{Lapses: 1}, 8},
		{"many lapses", &models.ConceptState{Lapses: 5}, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := estimateReviewMinutes(tc.cs); got != tc.want {
				t.Errorf("estimateReviewMinutes(%+v) = %d, want %d", tc.cs, got, tc.want)
			}
		})
	}

	// Indirectly exercised through ComputeAlerts → ensures the high-lapse
	// branch shows up in a recommended action when retention is low.
	states := []*models.ConceptState{
		{Concept: "X", Stability: 0.1, ElapsedDays: 30, PMastery: 0.3, Lapses: 5, CardState: "review",
			LastReview: ptrTime(time.Now().AddDate(0, 0, -30))},
	}
	alerts := ComputeAlerts(states, nil, time.Time{})
	for _, a := range alerts {
		if a.Type == models.AlertForgetting && !strings.Contains(a.RecommendedAction, "12") {
			t.Errorf("expected '12 minutes' in recommended action for high-lapse concept, got %q", a.RecommendedAction)
		}
	}
}

// TestComputeAlerts_ErrorTypeRecommendations covers the three error-type
// branches in the ZPD_DRIFT recommendation builder.
func TestComputeAlerts_ErrorTypeRecommendations(t *testing.T) {
	cases := []struct {
		name      string
		errorType string
		wantSub   string
	}{
		{"knowledge gap", "KNOWLEDGE_GAP", "lacune"},
		{"logic error", "LOGIC_ERROR", "logique"},
		{"syntax error", "SYNTAX_ERROR", "syntaxe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interactions := []*models.Interaction{
				{Concept: "X", Success: false, ErrorType: tc.errorType},
				{Concept: "X", Success: false, ErrorType: tc.errorType},
				{Concept: "X", Success: false, ErrorType: tc.errorType},
			}
			states := []*models.ConceptState{{Concept: "X", PMastery: 0.3, CardState: "learning"}}
			alerts := ComputeAlerts(states, interactions, time.Time{})

			found := false
			for _, a := range alerts {
				if a.Type == models.AlertZPDDrift && a.Concept == "X" {
					found = true
					if !strings.Contains(a.RecommendedAction, tc.wantSub) {
						t.Errorf("recommended action %q missing %q", a.RecommendedAction, tc.wantSub)
					}
				}
			}
			if !found {
				t.Error("expected ZPD_DRIFT alert")
			}
		})
	}
}

// TestComputeAlerts_OverloadOnLongSession covers the sessionStart > 45m branch.
func TestComputeAlerts_OverloadOnLongSession(t *testing.T) {
	start := time.Now().Add(-50 * time.Minute)
	alerts := ComputeAlerts(nil, nil, start)
	found := false
	for _, a := range alerts {
		if a.Type == models.AlertOverload {
			found = true
		}
	}
	if !found {
		t.Error("expected OVERLOAD alert when sessionStart > 45m ago")
	}
}

// TestComputeAlerts_NoOverloadOnShortSession covers the negative branch — a
// recent sessionStart does NOT produce an OVERLOAD alert.
func TestComputeAlerts_NoOverloadOnShortSession(t *testing.T) {
	start := time.Now().Add(-5 * time.Minute)
	alerts := ComputeAlerts(nil, nil, start)
	for _, a := range alerts {
		if a.Type == models.AlertOverload {
			t.Errorf("did not expect OVERLOAD with 5-minute session, got %+v", a)
		}
	}
}

// TestComputeAlerts_PlateauDetected uses a long run of successes so the PFA
// sigmoid saturates and the last 4 deltas all fall below 0.025.
func TestComputeAlerts_PlateauDetected(t *testing.T) {
	// With pfaBetaSuccess = 0.11, the sigmoid only saturates well after 20+
	// successes — by then deltas drop below the 0.025 plateau threshold.
	var interactions []*models.Interaction
	for i := 0; i < 30; i++ {
		interactions = append(interactions, &models.Interaction{Concept: "P", Success: true})
	}
	alerts := ComputeAlerts(nil, interactions, time.Time{})

	found := false
	for _, a := range alerts {
		if a.Type == models.AlertPlateau && a.Concept == "P" {
			found = true
		}
	}
	if !found {
		t.Error("expected PLATEAU alert after sustained successes")
	}
}

// TestComputeMetacognitiveAlerts_DifficultyBranch covers the second
// AFFECT_NEGATIVE branch (perceived_difficulty == 1 on two consecutives).
func TestComputeMetacognitiveAlerts_DifficultyBranch(t *testing.T) {
	affects := []*models.AffectState{
		{PerceivedDifficulty: 1, Satisfaction: 4},
		{PerceivedDifficulty: 1, Satisfaction: 4},
	}
	alerts := ComputeMetacognitiveAlerts(nil, 0, affects, nil)
	for _, a := range alerts {
		if a.Type == models.AlertAffectNegative {
			return
		}
	}
	t.Error("expected AFFECT_NEGATIVE alert from difficulty=1 branch")
}

// TestComputeMetacognitiveAlerts_NegCalibration covers the bias < 0 branch.
func TestComputeMetacognitiveAlerts_NegCalibration(t *testing.T) {
	alerts := ComputeMetacognitiveAlerts(nil, -2.0, nil, nil)
	for _, a := range alerts {
		if a.Type == models.AlertCalibrationDiverging {
			if !strings.Contains(a.RecommendedAction, "sous-estimation") {
				t.Errorf("expected 'sous-estimation' in action, got %q", a.RecommendedAction)
			}
			return
		}
	}
	t.Error("expected CALIBRATION_DIVERGING alert with negative bias")
}

// TestComputeMetacognitiveAlerts_TransferBlockedNoData ensures the
// transfer-blocked branch is skipped when no transfer data is provided.
func TestComputeMetacognitiveAlerts_TransferBlockedNoData(t *testing.T) {
	alerts := ComputeMetacognitiveAlerts(nil, 0, nil, nil)
	for _, a := range alerts {
		if a.Type == models.AlertTransferBlocked {
			t.Errorf("did not expect TRANSFER_BLOCKED without input data, got %+v", a)
		}
	}
}
