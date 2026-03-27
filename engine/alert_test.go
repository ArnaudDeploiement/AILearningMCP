package engine

import (
	"testing"
	"time"

	"learning-runtime/models"
)

func ptrTime(t time.Time) *time.Time { return &t }

func TestComputeAlertsForgetting(t *testing.T) {
	states := []*models.ConceptState{
		{Concept: "goroutines", Stability: 0.2, ElapsedDays: 5, PMastery: 0.5, CardState: "review",
			LastReview: ptrTime(time.Now().AddDate(0, 0, -5))},
	}
	alerts := ComputeAlerts(states, nil, time.Time{})
	found := false
	for _, a := range alerts {
		if a.Type == models.AlertForgetting && a.Concept == "goroutines" {
			found = true
			if a.Urgency != models.UrgencyCritical && a.Urgency != models.UrgencyWarning {
				t.Errorf("urgency = %s, want critical or warning", a.Urgency)
			}
		}
	}
	if !found {
		t.Error("expected FORGETTING alert for goroutines")
	}
}

func TestComputeAlertsMasteryReady(t *testing.T) {
	states := []*models.ConceptState{{Concept: "basics", PMastery: 0.90, CardState: "review"}}
	alerts := ComputeAlerts(states, nil, time.Time{})
	found := false
	for _, a := range alerts {
		if a.Type == models.AlertMasteryReady && a.Concept == "basics" {
			found = true
		}
	}
	if !found {
		t.Error("expected MASTERY_READY for basics")
	}
}

func TestComputeAlertsZPDDrift(t *testing.T) {
	interactions := []*models.Interaction{
		{Concept: "pointers", Success: false},
		{Concept: "pointers", Success: false},
		{Concept: "pointers", Success: false},
	}
	states := []*models.ConceptState{{Concept: "pointers", PMastery: 0.3, CardState: "learning"}}
	alerts := ComputeAlerts(states, interactions, time.Time{})
	found := false
	for _, a := range alerts {
		if a.Type == models.AlertZPDDrift && a.Concept == "pointers" {
			found = true
		}
	}
	if !found {
		t.Error("expected ZPD_DRIFT for pointers")
	}
}
