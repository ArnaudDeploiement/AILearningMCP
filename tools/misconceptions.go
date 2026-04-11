package tools

import (
	"context"
	"fmt"

	"learning-runtime/db"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetMisconceptionsParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, tous les domaines si absent)"`
	Concept  string `json:"concept,omitempty" jsonschema:"Filtre par concept (optionnel)"`
}

func registerGetMisconceptions(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_misconceptions",
		Description: "Liste les misconceptions detectees par concept, avec leur statut (active/resolved) et frequence. Permet de suivre les confusions recurrentes de l'apprenant.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetMisconceptionsParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("get_misconceptions: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		// Build concept filter from domain if provided
		var conceptFilter map[string]bool
		if params.DomainID != "" {
			domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
			if err != nil {
				deps.Logger.Error("get_misconceptions: domain resolution failed", "err", err, "domain", params.DomainID)
				r, _ := errorResult(fmt.Sprintf("domain not found: %s", params.DomainID))
				return r, nil, nil
			}
			conceptFilter = make(map[string]bool)
			for _, concept := range domain.Graph.Concepts {
				conceptFilter[concept] = true
			}
		}

		// Narrow filter to specific concept if provided
		if params.Concept != "" {
			if conceptFilter != nil && !conceptFilter[params.Concept] {
				// Concept not in domain, return empty
				r, _ := jsonResult(map[string]any{"misconceptions": []db.MisconceptionGroup{}})
				return r, nil, nil
			}
			conceptFilter = map[string]bool{params.Concept: true}
		}

		// Get misconception groups
		groups, err := deps.Store.GetMisconceptionGroups(learnerID, conceptFilter)
		if err != nil {
			deps.Logger.Error("get_misconceptions: failed to fetch groups", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to fetch misconceptions: %v", err))
			return r, nil, nil
		}

		// Replace nil with empty slice for JSON serialization
		if groups == nil {
			groups = []db.MisconceptionGroup{}
		}

		r, _ := jsonResult(map[string]any{"misconceptions": groups})
		return r, nil, nil
	})
}
