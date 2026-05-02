// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"testing"
)

func TestGetMetacognitiveMirror_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetMetacognitiveMirror, "", "get_metacognitive_mirror", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetMetacognitiveMirror_NoPattern(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetMetacognitiveMirror, "L_owner", "get_metacognitive_mirror", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if _, ok := out["mirror"]; !ok {
		t.Fatalf("expected mirror key, got %v", out)
	}
}
