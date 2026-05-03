// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package engine

import (
	"testing"

	"tutor-mcp/models"
)

func TestNodeClassify(t *testing.T) {
	cases := []struct {
		name string
		cs   *models.ConceptState
		want NodeState
	}{
		{"nil_state", nil, NodeNotStarted},
		{"new_card", &models.ConceptState{CardState: "new", PMastery: 0.0}, NodeNotStarted},
		{"solid", &models.ConceptState{CardState: "review", PMastery: 0.85, Stability: 5.0, ElapsedDays: 1}, NodeSolid},
		{"in_progress", &models.ConceptState{CardState: "review", PMastery: 0.50, Stability: 5.0, ElapsedDays: 1}, NodeInProgress},
		{"fragile_low_mastery", &models.ConceptState{CardState: "review", PMastery: 0.20, Stability: 5.0, ElapsedDays: 1}, NodeFragile},
		{"fragile_low_retention", &models.ConceptState{CardState: "review", PMastery: 0.50, Stability: 1.0, ElapsedDays: 30}, NodeFragile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NodeClassify(tc.cs)
			if got != tc.want {
				t.Errorf("NodeClassify(%+v) = %q, want %q", tc.cs, got, tc.want)
			}
		})
	}
}

func TestMetacogLine_Exported(t *testing.T) {
	// Bias-positive sur-estimation case (>1.5).
	got := MetacogLine(&OLMSnapshot{CalibrationBias: 2.0})
	if got == "" || got[:2] != "Tu" {
		t.Errorf("MetacogLine(over-confident) = %q, want non-empty starting with 'Tu'", got)
	}
	// No signal case.
	got = MetacogLine(&OLMSnapshot{CalibrationBias: 0.0})
	if got != "" {
		t.Errorf("MetacogLine(no signal) = %q, want empty", got)
	}
}

func TestOLMGraph_TypesCompile(t *testing.T) {
	// Compile-time check via field reference: confirms struct shape.
	g := &OLMGraph{
		OLMSnapshot: &OLMSnapshot{DomainID: "d", DomainName: "n"},
		Streak:      3,
		Concepts: []GraphNode{
			{Concept: "a", State: NodeSolid, PMastery: 0.9},
		},
		Edges: []GraphEdge{
			{From: "a", To: "b", Type: EdgeFuture},
		},
	}
	if g.DomainID != "d" || g.Streak != 3 || len(g.Concepts) != 1 || len(g.Edges) != 1 {
		t.Errorf("unexpected shape: %+v", g)
	}
}
