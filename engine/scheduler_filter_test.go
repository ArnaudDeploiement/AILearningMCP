package engine

import (
	"testing"
	"time"

	"learning-runtime/models"
)

// TestSchedulerFilters_DropOrphanConcepts protects against the cosmos-reported
// regression: scheduler.checkCriticalAlerts used to feed every concept_state into
// ComputeAlerts, so a deleted-domain concept (whose row in `domains` is gone but
// whose concept_states / interactions remain by design) could trigger a critical
// webhook on a concept the learner no longer has. Filter helpers must drop them.
func TestSchedulerFilters_DropOrphanConcepts(t *testing.T) {
	active := map[string]bool{"Variables": true}

	states := []*models.ConceptState{
		// Active — would generate a FORGETTING alert.
		{Concept: "Variables", Stability: 0.2, ElapsedDays: 5, PMastery: 0.5, CardState: "review",
			LastReview: ptrTime(time.Now().AddDate(0, 0, -5))},
		// Orphan — same retention profile, must NOT reach ComputeAlerts.
		{Concept: "Bases de la cuisine", Stability: 0.2, ElapsedDays: 5, PMastery: 0.5, CardState: "review",
			LastReview: ptrTime(time.Now().AddDate(0, 0, -5))},
	}
	interactions := []*models.Interaction{
		{Concept: "Variables", Success: false},
		{Concept: "Bases de la cuisine", Success: false},
	}

	filteredStates := filterStatesByActiveConcepts(states, active)
	filteredInteractions := filterInteractionsByActiveConcepts(interactions, active)

	if len(filteredStates) != 1 || filteredStates[0].Concept != "Variables" {
		t.Fatalf("filtered states = %+v, want only [Variables]", filteredStates)
	}
	if len(filteredInteractions) != 1 || filteredInteractions[0].Concept != "Variables" {
		t.Fatalf("filtered interactions = %+v, want only [Variables]", filteredInteractions)
	}

	alerts := ComputeAlerts(filteredStates, filteredInteractions, time.Time{})
	for _, a := range alerts {
		if a.Concept != "" && !active[a.Concept] {
			t.Errorf("invariant broken: alert references orphan concept %q", a.Concept)
		}
	}
}

// TestSchedulerFilters_EmptyActiveSet returns nil so a learner with zero domains
// can't receive alerts on any of their orphan history.
func TestSchedulerFilters_EmptyActiveSet(t *testing.T) {
	states := []*models.ConceptState{
		{Concept: "Variables", Stability: 0.2, ElapsedDays: 5, PMastery: 0.5, CardState: "review",
			LastReview: ptrTime(time.Now().AddDate(0, 0, -5))},
	}
	if got := filterStatesByActiveConcepts(states, nil); got != nil {
		t.Errorf("nil set should yield nil states, got %+v", got)
	}
	if got := filterStatesByActiveConcepts(states, map[string]bool{}); got != nil {
		t.Errorf("empty set should yield nil states, got %+v", got)
	}
}
