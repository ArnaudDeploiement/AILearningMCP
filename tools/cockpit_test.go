// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"

	"tutor-mcp/models"
)

func TestGetCockpitState_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetCockpitState, "", "get_cockpit_state", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetCockpitState_NoDomain(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{})
	if !res.IsError || !strings.Contains(resultText(res), "aucun domaine configure") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestGetCockpitState_ForeignDomainRejected(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	res := callTool(t, deps, registerGetCockpitState, "L_attacker", "get_cockpit_state", map[string]any{
		"domain_id": d.ID,
	})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestGetCockpitState_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	// Initialize a couple of concept states with mastery.
	cs1 := models.NewConceptState("L_owner", "a")
	cs1.PMastery = 0.95
	_ = store.InsertConceptStateIfNotExists(cs1)
	_ = store.UpsertConceptState(cs1)

	cs2 := models.NewConceptState("L_owner", "b")
	cs2.PMastery = 0.4
	_ = store.InsertConceptStateIfNotExists(cs2)
	_ = store.UpsertConceptState(cs2)

	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)

	if _, ok := out["domains"]; !ok {
		t.Fatalf("expected domains key, got %v", out)
	}
	if _, ok := out["alerts"]; !ok {
		t.Fatalf("expected alerts key, got %v", out)
	}
	if _, ok := out["autonomy_score"]; !ok {
		t.Fatalf("expected autonomy_score, got %v", out)
	}
	if _, ok := out["global_progress"]; !ok {
		t.Fatalf("expected global_progress, got %v", out)
	}
}

func TestGetCockpitState_AllDomains(t *testing.T) {
	store, deps := setupToolsTest(t)
	makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	domains, ok := out["domains"].([]any)
	if !ok || len(domains) != 1 {
		t.Fatalf("expected 1 domain in cockpit, got %v", out["domains"])
	}
}

func TestGetCockpitState_UnknownDomainID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{
		"domain_id": "nope",
	})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}
