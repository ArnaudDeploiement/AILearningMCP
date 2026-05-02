// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"
)

func TestCalibrationCheck_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerCalibrationCheck, "", "calibration_check", map[string]any{
		"concept_id":        "x",
		"predicted_mastery": 3.0,
	})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestCalibrationCheck_MissingConceptID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerCalibrationCheck, "L_owner", "calibration_check", map[string]any{
		"concept_id":        "",
		"predicted_mastery": 3.0,
	})
	if !res.IsError || !strings.Contains(resultText(res), "concept_id is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestCalibrationCheck_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)

	res := callTool(t, deps, registerCalibrationCheck, "L_owner", "calibration_check", map[string]any{
		"concept_id":        "calc",
		"predicted_mastery": 4.0, // → predicted = 0.75
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	pid, _ := out["prediction_id"].(string)
	if !strings.HasPrefix(pid, "cal_") {
		t.Fatalf("expected prediction_id starting with cal_, got %q", pid)
	}

	rec, err := store.GetCalibrationRecord(pid)
	if err != nil {
		t.Fatalf("GetCalibrationRecord: %v", err)
	}
	if rec.LearnerID != "L_owner" {
		t.Fatalf("expected learner L_owner, got %q", rec.LearnerID)
	}
	if rec.ConceptID != "calc" {
		t.Fatalf("expected concept calc, got %q", rec.ConceptID)
	}
	if rec.Predicted < 0.74 || rec.Predicted > 0.76 {
		t.Fatalf("expected predicted ~0.75, got %v", rec.Predicted)
	}
}

func TestCalibrationCheck_ClampsLowAndHigh(t *testing.T) {
	store, deps := setupToolsTest(t)

	low := callTool(t, deps, registerCalibrationCheck, "L_owner", "calibration_check", map[string]any{
		"concept_id":        "low",
		"predicted_mastery": -100.0, // clamped to 0
	})
	high := callTool(t, deps, registerCalibrationCheck, "L_owner", "calibration_check", map[string]any{
		"concept_id":        "high",
		"predicted_mastery": 100.0, // clamped to 1
	})
	for _, r := range []bool{low.IsError, high.IsError} {
		if r {
			t.Fatalf("clamp test should not error")
		}
	}
	lowID := decodeResult(t, low)["prediction_id"].(string)
	highID := decodeResult(t, high)["prediction_id"].(string)

	lowRec, _ := store.GetCalibrationRecord(lowID)
	highRec, _ := store.GetCalibrationRecord(highID)
	if lowRec.Predicted != 0 {
		t.Fatalf("expected clamped low=0, got %v", lowRec.Predicted)
	}
	if highRec.Predicted != 1 {
		t.Fatalf("expected clamped high=1, got %v", highRec.Predicted)
	}
}

func TestRecordCalibrationResult_MissingPredictionID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordCalibrationResult, "L_owner", "record_calibration_result", map[string]any{
		"prediction_id": "",
		"actual_score":  0.5,
	})
	if !res.IsError || !strings.Contains(resultText(res), "prediction_id is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestRecordCalibrationResult_NotFound(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerRecordCalibrationResult, "L_owner", "record_calibration_result", map[string]any{
		"prediction_id": "cal_unknown",
		"actual_score":  0.5,
	})
	if !res.IsError || !strings.Contains(resultText(res), "prediction not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestGeneratePredictionID_FormatAndUnique(t *testing.T) {
	a := generatePredictionID()
	b := generatePredictionID()
	if !strings.HasPrefix(a, "cal_") || !strings.HasPrefix(b, "cal_") {
		t.Fatalf("expected cal_ prefix, got %q %q", a, b)
	}
	if a == b {
		t.Fatalf("expected unique IDs, got duplicate %q", a)
	}
}
