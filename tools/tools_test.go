// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"reflect"
	"testing"
)

func TestJsonResult_SetsStructuredContent(t *testing.T) {
	payload := map[string]any{"foo": "bar", "n": float64(42)}
	res, err := jsonResult(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StructuredContent == nil {
		t.Fatal("expected StructuredContent to be non-nil")
	}
	got, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("StructuredContent type = %T, want map[string]any", res.StructuredContent)
	}
	if !reflect.DeepEqual(got, payload) {
		t.Fatalf("StructuredContent = %v, want %v", got, payload)
	}
}

func TestJsonResult_TextContentStillPresent(t *testing.T) {
	res, _ := jsonResult(map[string]any{"k": "v"})
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(res.Content))
	}
}
