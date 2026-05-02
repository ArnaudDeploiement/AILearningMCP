// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package engine

import (
	"testing"

	"tutor-mcp/models"
)

// TestRoutePlateauTriggers covers the PLATEAU path including format rotation
// and the "skip if practiced 2+ times this session" branch.
func TestRoutePlateauTriggers(t *testing.T) {
	plateauAlert := models.Alert{
		Type: models.AlertPlateau, Concept: "loops",
		Urgency: models.UrgencyWarning, SessionsStalled: 3,
	}

	t.Run("first plateau on concept picks debugging format", func(t *testing.T) {
		got := Route([]models.Alert{plateauAlert}, nil, nil, nil, nil)
		if got.Type != models.ActivityDebuggingCase {
			t.Errorf("type = %s, want DEBUGGING_CASE", got.Type)
		}
		if got.Concept != "loops" {
			t.Errorf("concept = %s, want loops", got.Concept)
		}
		if got.Format == "" {
			t.Errorf("format must be set, got empty")
		}
	})

	t.Run("rotates format with interaction count", func(t *testing.T) {
		// Two prior interactions on this concept rotate to the third format.
		recent := []*models.Interaction{
			{Concept: "loops"}, {Concept: "loops"},
		}
		got := Route([]models.Alert{plateauAlert}, nil, nil, recent, nil)
		// 2 % 4 == 2 → "teaching_exercise"
		if got.Format != "teaching_exercise" {
			t.Errorf("format = %q, want 'teaching_exercise' (rotated by interaction count)", got.Format)
		}
	})

	t.Run("skip if practiced 2+ times this session", func(t *testing.T) {
		session := map[string]int{"loops": 2}
		// With a state available, the router falls back to recall.
		states := []*models.ConceptState{
			{Concept: "other", Stability: 5, ElapsedDays: 3, CardState: "review"},
		}
		got := Route([]models.Alert{plateauAlert}, nil, states, nil, session)
		if got.Type == models.ActivityDebuggingCase {
			t.Errorf("expected plateau to be skipped after 2+ session practice, got %+v", got)
		}
	})
}

// TestRouteOverload covers the OVERLOAD priority.
func TestRouteOverload(t *testing.T) {
	got := Route([]models.Alert{{Type: models.AlertOverload, Urgency: models.UrgencyInfo}}, nil, nil, nil, nil)
	if got.Type != models.ActivityRest {
		t.Errorf("type = %s, want REST", got.Type)
	}
	if got.PromptForLLM == "" {
		t.Error("expected non-empty prompt for LLM")
	}
}

// TestRouteMasteryReady covers the MASTERY_READY priority and the dedup branch.
func TestRouteMasteryReady(t *testing.T) {
	alert := models.Alert{Type: models.AlertMasteryReady, Concept: "channels", Urgency: models.UrgencyInfo}

	t.Run("first time fires", func(t *testing.T) {
		got := Route([]models.Alert{alert}, nil, nil, nil, nil)
		if got.Type != models.ActivityMasteryChallenge {
			t.Errorf("type = %s, want MASTERY_CHALLENGE", got.Type)
		}
		if got.Concept != "channels" {
			t.Errorf("concept = %s", got.Concept)
		}
	})

	t.Run("skipped if already practiced this session", func(t *testing.T) {
		session := map[string]int{"channels": 1}
		got := Route([]models.Alert{alert}, nil, nil, nil, session)
		if got.Type == models.ActivityMasteryChallenge {
			t.Errorf("expected mastery_challenge to be skipped after session practice, got %+v", got)
		}
	})
}

// TestRouteFrontierSkipsAlreadyPracticed covers the new-concept loop where the
// first concept is in the session map and we fall through to the next.
func TestRouteFrontierSkipsAlreadyPracticed(t *testing.T) {
	frontier := []string{"A", "B"}
	session := map[string]int{"A": 1}
	got := Route(nil, frontier, nil, nil, session)
	if got.Type != models.ActivityNewConcept {
		t.Errorf("type = %s, want NEW_CONCEPT", got.Type)
	}
	if got.Concept != "B" {
		t.Errorf("concept = %s, want B (A was practiced)", got.Concept)
	}
}

// TestRouteDefaultRecall_PrefersUnpracticed exercises the "prefer unpracticed
// concept" branch in the default-recall fallback.
func TestRouteDefaultRecall_PrefersUnpracticed(t *testing.T) {
	states := []*models.ConceptState{
		// Lower retention but practiced this session.
		{Concept: "A", Stability: 0.5, ElapsedDays: 30, PMastery: 0.4, CardState: "review"},
		// Higher retention but unpracticed → must be picked.
		{Concept: "B", Stability: 5, ElapsedDays: 4, PMastery: 0.6, CardState: "review"},
	}
	session := map[string]int{"A": 1}
	got := Route(nil, nil, states, nil, session)
	if got.Concept != "B" {
		t.Errorf("expected to skip practiced 'A' for 'B', got %s", got.Concept)
	}
}

// TestRouteDefaultRecall_FallsBackToAnyConcept exercises the chosen==nil
// branch where every concept has been practiced this session.
func TestRouteDefaultRecall_FallsBackToAnyConcept(t *testing.T) {
	states := []*models.ConceptState{
		{Concept: "A", Stability: 5, ElapsedDays: 4, PMastery: 0.5, CardState: "review"},
	}
	session := map[string]int{"A": 1}
	got := Route(nil, nil, states, nil, session)
	if got.Type != models.ActivityRecall {
		t.Errorf("type = %s, want RECALL_EXERCISE", got.Type)
	}
	if got.Concept != "A" {
		t.Errorf("concept = %s, want A (only one available)", got.Concept)
	}
}

// TestRouteDefaultRecall_SkipsNewCardState — concepts in 'new' state are not
// candidates for the default-recall fallback.
func TestRouteDefaultRecall_SkipsNewCardState(t *testing.T) {
	states := []*models.ConceptState{
		{Concept: "fresh", Stability: 1, ElapsedDays: 0, PMastery: 0.1, CardState: "new"},
	}
	got := Route(nil, nil, states, nil, nil)
	if got.Type != models.ActivityRest {
		t.Errorf("expected REST when only candidate is in 'new' state, got %s", got.Type)
	}
}

// TestRouteFinalRest — no alerts, no frontier, no states → REST.
func TestRouteFinalRest(t *testing.T) {
	got := Route(nil, nil, nil, nil, nil)
	if got.Type != models.ActivityRest {
		t.Errorf("type = %s, want REST", got.Type)
	}
	if got.PromptForLLM == "" {
		t.Error("expected non-empty rest prompt")
	}
}
