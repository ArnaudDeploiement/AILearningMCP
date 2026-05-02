// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"encoding/json"
	"testing"
)

func TestUpdateLearnerProfile_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerUpdateLearnerProfile, "", "update_learner_profile", map[string]any{"device": "laptop"})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestUpdateLearnerProfile_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	res := callTool(t, deps, registerUpdateLearnerProfile, "L_owner", "update_learner_profile", map[string]any{
		"device":           "laptop",
		"background":       "engineer",
		"learning_style":   "visual",
		"objective":        "deep math",
		"language":         "fr",
		"level":            "intermediate",
		"calibration_bias": 0.15,
		"affect_baseline":  "calm",
		"autonomy_score":   0.6,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["updated"] != true {
		t.Fatalf("expected updated=true, got %v", out)
	}
	if out["fields_changed"].(float64) != 9 {
		t.Fatalf("expected 9 fields_changed, got %v", out["fields_changed"])
	}

	// DB state: profile is persisted.
	learner, err := store.GetLearnerByID("L_owner")
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(learner.ProfileJSON), &p); err != nil {
		t.Fatalf("bad profile JSON: %v", err)
	}
	if p["device"] != "laptop" || p["learning_style"] != "visual" {
		t.Fatalf("unexpected profile: %v", p)
	}
}

func TestUpdateLearnerProfile_PartialUpdatePreservesExisting(t *testing.T) {
	store, deps := setupToolsTest(t)

	// First call: sets device + level.
	res := callTool(t, deps, registerUpdateLearnerProfile, "L_owner", "update_learner_profile", map[string]any{
		"device": "phone",
		"level":  "beginner",
	})
	if res.IsError {
		t.Fatalf("first call: %q", resultText(res))
	}

	// Second call: only updates level — device should be preserved.
	res2 := callTool(t, deps, registerUpdateLearnerProfile, "L_owner", "update_learner_profile", map[string]any{
		"level": "advanced",
	})
	if res2.IsError {
		t.Fatalf("second call: %q", resultText(res2))
	}

	learner, err := store.GetLearnerByID("L_owner")
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(learner.ProfileJSON), &p)
	if p["device"] != "phone" {
		t.Fatalf("device should be preserved, got %v", p["device"])
	}
	if p["level"] != "advanced" {
		t.Fatalf("level should be updated, got %v", p["level"])
	}
}

func TestUpdateLearnerProfile_NoFieldsProvided(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerUpdateLearnerProfile, "L_owner", "update_learner_profile", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["fields_changed"].(float64) != 0 {
		t.Fatalf("expected 0 fields_changed, got %v", out["fields_changed"])
	}
}
