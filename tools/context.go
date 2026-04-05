package tools

import (
	"context"
	"fmt"
	"math"
	"time"

	"learning-runtime/algorithms"

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
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		learner, err := deps.Store.GetLearnerByID(learnerID)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("learner not found: %v", err))
			return r, nil, nil
		}

		// Check domain
		domain, domainErr := resolveDomain(deps.Store, learnerID, params.DomainID)
		needsDomainSetup := domainErr != nil || domain == nil

		states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
		interactions, _ := deps.Store.GetRecentInteractionsByLearner(learnerID, 10)

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

		// List all domains for multi-domain awareness
		allDomains, _ := deps.Store.GetDomainsByLearner(learnerID)
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
		})
		return r, nil, nil
	})
}
