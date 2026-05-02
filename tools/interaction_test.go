// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"

	"tutor-mcp/models"
)

func TestRecordInteraction_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordInteraction, "", "record_interaction", map[string]any{
		"concept":               "x",
		"activity_type":         "RECALL_EXERCISE",
		"success":               true,
		"response_time_seconds": 5.0,
		"confidence":            0.8,
		"notes":                 "",
	})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestRecordInteraction_MissingConcept(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordInteraction, "L_owner", "record_interaction", map[string]any{
		"concept":               "",
		"activity_type":         "RECALL_EXERCISE",
		"success":               true,
		"response_time_seconds": 5.0,
		"confidence":            0.8,
		"notes":                 "",
	})
	if !res.IsError || !strings.Contains(resultText(res), "concept is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestRecordInteraction_HappyPath_Success(t *testing.T) {
	store, deps := setupToolsTest(t)

	res := callTool(t, deps, registerRecordInteraction, "L_owner", "record_interaction", map[string]any{
		"concept":               "derivative",
		"activity_type":         "RECALL_EXERCISE",
		"success":               true,
		"response_time_seconds": 12.0,
		"confidence":            0.9,
		"notes":                 "great",
		"hints_requested":       1,
		"self_initiated":        true,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["updated"] != true {
		t.Fatalf("expected updated=true, got %v", out)
	}
	if _, ok := out["new_mastery"]; !ok {
		t.Fatalf("expected new_mastery key, got %v", out)
	}
	if out["engagement_signal"] != "positive" {
		t.Fatalf("expected positive engagement (success+conf>=0.8), got %v", out["engagement_signal"])
	}

	// DB: interaction created.
	recents, err := store.GetRecentInteractionsByLearner("L_owner", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recents) != 1 {
		t.Fatalf("expected 1 interaction, got %d", len(recents))
	}
	if !recents[0].Success || recents[0].Concept != "derivative" {
		t.Fatalf("unexpected interaction: %+v", recents[0])
	}

	// DB: concept state upserted.
	cs, err := store.GetConceptState("L_owner", "derivative")
	if err != nil {
		t.Fatalf("expected concept state: %v", err)
	}
	if cs.Reps == 0 {
		t.Fatalf("expected reps to be incremented, got %d", cs.Reps)
	}
}

func TestRecordInteraction_FailureDecliningSignal(t *testing.T) {
	_, deps := setupToolsTest(t)

	res := callTool(t, deps, registerRecordInteraction, "L_owner", "record_interaction", map[string]any{
		"concept":               "calc",
		"activity_type":         "RECALL_EXERCISE",
		"success":               false,
		"response_time_seconds": 30.0,
		"confidence":            0.1,
		"notes":                 "",
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["engagement_signal"] != "declining" {
		t.Fatalf("expected declining engagement, got %v", out["engagement_signal"])
	}
}

func TestRecordInteraction_StoresMisconceptionOnFailure(t *testing.T) {
	store, deps := setupToolsTest(t)

	res := callTool(t, deps, registerRecordInteraction, "L_owner", "record_interaction", map[string]any{
		"concept":               "loops",
		"activity_type":         "RECALL_EXERCISE",
		"success":               false,
		"response_time_seconds": 20.0,
		"confidence":            0.4,
		"notes":                 "",
		"misconception_type":    "off_by_one",
		"misconception_detail":  "uses < instead of <=",
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}

	recents, _ := store.GetRecentInteractionsByLearner("L_owner", 5)
	if len(recents) == 0 {
		t.Fatal("no interactions recorded")
	}
	got := recents[0]
	if got.MisconceptionType != "off_by_one" {
		t.Fatalf("misconception not stored: %+v", got)
	}
}

func TestRecordInteraction_MisconceptionIgnoredOnSuccess(t *testing.T) {
	store, deps := setupToolsTest(t)

	res := callTool(t, deps, registerRecordInteraction, "L_owner", "record_interaction", map[string]any{
		"concept":               "loops",
		"activity_type":         "RECALL_EXERCISE",
		"success":               true,
		"response_time_seconds": 5.0,
		"confidence":            0.7,
		"notes":                 "",
		"misconception_type":    "off_by_one",
		"misconception_detail":  "ignored",
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}

	recents, _ := store.GetRecentInteractionsByLearner("L_owner", 5)
	if len(recents) == 0 {
		t.Fatal("no interactions")
	}
	if recents[0].MisconceptionType != "" {
		t.Fatalf("misconception should NOT be stored on success: %+v", recents[0])
	}
}

func TestComputeCognitiveSignals(t *testing.T) {
	// Less than 3 interactions → no signals.
	fatigue, frust := computeCognitiveSignals([]*models.Interaction{
		{Success: true, Confidence: 0.8, ResponseTime: 10},
	})
	if fatigue != "none" || frust != "none" {
		t.Fatalf("expected none/none for tiny session, got %s/%s", fatigue, frust)
	}

	// Build a frustration scenario: consecutive failures + low confidence.
	bad := []*models.Interaction{
		{Success: false, Confidence: 0.1, ResponseTime: 20},
		{Success: false, Confidence: 0.2, ResponseTime: 22},
		{Success: false, Confidence: 0.1, ResponseTime: 25},
		{Success: true, Confidence: 0.5, ResponseTime: 10},
	}
	_, frust2 := computeCognitiveSignals(bad)
	if frust2 == "none" {
		t.Fatalf("expected non-none frustration, got %q", frust2)
	}

	// Build fatigue: poor recent vs solid earlier window.
	long := []*models.Interaction{
		// recent (newest first) — poor
		{Success: false, Confidence: 0.3, ResponseTime: 60},
		{Success: false, Confidence: 0.3, ResponseTime: 50},
		{Success: false, Confidence: 0.3, ResponseTime: 40},
		{Success: true, Confidence: 0.4, ResponseTime: 30},
		{Success: false, Confidence: 0.3, ResponseTime: 30},
		// earlier window — strong
		{Success: true, Confidence: 0.9, ResponseTime: 5},
		{Success: true, Confidence: 0.9, ResponseTime: 5},
		{Success: true, Confidence: 0.9, ResponseTime: 5},
		{Success: true, Confidence: 0.9, ResponseTime: 5},
		{Success: true, Confidence: 0.9, ResponseTime: 5},
	}
	fatigue3, _ := computeCognitiveSignals(long)
	if fatigue3 == "none" {
		t.Fatalf("expected fatigue signal, got %q", fatigue3)
	}
}
