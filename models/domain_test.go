// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package models

import (
	"sort"
	"testing"
)

// ─── ParseGoalRelevance ────────────────────────────────────────────────────

func TestParseGoalRelevance_EmptyJSONReturnsNil(t *testing.T) {
	d := &Domain{ID: "d1", GoalRelevanceJSON: ""}
	if got := d.ParseGoalRelevance(); got != nil {
		t.Errorf("empty JSON: want nil, got %+v", got)
	}
}

func TestParseGoalRelevance_MalformedJSONReturnsNilAndWarns(t *testing.T) {
	// The "WARN-on-corruption" path: malformed JSON must not panic, must
	// return nil, and must log a WARN. We don't assert the WARN itself
	// (would couple the test to slog.Default), only that the function
	// degrades silently.
	d := &Domain{ID: "d1", GoalRelevanceJSON: "{not_valid_json"}
	if got := d.ParseGoalRelevance(); got != nil {
		t.Errorf("malformed JSON: want nil, got %+v", got)
	}
}

func TestParseGoalRelevance_ValidStructureRoundTrips(t *testing.T) {
	// A full structured payload with relevance map + for_graph_version.
	json := `{"for_graph_version":2,"relevance":{"A":0.9,"B":0.4},"set_at":"2026-01-01T00:00:00Z"}`
	d := &Domain{ID: "d1", GoalRelevanceJSON: json}
	gr := d.ParseGoalRelevance()
	if gr == nil {
		t.Fatal("expected non-nil GoalRelevance")
	}
	if gr.ForGraphVersion != 2 {
		t.Errorf("ForGraphVersion: want 2, got %d", gr.ForGraphVersion)
	}
	if gr.Relevance["A"] != 0.9 || gr.Relevance["B"] != 0.4 {
		t.Errorf("Relevance map: %+v", gr.Relevance)
	}
}

func TestParseGoalRelevance_EmptyJSONObject(t *testing.T) {
	// Edge case: the JSON is structurally valid but has no fields. Should
	// still parse to a zero-value GoalRelevance (not nil).
	d := &Domain{ID: "d1", GoalRelevanceJSON: "{}"}
	gr := d.ParseGoalRelevance()
	if gr == nil {
		t.Fatal("expected non-nil GoalRelevance for {}")
	}
	if gr.ForGraphVersion != 0 {
		t.Errorf("ForGraphVersion: want 0, got %d", gr.ForGraphVersion)
	}
	if len(gr.Relevance) != 0 {
		t.Errorf("Relevance: want empty, got %+v", gr.Relevance)
	}
}

// ─── IsGoalRelevanceStale ──────────────────────────────────────────────────

func TestIsGoalRelevanceStale_NilVectorAndZeroGraphVersion(t *testing.T) {
	// No JSON, GraphVersion=0 → not stale (legacy domain, never had a
	// relevance vector and graph_version isn't tracked yet).
	d := &Domain{ID: "d1", GoalRelevanceJSON: "", GraphVersion: 0}
	if d.IsGoalRelevanceStale() {
		t.Error("nil + GraphVersion=0: want NOT stale")
	}
}

func TestIsGoalRelevanceStale_NilVectorWithGraphVersion(t *testing.T) {
	// No JSON but GraphVersion>0 → stale (the graph exists; the vector
	// was never set against it).
	d := &Domain{ID: "d1", GoalRelevanceJSON: "", GraphVersion: 1}
	if !d.IsGoalRelevanceStale() {
		t.Error("nil + GraphVersion=1: want stale")
	}
}

func TestIsGoalRelevanceStale_VersionMatch(t *testing.T) {
	d := &Domain{
		ID:                "d1",
		GoalRelevanceJSON: `{"for_graph_version":2,"relevance":{"A":0.5}}`,
		GraphVersion:      2,
	}
	if d.IsGoalRelevanceStale() {
		t.Error("for_graph_version == GraphVersion: want NOT stale")
	}
}

func TestIsGoalRelevanceStale_VersionMismatch(t *testing.T) {
	d := &Domain{
		ID:                "d1",
		GoalRelevanceJSON: `{"for_graph_version":1,"relevance":{"A":0.5}}`,
		GraphVersion:      2,
	}
	if !d.IsGoalRelevanceStale() {
		t.Error("for_graph_version < GraphVersion: want stale")
	}
}

func TestIsGoalRelevanceStale_MalformedJSONFallsBackToNilBranch(t *testing.T) {
	// Malformed JSON ⇒ ParseGoalRelevance returns nil ⇒ stale-when-
	// GraphVersion>0 branch is taken.
	d := &Domain{ID: "d1", GoalRelevanceJSON: "{garbage", GraphVersion: 1}
	if !d.IsGoalRelevanceStale() {
		t.Error("malformed JSON + GraphVersion=1: want stale")
	}
}

// ─── UncoveredConcepts ─────────────────────────────────────────────────────

func TestUncoveredConcepts_FullCoverage(t *testing.T) {
	d := &Domain{
		ID:                "d1",
		Graph:             KnowledgeSpace{Concepts: []string{"A", "B"}},
		GoalRelevanceJSON: `{"for_graph_version":1,"relevance":{"A":0.9,"B":0.4}}`,
	}
	if got := d.UncoveredConcepts(); len(got) != 0 {
		t.Errorf("full coverage: want empty, got %v", got)
	}
}

func TestUncoveredConcepts_PartialCoverage(t *testing.T) {
	d := &Domain{
		ID:                "d1",
		Graph:             KnowledgeSpace{Concepts: []string{"A", "B", "C"}},
		GoalRelevanceJSON: `{"for_graph_version":1,"relevance":{"A":0.9}}`,
	}
	got := d.UncoveredConcepts()
	sort.Strings(got)
	want := []string{"B", "C"}
	if len(got) != len(want) {
		t.Fatalf("len: want %d, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestUncoveredConcepts_EmptyVectorAllUncovered(t *testing.T) {
	// No vector at all ⇒ every concept in Graph is uncovered.
	d := &Domain{
		ID:                "d1",
		Graph:             KnowledgeSpace{Concepts: []string{"A", "B"}},
		GoalRelevanceJSON: "",
	}
	got := d.UncoveredConcepts()
	sort.Strings(got)
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("empty vector: want [A B], got %v", got)
	}
}

func TestUncoveredConcepts_EmptyGraph(t *testing.T) {
	d := &Domain{
		ID:                "d1",
		Graph:             KnowledgeSpace{Concepts: nil},
		GoalRelevanceJSON: `{"for_graph_version":1,"relevance":{"A":0.9}}`,
	}
	if got := d.UncoveredConcepts(); len(got) != 0 {
		t.Errorf("empty graph: want empty, got %v", got)
	}
}

func TestUncoveredConcepts_MalformedJSONTreatsAllAsUncovered(t *testing.T) {
	// Corrupt JSON ⇒ Parse returns nil ⇒ no concept is "covered" ⇒ every
	// concept of the graph is reported as uncovered.
	d := &Domain{
		ID:                "d1",
		Graph:             KnowledgeSpace{Concepts: []string{"A"}},
		GoalRelevanceJSON: "{not_valid",
	}
	got := d.UncoveredConcepts()
	if len(got) != 1 || got[0] != "A" {
		t.Errorf("malformed JSON: want [A], got %v", got)
	}
}
