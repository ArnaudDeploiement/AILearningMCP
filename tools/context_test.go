// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"testing"

	"tutor-mcp/models"
)

func TestGetLearnerContext_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetLearnerContext, "", "get_learner_context", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetLearnerContext_NeedsDomainSetup(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetLearnerContext, "L_owner", "get_learner_context", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["needs_domain_setup"] != true {
		t.Fatalf("expected needs_domain_setup=true, got %v", out["needs_domain_setup"])
	}
}

func TestGetLearnerContext_WithDomain(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	// Seed a non-new concept with low retention.
	cs := models.NewConceptState("L_owner", "a")
	cs.PMastery = 0.6
	cs.CardState = "review"
	cs.Stability = 1.0
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerGetLearnerContext, "L_owner", "get_learner_context", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["needs_domain_setup"] != false {
		t.Fatalf("expected needs_domain_setup=false, got %v", out["needs_domain_setup"])
	}
	if out["learner_id"] != "L_owner" {
		t.Fatalf("expected learner_id L_owner, got %v", out["learner_id"])
	}
	if out["opening_message"] == nil {
		t.Fatalf("expected opening_message, got %v", out)
	}
	domains, ok := out["domains"].([]any)
	if !ok || len(domains) != 1 {
		t.Fatalf("expected 1 active domain, got %v", out["domains"])
	}
}

func TestBuildProgressNarrative_ReturnsNilWhenNoData(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	learner, err := store.GetLearnerByID("L_owner")
	if err != nil {
		t.Fatal(err)
	}
	got := buildProgressNarrative(deps, "L_owner", learner, d)
	if got != nil {
		t.Fatalf("expected nil narrative when no signals, got %+v", got)
	}
}
