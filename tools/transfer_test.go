// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"

	"tutor-mcp/models"
)

func TestTransferChallenge_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerTransferChallenge, "", "transfer_challenge", map[string]any{"concept_id": "x"})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestTransferChallenge_MissingConcept(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerTransferChallenge, "L_owner", "transfer_challenge", map[string]any{"concept_id": ""})
	if !res.IsError || !strings.Contains(resultText(res), "concept_id is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestTransferChallenge_NotFound(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerTransferChallenge, "L_owner", "transfer_challenge", map[string]any{"concept_id": "ghost"})
	if !res.IsError {
		t.Fatalf("expected error for missing concept state")
	}
}

func TestTransferChallenge_NotMastered(t *testing.T) {
	store, deps := setupToolsTest(t)
	cs := models.NewConceptState("L_owner", "calc")
	cs.PMastery = 0.3
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerTransferChallenge, "L_owner", "transfer_challenge", map[string]any{"concept_id": "calc"})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["eligible"] != false {
		t.Fatalf("expected eligible=false, got %v", out)
	}
}

func TestTransferChallenge_DefaultContextType(t *testing.T) {
	store, deps := setupToolsTest(t)
	cs := models.NewConceptState("L_owner", "calc")
	cs.PMastery = 0.95
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerTransferChallenge, "L_owner", "transfer_challenge", map[string]any{"concept_id": "calc"})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["context_type"] != "real_world" {
		t.Fatalf("expected default context_type=real_world, got %v", out["context_type"])
	}
}

func TestTransferChallenge_CustomContextType(t *testing.T) {
	store, deps := setupToolsTest(t)
	cs := models.NewConceptState("L_owner", "calc")
	cs.PMastery = 0.95
	_ = store.InsertConceptStateIfNotExists(cs)
	_ = store.UpsertConceptState(cs)

	res := callTool(t, deps, registerTransferChallenge, "L_owner", "transfer_challenge", map[string]any{
		"concept_id":   "calc",
		"context_type": "interview",
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["context_type"] != "interview" {
		t.Fatalf("expected interview, got %v", out["context_type"])
	}
}

func TestRecordTransferResult_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordTransferResult, "", "record_transfer_result", map[string]any{
		"concept_id":   "x",
		"context_type": "real_world",
		"score":        0.5,
	})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestRecordTransferResult_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordTransferResult, "L_owner", "record_transfer_result", map[string]any{
		"concept_id":   "calc",
		"context_type": "real_world",
		"score":        0.7,
		"session_id":   "s1",
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["recorded"] != true {
		t.Fatalf("expected recorded=true, got %v", out)
	}
	if out["transfer_score"].(float64) != 0.7 {
		t.Fatalf("expected score=0.7, got %v", out["transfer_score"])
	}
	if out["blocked"] != false {
		t.Fatalf("expected blocked=false for 0.7, got %v", out["blocked"])
	}

	// DB state — record persisted.
	scores, err := store.GetTransferScores("L_owner", "calc")
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 || scores[0].Score != 0.7 {
		t.Fatalf("expected single transfer record with score 0.7, got %+v", scores)
	}
}

func TestRecordTransferResult_BlockedFlag(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordTransferResult, "L_owner", "record_transfer_result", map[string]any{
		"concept_id":   "calc",
		"context_type": "real_world",
		"score":        0.3,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["blocked"] != true {
		t.Fatalf("expected blocked=true for low score, got %v", out["blocked"])
	}
}
