// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

// Package engine — Open Learner Model graph layer.
//
// OLMGraph extends OLMSnapshot with the full KST graph (nodes + edges) and the
// learner's activity streak. Consumed by the open_cockpit tool's
// structuredContent and by the in-iframe cockpit JS to draw the visual map.

package engine

import (
	"fmt"

	"tutor-mcp/algorithms"
	"tutor-mcp/db"
	"tutor-mcp/models"
)

// BuildOLMGraph builds an OLMGraph for the given (learner, domain). If domainID
// is empty, the most recently created non-archived domain is used.
//
// It calls BuildOLMSnapshot for the mastery distribution + focus + metacog +
// KST progress, then enriches with per-concept ConceptState data and classifies
// each prerequisite edge.
func BuildOLMGraph(store *db.Store, learnerID, domainID string) (*OLMGraph, error) {
	snap, err := BuildOLMSnapshot(store, learnerID, domainID)
	if err != nil {
		return nil, err
	}
	domain, err := resolveActiveDomain(store, learnerID, snap.DomainID)
	if err != nil {
		return nil, fmt.Errorf("olm graph: resolve domain: %w", err)
	}

	// Pull all states once (graph stays under O(N) for the learner).
	allStates, err := store.GetConceptStatesByLearner(learnerID)
	if err != nil {
		return nil, fmt.Errorf("olm graph: get states: %w", err)
	}
	stateByConcept := make(map[string]*models.ConceptState, len(allStates))
	for _, cs := range allStates {
		stateByConcept[cs.Concept] = cs
	}

	// Build nodes — focus state overrides whatever NodeClassify returns.
	nodes := make([]GraphNode, 0, len(domain.Graph.Concepts))
	for _, concept := range domain.Graph.Concepts {
		cs := stateByConcept[concept]
		state := NodeClassify(cs)
		if concept == snap.FocusConcept {
			state = NodeFocus
		}
		n := GraphNode{Concept: concept, State: state}
		if cs != nil {
			n.PMastery = cs.PMastery
			n.Retention = algorithms.Retrievability(cs.ElapsedDays, cs.Stability)
			n.Reps = cs.Reps
			n.Lapses = cs.Lapses
			n.DaysSince = cs.ElapsedDays
		}
		nodes = append(nodes, n)
	}

	// Build edges — classification per edge:
	//   target == focus  → active
	//   both endpoints are NodeSolid → traversed
	//   otherwise → future
	stateByName := make(map[string]NodeState, len(nodes))
	for _, n := range nodes {
		stateByName[n.Concept] = n.State
	}
	var edges []GraphEdge
	for to, prereqs := range domain.Graph.Prerequisites {
		for _, from := range prereqs {
			et := EdgeFuture
			if to == snap.FocusConcept {
				et = EdgeActive
			} else if stateByName[from] == NodeSolid && stateByName[to] == NodeSolid {
				et = EdgeTraversed
			}
			edges = append(edges, GraphEdge{From: from, To: to, Type: et})
		}
	}

	// Streak enriches the UI only — DB error means zero, which is safe.
	streak, _ := store.GetActivityStreak(learnerID)

	// Available domains for the cockpit's selector. Best-effort: a DB error
	// here just hides the selector, the rest of the cockpit stays usable.
	var availableDomains []DomainRef
	if domains, err := store.GetDomainsByLearner(learnerID, false /*includeArchived*/); err == nil {
		availableDomains = make([]DomainRef, 0, len(domains))
		for _, d := range domains {
			availableDomains = append(availableDomains, DomainRef{ID: d.ID, Name: d.Name})
		}
	}

	return &OLMGraph{
		OLMSnapshot:      snap,
		Concepts:         nodes,
		Edges:            edges,
		Streak:           streak,
		AvailableDomains: availableDomains,
	}, nil
}

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

// DomainRef is one entry in OLMGraph.AvailableDomains — the cockpit's domain
// selector reads this list to let the learner switch between active domains
// without re-running open_cockpit.
type DomainRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OLMGraph is the structured payload exposed to the cockpit iframe.
// It composes OLMSnapshot (mastery distribution + focus + metacog + KST progress)
// with the per-concept graph data needed to render the visual map.
type OLMGraph struct {
	*OLMSnapshot
	Concepts         []GraphNode `json:"concepts"`
	Edges            []GraphEdge `json:"edges"`
	Streak           int         `json:"streak"`
	AvailableDomains []DomainRef `json:"available_domains"`
}
