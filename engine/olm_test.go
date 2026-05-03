// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package engine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"tutor-mcp/db"
	"tutor-mcp/models"
)

var olmTestDSNCounter int64

// newOLMTestStore opens a fresh in-memory SQLite database with migrations applied
// and returns the wrapped Store + raw *sql.DB. Reused across olm_test.go.
func newOLMTestStore(t *testing.T) (*db.Store, *sql.DB) {
	t.Helper()
	n := atomic.AddInt64(&olmTestDSNCounter, 1)
	dsn := fmt.Sprintf("file:olm_%s_%d?mode=memory&cache=shared", t.Name(), n)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	return db.NewStore(raw), raw
}

func TestBuildOLMSnapshot_NoDomain_ReturnsError(t *testing.T) {
	store, _ := newOLMTestStore(t)

	snap, err := BuildOLMSnapshot(store, "nonexistent_learner", "")
	if err == nil {
		t.Fatalf("expected error for learner with no active domain, got snap=%+v", snap)
	}
}

// seedDomain inserts a non-archived (or archived) domain with the given concepts.
func seedDomain(t *testing.T, raw *sql.DB, learnerID, name string, concepts []string, prereqs map[string][]string, archived bool) string {
	t.Helper()
	graphJSON, err := json.Marshal(map[string]any{
		"concepts":      concepts,
		"prerequisites": prereqs,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := learnerID + "_" + name
	archInt := 0
	if archived {
		archInt = 1
	}
	_, err = raw.Exec(
		`INSERT INTO domains (id, learner_id, name, graph_json, personal_goal, archived, value_framings_json, last_value_axis, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, '', '', ?)`,
		id, learnerID, name, string(graphJSON), "test goal", archInt, time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// seedConceptState upserts a concept_state row for a concept.
func seedConceptState(t *testing.T, store *db.Store, learnerID, concept string, mastery float64, cardState string) {
	t.Helper()
	cs := models.NewConceptState(learnerID, concept)
	cs.PMastery = mastery
	cs.CardState = cardState
	if cardState != "new" {
		cs.Stability = 5.0
		cs.Reps = 1
		now := time.Now().UTC()
		cs.LastReview = &now
		cs.ScheduledDays = 7
	}
	if err := store.UpsertConceptState(cs); err != nil {
		t.Fatal(err)
	}
}

func seedLearner(t *testing.T, raw *sql.DB, learnerID string) {
	t.Helper()
	_, err := raw.Exec(
		`INSERT INTO learners (id, email, password_hash, objective, created_at) VALUES (?, ?, 'h', 'obj', ?)`,
		learnerID, learnerID+"@t.com", time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildOLMSnapshot_MasteryBuckets(t *testing.T) {
	store, raw := newOLMTestStore(t)
	seedLearner(t, raw, "L1")
	seedDomain(t, raw, "L1", "math",
		[]string{"a", "b", "c", "d", "e"},
		map[string][]string{"b": {"a"}, "c": {"b"}},
		false,
	)
	seedConceptState(t, store, "L1", "a", 0.85, "review") // Solid
	seedConceptState(t, store, "L1", "b", 0.50, "review") // InProgress
	seedConceptState(t, store, "L1", "c", 0.10, "review") // Fragile
	seedConceptState(t, store, "L1", "d", 0.0, "new")     // NotStarted
	// "e" has NO concept_state row → also NotStarted

	snap, err := BuildOLMSnapshot(store, "L1", "")
	if err != nil {
		t.Fatalf("BuildOLMSnapshot: %v", err)
	}
	if snap.Solid != 1 {
		t.Errorf("Solid=%d, want 1", snap.Solid)
	}
	if snap.InProgress != 1 {
		t.Errorf("InProgress=%d, want 1", snap.InProgress)
	}
	if snap.Fragile != 1 {
		t.Errorf("Fragile=%d, want 1", snap.Fragile)
	}
	if snap.NotStarted != 2 {
		t.Errorf("NotStarted=%d (incl. concept with no state), want 2", snap.NotStarted)
	}
}

func TestBuildOLMSnapshot_FocusForgettingCritical(t *testing.T) {
	store, raw := newOLMTestStore(t)
	seedLearner(t, raw, "L1")
	seedDomain(t, raw, "L1", "math",
		[]string{"a", "b"},
		map[string][]string{"b": {"a"}},
		false,
	)
	// 'a' is in deep forgetting (low retention).
	cs := models.NewConceptState("L1", "a")
	cs.PMastery = 0.40
	cs.Stability = 1.0
	cs.ElapsedDays = 30
	cs.Reps = 5
	cs.CardState = "review"
	now := time.Now().UTC().Add(-30 * 24 * time.Hour)
	cs.LastReview = &now
	if err := store.UpsertConceptState(cs); err != nil {
		t.Fatal(err)
	}

	snap, err := BuildOLMSnapshot(store, "L1", "")
	if err != nil {
		t.Fatalf("BuildOLMSnapshot: %v", err)
	}
	if snap.FocusConcept != "a" {
		t.Errorf("FocusConcept=%q, want a", snap.FocusConcept)
	}
	if snap.FocusUrgency != models.UrgencyCritical && snap.FocusUrgency != models.UrgencyWarning {
		t.Errorf("FocusUrgency=%q, want critical or warning (FORGETTING)", snap.FocusUrgency)
	}
	if snap.FocusReason == "" {
		t.Error("FocusReason should be non-empty (retention …%)")
	}
}

func TestBuildOLMSnapshot_FocusFallsBackToFrontier(t *testing.T) {
	store, raw := newOLMTestStore(t)
	seedLearner(t, raw, "L1")
	seedDomain(t, raw, "L1", "math",
		[]string{"a", "b", "c"},
		map[string][]string{"b": {"a"}, "c": {"b"}},
		false,
	)
	// 'a' mastered → 'b' is on the frontier. 'b' has no state yet.
	seedConceptState(t, store, "L1", "a", 0.90, "review")

	snap, err := BuildOLMSnapshot(store, "L1", "")
	if err != nil {
		t.Fatalf("BuildOLMSnapshot: %v", err)
	}
	if snap.FocusConcept != "b" {
		t.Errorf("FocusConcept=%q, want b (frontier)", snap.FocusConcept)
	}
	if snap.FocusUrgency != models.UrgencyInfo {
		t.Errorf("FocusUrgency=%q, want info (frontier fallback)", snap.FocusUrgency)
	}
	if snap.FocusReason != "prochain palier" {
		t.Errorf("FocusReason=%q, want 'prochain palier'", snap.FocusReason)
	}
}

func TestBuildOLMSnapshot_MetacognitiveSignals(t *testing.T) {
	store, raw := newOLMTestStore(t)
	seedLearner(t, raw, "L1")
	seedDomain(t, raw, "L1", "math", []string{"a"}, nil, false)
	seedConceptState(t, store, "L1", "a", 0.50, "review")

	// Affect history. UpsertAffectState orders by created_at desc; we want
	// the most recent to have AutonomyScore=0.3 and the oldest (3rd back)
	// 0.7. Insert oldest first so created_at increases monotonically.
	for _, score := range []float64{0.7, 0.5, 0.3} {
		af := &models.AffectState{
			LearnerID:     "L1",
			SessionID:     fmt.Sprintf("s%.0f", score*10),
			Energy:        3,
			Satisfaction:  2,
			AutonomyScore: score,
		}
		if err := store.UpsertAffectState(af); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond) // ensure distinct created_at
	}

	snap, err := BuildOLMSnapshot(store, "L1", "")
	if err != nil {
		t.Fatalf("BuildOLMSnapshot: %v", err)
	}
	// Most recent autonomy (0.3) < oldest of the 3 (0.7) by 0.40 → declining.
	if snap.AutonomyTrend != "declining" {
		t.Errorf("AutonomyTrend=%q, want declining", snap.AutonomyTrend)
	}
	if snap.AffectTrend != "stable" && snap.AffectTrend != "" {
		// All 3 satisfaction = 2 → no diff → stable. Empty also acceptable
		// if implementation returns "" when diff is below threshold.
		t.Errorf("AffectTrend=%q, want stable or empty", snap.AffectTrend)
	}
}
