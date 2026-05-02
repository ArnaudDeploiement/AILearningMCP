// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"testing"
)

func TestGetPendingAlerts_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetPendingAlerts, "", "get_pending_alerts", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetPendingAlerts_NoDataReturnsEmpty(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetPendingAlerts, "L_owner", "get_pending_alerts", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	alerts, ok := out["alerts"].([]any)
	if !ok {
		t.Fatalf("expected alerts array, got %v", out["alerts"])
	}
	if len(alerts) != 0 {
		t.Fatalf("expected empty alerts list, got %v", alerts)
	}
	if out["has_critical"] != false {
		t.Fatalf("expected has_critical=false, got %v", out["has_critical"])
	}
}

func TestGetPendingAlerts_FilterByDomain(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	res := callTool(t, deps, registerGetPendingAlerts, "L_owner", "get_pending_alerts", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
}
