// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"

	"tutor-mcp/models"
)

func TestRecordAffect_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordAffect, "", "record_affect", map[string]any{"session_id": "s1"})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestRecordAffect_MissingSessionID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordAffect, "L_owner", "record_affect", map[string]any{"session_id": ""})
	if !res.IsError || !strings.Contains(resultText(res), "session_id is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestRecordAffect_StartOfSession(t *testing.T) {
	store, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordAffect, "L_owner", "record_affect", map[string]any{
		"session_id": "s1",
		"energy":     3,
		"confidence": 3,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["affect_state"] == nil {
		t.Fatalf("expected affect_state in response")
	}

	saved, err := store.GetAffectBySession("L_owner", "s1")
	if err != nil || saved == nil {
		t.Fatalf("expected saved affect: %v", err)
	}
	if saved.Energy != 3 || saved.SubjectConfidence != 3 {
		t.Fatalf("unexpected saved affect: %+v", saved)
	}
}

func TestRecordAffect_LowConfidenceTriggersScaffolding(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordAffect, "L_owner", "record_affect", map[string]any{
		"session_id": "s2",
		"energy":     3,
		"confidence": 1,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["tutor_mode_override"] != "scaffolding" {
		t.Fatalf("expected scaffolding override, got %v", out["tutor_mode_override"])
	}
}

func TestRecordAffect_LowEnergyTriggersLighter(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordAffect, "L_owner", "record_affect", map[string]any{
		"session_id": "s3",
		"energy":     1,
		"confidence": 3,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["tutor_mode_override"] != "lighter" {
		t.Fatalf("expected lighter override, got %v", out["tutor_mode_override"])
	}
}

func TestRecordAffect_EndOfSessionAutonomyAndDelta(t *testing.T) {
	store, deps := setupToolsTest(t)

	// Seed with a successful interaction so calibration delta is computed.
	if err := store.CreateInteraction(&models.Interaction{
		LearnerID:    "L_owner",
		Concept:      "x",
		ActivityType: "RECALL_EXERCISE",
		Success:      true,
		Confidence:   0.7,
	}); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, deps, registerRecordAffect, "L_owner", "record_affect", map[string]any{
		"session_id":           "s_end",
		"satisfaction":         3,
		"perceived_difficulty": 2,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if _, ok := out["calibration_bias_delta"]; !ok {
		t.Fatalf("expected calibration_bias_delta, got %v", out)
	}
	if _, ok := out["autonomy_score"]; !ok {
		t.Fatalf("expected autonomy_score, got %v", out)
	}

	// DB: autonomy persisted on the affect row.
	saved, err := store.GetAffectBySession("L_owner", "s_end")
	if err != nil {
		t.Fatal(err)
	}
	if saved.PerceivedDifficulty != 2 || saved.Satisfaction != 3 {
		t.Fatalf("affect not saved: %+v", saved)
	}
}
