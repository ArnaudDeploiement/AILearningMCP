package tools

import (
	"context"

	"learning-runtime/engine"
	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetPendingAlertsParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, utilise le dernier domaine si absent)"`
}

func registerGetPendingAlerts(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_pending_alerts",
		Description: "Recupere les alertes en attente pour l'apprenant. Appeler en premier a chaque reponse.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetPendingAlertsParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
		interactions, _ := deps.Store.GetRecentInteractionsByLearner(learnerID, 20)
		sessionStart, _ := deps.Store.GetSessionStart(learnerID)

		// Filter to domain concepts if domain specified
		domain, domainErr := resolveDomain(deps.Store, learnerID, params.DomainID)
		if domainErr == nil && domain != nil {
			domainConcepts := make(map[string]bool)
			for _, c := range domain.Graph.Concepts {
				domainConcepts[c] = true
			}
			var filteredStates []*models.ConceptState
			for _, cs := range states {
				if domainConcepts[cs.Concept] {
					filteredStates = append(filteredStates, cs)
				}
			}
			var filteredInteractions []*models.Interaction
			for _, i := range interactions {
				if domainConcepts[i.Concept] {
					filteredInteractions = append(filteredInteractions, i)
				}
			}
			states = filteredStates
			interactions = filteredInteractions
		}

		alerts := engine.ComputeAlerts(states, interactions, sessionStart)

		hasCritical := false
		for _, a := range alerts {
			if a.Urgency == models.UrgencyCritical {
				hasCritical = true
				break
			}
		}
		if alerts == nil {
			alerts = []models.Alert{}
		}

		r, _ := jsonResult(map[string]interface{}{
			"alerts":       alerts,
			"has_critical": hasCritical,
		})
		return r, nil, nil
	})
}
