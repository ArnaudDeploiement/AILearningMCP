// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

// Package engine — Open Learner Model layer.
//
// BuildOLMSnapshot computes a transparent snapshot of a learner's state on a
// given (active) domain: mastery distribution buckets, the most actionable
// focus concept, metacognitive signals if active, and KST progress vs the
// personal goal. This snapshot is consumed by the get_olm_snapshot MCP tool
// and by the scheduler's sendOLM dispatch.
//
// All concept-level data is filtered through ActiveDomainConceptSet so
// archived or orphan concepts never leak into the OLM.

package engine

import (
	"fmt"

	"tutor-mcp/algorithms"
	"tutor-mcp/db"
	"tutor-mcp/models"
)

// OLMSnapshot is the structured Open Learner Model returned to the LLM in
// session and rendered by the scheduler when no LLM-authored copy is queued.
// Fields with zero values mean "no signal" — the LLM and the Go fallback
// template both use the empty string / zero count to skip the corresponding
// line.
type OLMSnapshot struct {
	DomainID     string `json:"domain_id"`
	DomainName   string `json:"domain_name"`
	PersonalGoal string `json:"personal_goal,omitempty"`

	// Mastery distribution on ACTIVE concepts (KST threshold = 0.70).
	Solid      int `json:"solid"`
	InProgress int `json:"in_progress"`
	Fragile    int `json:"fragile"`
	NotStarted int `json:"not_started"`

	// Focus — the most actionable concept right now. Empty string if nothing
	// is actionable on this domain.
	FocusConcept string              `json:"focus_concept,omitempty"`
	FocusReason  string              `json:"focus_reason,omitempty"`
	FocusUrgency models.AlertUrgency `json:"focus_urgency,omitempty"`

	// Metacognitive signals — empty string / zero means "no actionable signal".
	AutonomyTrend   string  `json:"autonomy_trend,omitempty"` // "improving" | "stable" | "declining"
	CalibrationBias float64 `json:"calibration_bias"`         // signed; |x|>1.5 → actionable
	AffectTrend     string  `json:"affect_trend,omitempty"`   // "improving" | "stable" | "declining"

	// KST progress toward the personal goal.
	KSTProgress     float64 `json:"kst_progress"` // 0..1
	NextStepConcept string  `json:"next_step_concept,omitempty"`

	// HasActionable: true if anything in this snapshot is worth surfacing.
	// The scheduler skips dispatch when false (silence ≠ panne).
	HasActionable bool `json:"has_actionable"`
}

// BuildOLMSnapshot composes an OLMSnapshot for the given (learner, domain).
// If domainID is empty, the most recently created non-archived domain is used.
// Returns an error if no active domain exists or the requested domain is
// archived.
func BuildOLMSnapshot(store *db.Store, learnerID, domainID string) (*OLMSnapshot, error) {
	domain, err := resolveActiveDomain(store, learnerID, domainID)
	if err != nil {
		return nil, err
	}

	snap := &OLMSnapshot{
		DomainID:     domain.ID,
		DomainName:   domain.Name,
		PersonalGoal: domain.PersonalGoal,
	}

	allStates, err := store.GetConceptStatesByLearner(learnerID)
	if err != nil {
		return nil, fmt.Errorf("olm: get states: %w", err)
	}
	statesByConcept := make(map[string]*models.ConceptState, len(allStates))
	for _, cs := range allStates {
		statesByConcept[cs.Concept] = cs
	}

	for _, c := range domain.Graph.Concepts {
		cs, hasState := statesByConcept[c]
		if !hasState || cs.CardState == "new" {
			snap.NotStarted++
			continue
		}
		if cs.PMastery >= algorithms.KSTMasteryThreshold {
			snap.Solid++
			continue
		}
		if cs.PMastery < 0.30 {
			snap.Fragile++
			continue
		}
		retention := algorithms.Retrievability(cs.ElapsedDays, cs.Stability)
		if retention < 0.50 {
			snap.Fragile++
			continue
		}
		snap.InProgress++
	}

	return snap, nil
}

// resolveActiveDomain returns the domain to use for the OLM. If domainID is
// empty, picks the most recently created non-archived domain. Returns an error
// if the learner has no active domain, or the requested domain is archived.
func resolveActiveDomain(store *db.Store, learnerID, domainID string) (*models.Domain, error) {
	if domainID == "" {
		domains, err := store.GetDomainsByLearner(learnerID, false /*includeArchived*/)
		if err != nil {
			return nil, fmt.Errorf("olm: list domains: %w", err)
		}
		if len(domains) == 0 {
			return nil, fmt.Errorf("olm: no active domain for learner %s", learnerID)
		}
		return domains[0], nil
	}
	d, err := store.GetDomainByID(domainID)
	if err != nil {
		return nil, fmt.Errorf("olm: get domain %s: %w", domainID, err)
	}
	if d == nil || d.LearnerID != learnerID {
		return nil, fmt.Errorf("olm: domain %s not found for learner", domainID)
	}
	if d.Archived {
		return nil, fmt.Errorf("olm: domain %s is archived", domainID)
	}
	return d, nil
}
