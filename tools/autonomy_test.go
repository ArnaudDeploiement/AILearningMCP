// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"testing"
)

func TestGetAutonomyMetrics_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetAutonomyMetrics, "", "get_autonomy_metrics", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetAutonomyMetrics_HappyPath(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetAutonomyMetrics, "L_owner", "get_autonomy_metrics", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if _, ok := out["score"]; !ok {
		t.Fatalf("expected score in result, got %v", out)
	}
	if _, ok := out["trend"]; !ok {
		t.Fatalf("expected trend in result, got %v", out)
	}
}
