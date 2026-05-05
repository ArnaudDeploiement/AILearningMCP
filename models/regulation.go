// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package models

// Phase identifies which regulation phase a learner is currently in.
// Decided by [2] PhaseController (engine/phase_fsm.go) and consumed by
// [4] ConceptSelector to choose the eligible-concept pool and scoring
// formula. See docs/regulation-design/04-concept-selector.md §4.
//
// Callers must pass an explicit phase — there is no implicit default
// at the SelectConcept boundary, by design (an unrecognised phase
// returns an explicit error rather than silently degrading; cf.
// OQ-4.1). When the orchestrator first sees a domain whose phase is
// NULL in the DB, it bootstraps as PhaseDiagnostic.
type Phase string

const (
	// PhaseInstruction — default learning phase. Eligible pool is the
	// "external fringe": concepts whose prereqs are mastered and whose
	// own mastery is below MasteryBKT(). Score: goal_relevance × (1 -
	// mastery).
	PhaseInstruction Phase = "INSTRUCTION"

	// PhaseDiagnostic — uncertainty-reduction phase. Eligible pool is
	// concepts with non-saturated P(L). Score: BKT info-gain (pure,
	// no goal_relevance multiplier in v1 — cf. OQ-4.2 sub A1).
	PhaseDiagnostic Phase = "DIAGNOSTIC"

	// PhaseMaintenance — review phase. Eligible pool is mastered
	// concepts (PMastery >= MasteryBKT()). Score: (1 - retention) ×
	// goal_relevance.
	PhaseMaintenance Phase = "MAINTENANCE"
)
