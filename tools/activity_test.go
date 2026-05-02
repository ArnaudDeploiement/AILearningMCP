// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
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
