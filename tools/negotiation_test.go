// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"

	"tutor-mcp/models"
)

func TestLearningNegotiation_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerLearningNegotiation, "", "learning_negotiation", map[string]any{
		"session_id": "s1",
	})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestLearningNegotiation_NoDomain(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerLearningNegotiation, "L_owner", "learning_negotiation", map[string]any{
		"session_id": "s1",
	})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestLearningNegotiation_SystemPlanOnly(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerLearningNegotiation, "L_owner", "learning_negotiation", map[string]any{
		"session_id": "s1",
		"domain_id":  d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if _, ok := out["system_plan"]; !ok {
		t.Fatalf("expected system_plan, got %v", out)
	}
	if _, ok := out["learner_proposal"]; ok {
		t.Fatalf("learner_proposal should not be present, got %v", out["learner_proposal"])
	}
}

func TestLearningNegotiation_LearnerProposalAccepted(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	// Seed concept state for a (learner's choice).
	cs := models.NewConceptState("L_owner", "a")
	cs.PMastery = 0.5
	cs.Difficulty = 5.0
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerLearningNegotiation, "L_owner", "learning_negotiation", map[string]any{
		"session_id":        "s1",
		"learner_concept":   "a",
		"learner_rationale": "envie",
		"domain_id":         d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["learner_proposal"] != "a" {
		t.Fatalf("expected learner_proposal=a, got %v", out["learner_proposal"])
	}
	if _, ok := out["accepted"]; !ok {
		t.Fatalf("expected accepted key, got %v", out)
	}
	if _, ok := out["accepted_plan"]; !ok {
		t.Fatalf("expected accepted_plan key, got %v", out)
	}
	if _, ok := out["counts_as_self_initiated"]; !ok {
		t.Fatalf("expected counts_as_self_initiated key, got %v", out)
	}
}
