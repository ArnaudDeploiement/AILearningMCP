package tools

import (
	"context"
	"fmt"
	"math"
	"time"

	"learning-runtime/algorithms"
	"learning-runtime/engine"
	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetLearnerContextParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, utilise le dernier domaine si absent)"`
}

func registerGetLearnerContext(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_learner_context",
		Description: "Recupere le contexte complet de l'apprenant pour le debut de session.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetLearnerContextParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("get_learner_context: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		learner, err := deps.Store.GetLearnerByID(learnerID)
		if err != nil {
			deps.Logger.Error("get_learner_context: failed to get learner", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("learner not found: %v", err))
			return r, nil, nil
		}

		// Check domain
		domain, domainErr := resolveDomain(deps.Store, learnerID, params.DomainID)
		needsDomainSetup := domainErr != nil || domain == nil

		states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
		interactions, _ := deps.Store.GetRecentInteractionsByLearner(learnerID, 10)

		// Filter out orphan states/interactions left over from deleted or
		// archived domains — only surface concepts that still belong to an
		// active domain. Without this, priority_concept and opening_message
		// can reference ghost concepts (see bug report from cosmos client).
		activeConcepts, _ := deps.Store.ActiveDomainConceptSet(learnerID)
		states = filterStatesByConcepts(states, activeConcepts)
		interactions = filterInteractionsByConcepts(interactions, activeConcepts)

		// Compute day number since account creation (day 1 = creation day)
		dayNumber := int(math.Floor(time.Since(learner.CreatedAt).Hours()/24)) + 1

		// Last session info
		lastSessionInfo := "premiere session"
		if !learner.LastActive.IsZero() {
			hoursSince := time.Since(learner.LastActive).Hours()
			if hoursSince < 24 {
				lastSessionInfo = fmt.Sprintf("derniere session il y a %.0fh", hoursSince)
			} else {
				lastSessionInfo = fmt.Sprintf("derniere session il y a %d jours", int(hoursSince/24))
			}
		}

		// Today's priority: concept with lowest retention
		var priorityConcept string
		var priorityRetention float64 = 1.0
		for _, cs := range states {
			if cs.CardState == "new" {
				continue
			}
			elapsed := cs.ElapsedDays
			if cs.LastReview != nil {
				elapsed = int(time.Since(*cs.LastReview).Hours() / 24)
			}
			ret := algorithms.Retrievability(elapsed, cs.Stability)
			if ret < priorityRetention {
				priorityRetention = ret
				priorityConcept = cs.Concept
			}
		}

		// Build opening message
		openingMessage := fmt.Sprintf("Jour %d", dayNumber)
		if learner.Objective != "" {
			openingMessage += fmt.Sprintf(" · Objectif: %s", learner.Objective)
		}
		openingMessage += fmt.Sprintf(" · %s", lastSessionInfo)
		if priorityConcept != "" {
			openingMessage += fmt.Sprintf(" · Priorite: %s (retention %.0f%%)", priorityConcept, priorityRetention*100)
		}

		// List active domains for multi-domain awareness
		allDomains, _ := deps.Store.GetDomainsByLearner(learnerID, false)
		var domainList []map[string]interface{}
		for _, d := range allDomains {
			domainList = append(domainList, map[string]interface{}{
				"domain_id":     d.ID,
				"name":          d.Name,
				"concept_count": len(d.Graph.Concepts),
			})
		}
		if domainList == nil {
			domainList = []map[string]interface{}{}
		}

		// List archived domains so Claude knows they exist
		archivedDomains, _ := deps.Store.GetDomainsByLearner(learnerID, true)
		var archivedList []map[string]interface{}
		for _, d := range archivedDomains {
			if d.Archived {
				archivedList = append(archivedList, map[string]interface{}{
					"domain_id": d.ID,
					"name":      d.Name,
				})
			}
		}
		if archivedList == nil {
			archivedList = []map[string]interface{}{}
		}

		// Progress narrative — open learner model surfaced at session start.
		var narrative *models.ProgressNarrative
		if !needsDomainSetup && domain != nil {
			narrative = buildProgressNarrative(deps, learnerID, learner, domain)
		}

		r, _ := jsonResult(map[string]interface{}{
			"learner_id":         learnerID,
			"objective":          learner.Objective,
			"day_number":         dayNumber,
			"last_session":       lastSessionInfo,
			"concepts_count":     len(states),
			"interactions_today": len(interactions),
			"needs_domain_setup": needsDomainSetup,
			"opening_message":    openingMessage,
			"priority_concept":   priorityConcept,
			"priority_retention": priorityRetention,
			"domains":            domainList,
			"archived_domains":   archivedList,
			"progress_narrative": narrative,
		})
		return r, nil, nil
	})
}

// buildProgressNarrative composes the session-opening OLM narrative signals. Returns
// nil if there's nothing meaningful to narrate (e.g., zero interactions so far).
func buildProgressNarrative(deps *Deps, learnerID string, learner *models.Learner, domain *models.Domain) *models.ProgressNarrative {
	window := 30 * 24 * time.Hour
	since := time.Now().UTC().Add(-window)

	deltas, _ := deps.Store.ConceptMasteryDelta(learnerID, domain.Graph.Concepts, since, 3)
	streak, _ := deps.Store.CountLearnerSessionStreak(learnerID)
	milestones, _ := deps.Store.MilestonesInWindow(learnerID, domain.Graph.Concepts, time.Now().UTC().Add(-7*24*time.Hour))

	trend := "stable"
	affects, _ := deps.Store.GetRecentAffectStates(learnerID, 5)
	if len(affects) >= 3 {
		var scores []float64
		for _, a := range affects {
			scores = append(scores, a.AutonomyScore)
		}
		trend = engine.ComputeAutonomyTrendExported(scores)
	}

	dormancy := false
	if !learner.LastActive.IsZero() && time.Since(learner.LastActive) > 24*time.Hour {
		dormancy = true
	}

	// Only return a narrative if there's something to say.
	if len(deltas) == 0 && streak == 0 && len(milestones) == 0 && !dormancy {
		return nil
	}

	return &models.ProgressNarrative{
		MasteryTrajectory:  deltas,
		SessionStreak:      streak,
		AutonomyTrend:      trend,
		MilestonesThisWeek: milestones,
		DormancyImminent:   dormancy,
		Instruction:        "Raconte la trajectoire en 1-2 phrases, pas une liste. Si dormancy_imminent est vrai, formule une reprise accueillante sans reproche.",
	}
}
