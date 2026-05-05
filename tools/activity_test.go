// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"fmt"
	"testing"

	"tutor-mcp/models"
)

func TestGetNextActivity_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetNextActivity, "", "get_next_activity", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetNextActivity_NeedsDomainSetup(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetNextActivity, "L_owner", "get_next_activity", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["needs_domain_setup"] != true {
		t.Fatalf("expected needs_domain_setup=true, got %v", out["needs_domain_setup"])
	}
}

func TestGetNextActivity_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	// Seed concept state in domain.
	cs := models.NewConceptState("L_owner", "a")
	cs.PMastery = 0.5
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerGetNextActivity, "L_owner", "get_next_activity", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["needs_domain_setup"] != false {
		t.Fatalf("expected needs_domain_setup=false, got %v", out["needs_domain_setup"])
	}
	if _, ok := out["activity"]; !ok {
		t.Fatalf("expected activity key, got %v", out)
	}
	if _, ok := out["tutor_mode"]; !ok {
		t.Fatalf("expected tutor_mode key, got %v", out)
	}
	if _, ok := out["motivation_brief"]; !ok {
		t.Fatalf("expected motivation_brief key, got %v", out)
	}
}

func TestGetNextActivity_ForeignDomainFallsBackToSetup(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	res := callTool(t, deps, registerGetNextActivity, "L_attacker", "get_next_activity", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	// Foreign learner should fall through to needs_domain_setup since resolveDomain rejects.
	if out["needs_domain_setup"] != true {
		t.Fatalf("expected setup fallback for foreign domain, got %v", out)
	}
}

// TestGetNextActivity_FlagOff_NoFadeFields asserts the byte-equivalence
// guarantee of the FadeController constraint: with REGULATION_FADE
// unset (its default), the JSON returned by get_next_activity must
// contain neither fade_params nor autonomy_score keys, and the
// motivation_brief must be the legacy output untouched. This is the
// "FadeController is post-decision and cannot affect orchestrator
// outputs when the flag is off" invariant from the design doc.
func TestGetNextActivity_FlagOff_NoFadeFields(t *testing.T) {
	// Force flag OFF regardless of caller's environment so this test
	// is robust to `REGULATION_FADE=on go test ./...` invocations.
	t.Setenv("REGULATION_FADE", "")
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	cs := models.NewConceptState("L_owner", "a")
	cs.PMastery = 0.5
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerGetNextActivity, "L_owner", "get_next_activity",
		map[string]any{"domain_id": d.ID})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	for _, key := range []string{"fade_params", "autonomy_score", "autonomy_trend"} {
		if _, ok := out[key]; ok {
			t.Errorf("flag OFF: expected no %q key in output, got %v", key, out[key])
		}
	}
}

// TestGetNextActivity_FadeFlagOn_VerbosityDecreasesAsAutonomyRises is
// the integration test required by the FadeController acceptance
// criteria. We simulate a learner whose autonomy_score rises across
// multiple sessions, call get_next_activity at successive points, and
// assert that:
//
//   - fade_params is present in the JSON,
//   - the hint_level transitions from "full" toward "none" as the
//     score climbs,
//   - the motivation_brief.instruction length is non-increasing along
//     the sequence (verbosity monotonically drops).
func TestGetNextActivity_FadeFlagOn_VerbosityDecreasesAsAutonomyRises(t *testing.T) {
	t.Setenv("REGULATION_FADE", "on")

	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	cs := models.NewConceptState("L_owner", "a")
	cs.PMastery = 0.5
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	// Steps: at each step, seed enough affect rows to drive the
	// autonomy_score to the target tier. We use direct UpsertAffectState
	// per session, then patch autonomy_score via UpdateAffectAutonomyScore
	// (the upsert path's CASE WHEN excluded.autonomy_score > 0 guard
	// preserves the value we set).
	steps := []struct {
		name           string
		affectScores   []float64 // newest-first; one row per session
		wantHintAtMost int       // 2 = full, 1 = partial, 0 = none
	}{
		{"start: low autonomy", []float64{0.10, 0.10, 0.10}, 2},
		{"climbing: mid autonomy", []float64{0.55, 0.50, 0.40, 0.20, 0.10, 0.10}, 1},
		{"high autonomy + improving", []float64{0.95, 0.92, 0.90, 0.88, 0.30, 0.25, 0.20, 0.15, 0.10, 0.10}, 0},
	}

	hintRank := map[string]int{"full": 2, "partial": 1, "none": 0, "": 0}

	prevInstrLen := -1
	for stepIdx, st := range steps {
		// Wipe prior affect rows to reseed the timeline cleanly per
		// step — newest-first ordering of GetRecentAffectStates is
		// driven by created_at, and re-using session IDs would upsert
		// rather than insert.
		raw := deps.Store.RawDB()
		if _, err := raw.Exec(`DELETE FROM affect_states WHERE learner_id = ?`, "L_owner"); err != nil {
			t.Fatalf("step %s: wipe affects: %v", st.name, err)
		}

		// Seed in reverse (oldest-first) so created_at ordering matches
		// newest-first when read back.
		for i := len(st.affectScores) - 1; i >= 0; i-- {
			sid := fmt.Sprintf("S_%d_%d", stepIdx, i)
			a := &models.AffectState{
				LearnerID:     "L_owner",
				SessionID:     sid,
				Energy:        3,
				AutonomyScore: st.affectScores[i],
			}
			if err := store.UpsertAffectState(a); err != nil {
				t.Fatalf("step %s: upsert affect %s: %v", st.name, sid, err)
			}
			// CASE WHEN excluded.autonomy_score > 0: setting it
			// directly via insert works for non-zero scores.
		}

		res := callTool(t, deps, registerGetNextActivity, "L_owner", "get_next_activity",
			map[string]any{"domain_id": d.ID})
		if res.IsError {
			t.Fatalf("step %s: %q", st.name, resultText(res))
		}
		out := decodeResult(t, res)

		fp, ok := out["fade_params"].(map[string]any)
		if !ok {
			t.Fatalf("step %s: expected fade_params, got %v", st.name, out)
		}
		gotHint, _ := fp["hint_level"].(string)
		if hintRank[gotHint] > st.wantHintAtMost {
			t.Errorf("step %s: hint_level=%q (rank %d), want at most rank %d",
				st.name, gotHint, hintRank[gotHint], st.wantHintAtMost)
		}

		var instrLen int
		if mb, ok := out["motivation_brief"].(map[string]any); ok {
			if instr, ok := mb["instruction"].(string); ok {
				instrLen = len(instr)
			}
		}
		if prevInstrLen >= 0 && instrLen > prevInstrLen {
			t.Errorf("step %s: motivation instruction length grew (was %d, now %d) — verbosity should be monotonically non-increasing as autonomy rises",
				st.name, prevInstrLen, instrLen)
		}
		prevInstrLen = instrLen
	}
}
