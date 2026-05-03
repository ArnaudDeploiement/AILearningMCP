// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

// Package engine — Open Learner Model graph layer.
//
// OLMGraph extends OLMSnapshot with the full KST graph (nodes + edges) and the
// learner's activity streak. Consumed by the open_cockpit tool's
// structuredContent and by the in-iframe cockpit JS to draw the visual map.

package engine

// EdgeType classifies a prerequisite edge by the state of its endpoints.
type EdgeType string

const (
	// EdgeTraversed: both endpoints are Solid — the learner has crossed it.
	EdgeTraversed EdgeType = "traversed"
	// EdgeActive: edge points into the focus concept — current path of effort.
	EdgeActive EdgeType = "active"
	// EdgeFuture: at least one endpoint is NotStarted/InProgress/Fragile and
	// not the focus — potential future progression.
	EdgeFuture EdgeType = "future"
)

// GraphNode is one concept in the cockpit graph.
type GraphNode struct {
	Concept   string    `json:"concept"`
	State     NodeState `json:"state"`
	PMastery  float64   `json:"p_mastery"`
	Retention float64   `json:"retention"`
	Reps      int       `json:"reps"`
	Lapses    int       `json:"lapses"`
	DaysSince int       `json:"days_since_review"`
}

// GraphEdge is a directed prerequisite edge from -> to (to depends on from).
type GraphEdge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Type EdgeType `json:"type"`
}

// OLMGraph is the structured payload exposed to the cockpit iframe.
// It composes OLMSnapshot (mastery distribution + focus + metacog + KST progress)
// with the per-concept graph data needed to render the visual map.
type OLMGraph struct {
	*OLMSnapshot
	Concepts []GraphNode `json:"concepts"`
	Edges    []GraphEdge `json:"edges"`
	Streak   int         `json:"streak"`
}
