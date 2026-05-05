// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"
	"time"

	"tutor-mcp/algorithms"
	"tutor-mcp/engine"
	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type LearningNegotiationParams struct {
	SessionID        string `json:"session_id" jsonschema:"ID de la session courante"`
	LearnerConcept   string `json:"learner_concept,omitempty" jsonschema:"Concept proposé par l'apprenant (optionnel)"`
	LearnerRationale string `json:"learner_rationale,omitempty" jsonschema:"Justification de l'apprenant (optionnel)"`
	DomainID         string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
}

type tradeoff struct {
	Factor      string  `json:"factor"`
	SystemPlan  string  `json:"system_plan_impact"`
	LearnerPlan string  `json:"learner_plan_impact"`
	Delta       float64 `json:"delta"`
}

func registerLearningNegotiation(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "learning_negotiation",
		Description: "Expose le plan de session avec justifications. L'apprenant peut proposer une alternative — le système accepte ou explique les compromis.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params LearningNegotiationParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("learning_negotiation: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil || domain == nil {
			deps.Logger.Error("learning_negotiation: failed to resolve domain", "err", err, "learner", learnerID)
			r, _ := errorResult("domain not found")
			return r, nil, nil
		}

		states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
		interactions, _ := deps.Store.GetRecentInteractionsByLearner(learnerID, 20)
		sessionStart, _ := deps.Store.GetSessionStart(learnerID)
		sessionInteractions, _ := deps.Store.GetSessionInteractions(learnerID)

		domainConcepts := make(map[string]bool)
		for _, c := range domain.Graph.Concepts {
			domainConcepts[c] = true
		}
		var domainStates []*models.ConceptState
		for _, cs := range states {
			if domainConcepts[cs.Concept] {
				domainStates = append(domainStates, cs)
			}
		}
		var domainInteractions []*models.Interaction
		for _, i := range interactions {
			if domainConcepts[i.Concept] {
				domainInteractions = append(domainInteractions, i)
			}
		}

		alerts := engine.ComputeAlerts(domainStates, domainInteractions, sessionStart)
		mastery := make(map[string]float64)
		for _, cs := range domainStates {
			mastery[cs.Concept] = cs.PMastery
		}
		graph := algorithms.KSTGraph{
			Concepts:      domain.Graph.Concepts,
			Prerequisites: domain.Graph.Prerequisites,
		}
		frontier := algorithms.ComputeFrontier(graph, mastery)

		sessionConcepts := make(map[string]int)
		for _, i := range sessionInteractions {
			if domainConcepts[i.Concept] {
				sessionConcepts[i.Concept]++
			}
		}

		systemActivity := engine.Route(alerts, frontier, domainStates, domainInteractions, sessionConcepts)

		result := map[string]interface{}{
			"system_plan": map[string]interface{}{
				"activity":  systemActivity,
				"rationale": systemActivity.Rationale,
			},
		}

		if params.LearnerConcept != "" {
			var tradeoffs []tradeoff

			systemConcept := systemActivity.Concept
			if systemConcept != "" && systemConcept != params.LearnerConcept {
				systemCS, _ := deps.Store.GetConceptState(learnerID, systemConcept)
				if systemCS != nil && systemCS.LastReview != nil {
					elapsed := int(time.Since(*systemCS.LastReview).Hours() / 24)
					currentRet := algorithms.Retrievability(elapsed, systemCS.Stability)
					futureRet := algorithms.Retrievability(elapsed+1, systemCS.Stability)
					tradeoffs = append(tradeoffs, tradeoff{
						Factor:      "retention",
						SystemPlan:  fmt.Sprintf("reviser %s maintient retention a %.0f%%", systemConcept, currentRet*100),
						LearnerPlan: fmt.Sprintf("reporter %s — retention tombera a %.0f%% demain", systemConcept, futureRet*100),
						Delta:       currentRet - futureRet,
					})
				}
			}

			prereqs := domain.Graph.Prerequisites[params.LearnerConcept]
			unmetPrereqs := 0
			masteryMid := algorithms.MasteryMid()
			for _, p := range prereqs {
				if mastery[p] < masteryMid {
					unmetPrereqs++
				}
			}
			if unmetPrereqs > 0 {
				tradeoffs = append(tradeoffs, tradeoff{
					Factor:      "prerequisites",
					SystemPlan:  "prerequis respectes",
					LearnerPlan: fmt.Sprintf("%d prerequis non maitrises pour %s", unmetPrereqs, params.LearnerConcept),
					Delta:       float64(unmetPrereqs) * 0.2,
				})
			}

			accepted := true
			for _, t := range tradeoffs {
				if t.Delta > 0.15 {
					accepted = false
					break
				}
			}

			acceptedPlan := systemActivity
			if accepted && params.LearnerConcept != "" {
				learnerCS, _ := deps.Store.GetConceptState(learnerID, params.LearnerConcept)
				difficulty := 0.55
				if learnerCS != nil {
					difficulty = learnerCS.Difficulty / 10.0
				}
				acceptedPlan = models.Activity{
					Type:             models.ActivityRecall,
					Concept:          params.LearnerConcept,
					DifficultyTarget: difficulty,
					Format:           "mixed",
					EstimatedMinutes: 15,
					Rationale:        fmt.Sprintf("choix negocie de l'apprenant: %s", params.LearnerRationale),
					PromptForLLM:     fmt.Sprintf("L'apprenant a choisi de travailler sur %s. Genere un exercice adapte.", params.LearnerConcept),
				}
			}

			result["learner_proposal"] = params.LearnerConcept
			result["tradeoffs"] = tradeoffs
			result["accepted"] = accepted
			result["accepted_plan"] = map[string]interface{}{
				"activity":  acceptedPlan,
				"rationale": acceptedPlan.Rationale,
			}
			result["counts_as_self_initiated"] = accepted
		}

		r, _ := jsonResult(result)
		return r, nil, nil
	})
}
