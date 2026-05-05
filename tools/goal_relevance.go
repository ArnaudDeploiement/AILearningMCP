// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

// Package tools — set_goal_relevance / get_goal_relevance handlers.
//
// Component [1] of the regulation pipeline. The runtime never produces the
// relevance vector itself: the LLM decomposes the learner's personal_goal
// against the domain's concept list and writes the result via
// set_goal_relevance. Read-only observation via get_goal_relevance lets the
// operator see what the LLM produced before [4] ConceptSelector consumes
// it. See docs/regulation-design/01-goal-decomposer.md.
package tools

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// regulationGoalEnabled gates registration and execution of both
// set_goal_relevance and get_goal_relevance. Default-on, opt-out via
// REGULATION_GOAL=off — same convention as REGULATION_THRESHOLD.
func regulationGoalEnabled() bool {
	return os.Getenv("REGULATION_GOAL") != "off"
}

const (
	maxGoalRelevanceEntries  = 500 // mirrors maxConceptsPerCall
	maxGoalRelevanceConceptL = 200 // mirrors maxConceptNameLen
)

// ─── set_goal_relevance ──────────────────────────────────────────────────────

type SetGoalRelevanceParams struct {
	DomainID  string             `json:"domain_id,omitempty" jsonschema:"ID du domaine cible (optionnel, dernier domaine actif si absent)"`
	Relevance map[string]float64 `json:"relevance" jsonschema:"Map concept_id -> score [0,1]. Concepts inconnus produisent une erreur explicite. Sémantique incrémentale : seuls les concepts fournis sont mis à jour."`
}

func registerSetGoalRelevance(server *mcp.Server, deps *Deps) {
	if !regulationGoalEnabled() {
		return
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "set_goal_relevance",
		Description: "Décompose le personal_goal du learner contre les concepts du domaine. " +
			"Pour chaque concept, fournis un score dans [0,1] : 1.0 = central au goal, " +
			"0.0 = orthogonal. Sémantique incrémentale (merge) : seuls les concepts fournis " +
			"sont mis à jour, les autres conservent leur score précédent. Concept inconnu " +
			"(absent du graph) renvoie une erreur explicite.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params SetGoalRelevanceParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("set_goal_relevance: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if len(params.Relevance) == 0 {
			r, _ := errorResult("relevance map must be non-empty")
			return r, nil, nil
		}
		if len(params.Relevance) > maxGoalRelevanceEntries {
			r, _ := errorResult(fmt.Sprintf("too many entries: %d (max %d)", len(params.Relevance), maxGoalRelevanceEntries))
			return r, nil, nil
		}
		for k := range params.Relevance {
			if k == "" {
				r, _ := errorResult("empty concept id in relevance map")
				return r, nil, nil
			}
			if len(k) > maxGoalRelevanceConceptL {
				r, _ := errorResult(fmt.Sprintf("concept id too long: %q (max %d chars)", k, maxGoalRelevanceConceptL))
				return r, nil, nil
			}
		}

		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil || domain == nil {
			deps.Logger.Error("set_goal_relevance: domain not found", "err", err, "learner", learnerID)
			r, _ := errorResult("domain not found — call init_domain first")
			return r, nil, nil
		}

		// OQ-1.4 strictness: every supplied concept MUST be in the graph.
		// First unknown breaks the operation explicitly. No silent ignore.
		known := make(map[string]bool, len(domain.Graph.Concepts))
		for _, c := range domain.Graph.Concepts {
			known[c] = true
		}
		for k := range params.Relevance {
			if !known[k] {
				r, _ := errorResult(fmt.Sprintf("unknown concept %q — not present in domain %q (call get_learner_context to see the concept list)", k, domain.ID))
				return r, nil, nil
			}
		}

		// Clamp to [0,1] / reject NaN. Count clamps for observability.
		clamped := 0
		for k, v := range params.Relevance {
			if math.IsNaN(v) {
				r, _ := errorResult(fmt.Sprintf("NaN score for concept %q", k))
				return r, nil, nil
			}
			if v < 0 {
				params.Relevance[k] = 0
				clamped++
			} else if v > 1 {
				params.Relevance[k] = 1
				clamped++
			}
		}

		merged, err := deps.Store.MergeDomainGoalRelevance(domain.ID, params.Relevance)
		if err != nil {
			deps.Logger.Error("set_goal_relevance: merge failed", "err", err, "domain", domain.ID)
			r, _ := errorResult(fmt.Sprintf("persist failed: %v", err))
			return r, nil, nil
		}

		// Compute uncovered against the graph captured at read time. If
		// add_concepts ran between read and write, uncovered may include
		// the freshly added concepts — that is the correct stale-after-set
		// signal.
		fresh, _ := deps.Store.GetDomainByID(domain.ID)
		var uncovered []string
		var staleAfterSet bool
		if fresh != nil {
			uncovered = fresh.UncoveredConcepts()
			staleAfterSet = fresh.GraphVersion > merged.ForGraphVersion
		}

		r, _ := jsonResult(map[string]any{
			"domain_id":              domain.ID,
			"for_graph_version":      merged.ForGraphVersion,
			"concepts_updated":       len(params.Relevance),
			"concepts_clamped":       clamped,
			"covered_concepts_count": len(merged.Relevance),
			"all_concepts_count":     len(domain.Graph.Concepts),
			"uncovered_concepts":     uncovered,
			"stale_after_set":        staleAfterSet,
		})
		return r, nil, nil
	})
}

// ─── get_goal_relevance ──────────────────────────────────────────────────────

type GetGoalRelevanceParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
}

func registerGetGoalRelevance(server *mcp.Server, deps *Deps) {
	if !regulationGoalEnabled() {
		return
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_goal_relevance",
		Description: "Lit le vecteur de pertinence stocké pour un domaine et la liste des " +
			"concepts encore sans relevance. Outil d'observation : utiliser pour décider s'il " +
			"faut compléter avec set_goal_relevance (ex: après add_concepts).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetGoalRelevanceParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("get_goal_relevance: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil || domain == nil {
			deps.Logger.Error("get_goal_relevance: domain not found", "err", err, "learner", learnerID)
			r, _ := errorResult("domain not found — call init_domain first")
			return r, nil, nil
		}

		gr := domain.ParseGoalRelevance()
		var relevance map[string]float64
		var setAt any = nil
		forGraphVersion := 0
		if gr != nil {
			relevance = gr.Relevance
			forGraphVersion = gr.ForGraphVersion
			setAt = gr.SetAt
		}
		uncovered := domain.UncoveredConcepts()
		coveredCount := 0
		if relevance != nil {
			coveredCount = len(relevance)
		}

		payload := map[string]any{
			"domain_id":              domain.ID,
			"graph_version":          domain.GraphVersion,
			"for_graph_version":      forGraphVersion,
			"stale":                  domain.IsGoalRelevanceStale(),
			"all_concepts_count":     len(domain.Graph.Concepts),
			"covered_concepts_count": coveredCount,
			"uncovered_concepts":     uncovered,
			"set_at":                 setAt,
		}
		// Emit relevance only when non-empty so a fresh-uninstrumented
		// domain doesn't return a misleading "{}".
		if len(relevance) > 0 {
			payload["relevance"] = relevance
		}
		r, _ := jsonResult(payload)
		return r, nil, nil
	})
}
