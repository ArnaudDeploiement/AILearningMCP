// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"
	"time"

	"tutor-mcp/models"
)

func TestRecordSessionClose_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordSessionClose, "", "record_session_close", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestRecordSessionClose_NoDomain(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordSessionClose, "L_owner", "record_session_close", map[string]any{})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestRecordSessionClose_HappyPath_NoIntention(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	// Seed a successful session interaction so RecapBrief shows wins.
	if err := store.CreateInteraction(&models.Interaction{
		LearnerID:    "L_owner",
		Concept:      "a",
		ActivityType: "RECALL_EXERCISE",
		Success:      true,
		Confidence:   0.8,
	}); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, deps, registerRecordSessionClose, "L_owner", "record_session_close", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	recap, ok := out["recap_brief"].(map[string]any)
	if !ok {
		t.Fatalf("expected recap_brief, got %v", out)
	}
	wins, _ := recap["wins"].([]any)
	if len(wins) == 0 || wins[0] != "a" {
		t.Fatalf("expected wins=[a], got %v", wins)
	}
}

func TestRecordSessionClose_PersistsImplementationIntention(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	scheduled := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	res := callTool(t, deps, registerRecordSessionClose, "L_owner", "record_session_close", map[string]any{
		"domain_id": d.ID,
		"implementation_intention": map[string]any{
			"trigger":       "demain matin au cafe",
			"action":        "ferai 1 exercice de derivees",
			"scheduled_for": scheduled,
		},
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}

	// DB state: intention persisted.
	intentions, err := store.GetRecentImplementationIntentions("L_owner", time.Now().UTC().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(intentions) != 1 {
		t.Fatalf("expected 1 intention, got %d", len(intentions))
	}
	if intentions[0].Trigger != "demain matin au cafe" || intentions[0].Action != "ferai 1 exercice de derivees" {
		t.Fatalf("intention not persisted correctly: %+v", intentions[0])
	}
}

func TestRecordSessionClose_EmptyIntentionFieldsSkipped(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerRecordSessionClose, "L_owner", "record_session_close", map[string]any{
		"domain_id": d.ID,
		"implementation_intention": map[string]any{
			"trigger": "x",
			"action":  "", // empty action — should not insert
		},
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}

	intentions, _ := store.GetRecentImplementationIntentions("L_owner", time.Now().UTC().Add(-time.Hour), 10)
	if len(intentions) != 0 {
		t.Fatalf("should not have persisted intention with empty action, got %d", len(intentions))
	}
}

func TestMapKeysHelper(t *testing.T) {
	if got := mapKeys(nil); len(got) != 0 {
		t.Fatalf("expected empty for nil, got %v", got)
	}
	got := mapKeys(map[string]bool{"a": true, "b": true})
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %v", got)
	}
}
