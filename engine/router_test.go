package engine

import (
	"testing"

	"learning-runtime/models"
)

func TestRouteForgettingPriority(t *testing.T) {
	alerts := []models.Alert{
		{Type: models.AlertForgetting, Concept: "goroutines", Urgency: models.UrgencyCritical, Retention: 0.25},
		{Type: models.AlertMasteryReady, Concept: "basics", Urgency: models.UrgencyInfo},
	}
	a := Route(alerts, nil, nil, nil, nil)
	if a.Type != models.ActivityRecall {
		t.Errorf("type = %s, want RECALL_EXERCISE", a.Type)
	}
	if a.Concept != "goroutines" {
		t.Errorf("concept = %s, want goroutines", a.Concept)
	}
}

func TestRouteZPDDrift(t *testing.T) {
	alerts := []models.Alert{{Type: models.AlertZPDDrift, Concept: "pointers", Urgency: models.UrgencyWarning}}
	a := Route(alerts, nil, nil, nil, nil)
	if a.Concept != "pointers" {
		t.Errorf("concept = %s, want pointers", a.Concept)
	}
	if a.DifficultyTarget >= 0.55 {
		t.Errorf("difficulty should be reduced, got %f", a.DifficultyTarget)
	}
}

func TestRouteNewConcept(t *testing.T) {
	a := Route(nil, []string{"interfaces"}, []*models.ConceptState{{Concept: "basics", Stability: 5.0, PMastery: 0.8}}, nil, nil)
	if a.Type != models.ActivityNewConcept {
		t.Errorf("type = %s, want NEW_CONCEPT", a.Type)
	}
	if a.Concept != "interfaces" {
		t.Errorf("concept = %s, want interfaces", a.Concept)
	}
}

func TestRouteDefaultRecall(t *testing.T) {
	states := []*models.ConceptState{
		{Concept: "A", Stability: 10, PMastery: 0.6, CardState: "review", ElapsedDays: 8},
		{Concept: "B", Stability: 5, PMastery: 0.5, CardState: "review", ElapsedDays: 4},
	}
	a := Route(nil, nil, states, nil, nil)
	if a.Type != models.ActivityRecall {
		t.Errorf("type = %s, want RECALL_EXERCISE", a.Type)
	}
}
