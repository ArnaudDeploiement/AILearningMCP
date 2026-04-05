package tools

import (
	"context"
	"fmt"

	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type InitDomainParams struct {
	Name          string              `json:"name" jsonschema:"Nom du domaine d'apprentissage"`
	Concepts      []string            `json:"concepts" jsonschema:"Liste des concepts du domaine"`
	Prerequisites map[string][]string `json:"prerequisites" jsonschema:"Graphe de prerequis (concept -> liste de prerequis)"`
	PersonalGoal  string              `json:"personal_goal,omitempty" jsonschema:"Objectif personnel de l'apprenant dans ce domaine (optionnel)"`
}

func registerInitDomain(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "init_domain",
		Description: "Initialise un domaine d'apprentissage avec ses concepts et prerequis. Ne detruit pas la progression existante.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params InitDomainParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.Name == "" {
			r, _ := errorResult("name is required")
			return r, nil, nil
		}
		if len(params.Concepts) == 0 {
			r, _ := errorResult("at least one concept is required")
			return r, nil, nil
		}

		graph := models.KnowledgeSpace{
			Concepts:      params.Concepts,
			Prerequisites: params.Prerequisites,
		}
		if graph.Prerequisites == nil {
			graph.Prerequisites = make(map[string][]string)
		}

		domain, err := deps.Store.CreateDomain(learnerID, params.Name, params.PersonalGoal, graph)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("failed to create domain: %v", err))
			return r, nil, nil
		}

		// Initialize ConceptState for each concept — INSERT OR IGNORE preserves existing progress
		for _, concept := range params.Concepts {
			cs := models.NewConceptState(learnerID, concept)
			if err := deps.Store.InsertConceptStateIfNotExists(cs); err != nil {
				r, _ := errorResult(fmt.Sprintf("failed to initialize concept %s: %v", concept, err))
				return r, nil, nil
			}
		}

		r, _ := jsonResult(map[string]interface{}{
			"domain_id":     domain.ID,
			"concept_count": len(params.Concepts),
			"message":       fmt.Sprintf("Domaine '%s' cree avec %d concepts. La progression existante est preservee.", params.Name, len(params.Concepts)),
		})
		return r, nil, nil
	})
}

// ─── Add Concepts ────────────────────────────────────────────────────────────

type AddConceptsParams struct {
	DomainID      string              `json:"domain_id" jsonschema:"ID du domaine cible"`
	Concepts      []string            `json:"concepts" jsonschema:"Nouveaux concepts a ajouter"`
	Prerequisites map[string][]string `json:"prerequisites" jsonschema:"Nouveaux prerequis (concept -> liste de prerequis). Peut inclure des liens vers des concepts existants."`
}

func registerAddConcepts(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_concepts",
		Description: "Ajoute des concepts a un domaine existant sans detruire la progression. Utiliser pour enrichir un domaine en cours de route.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params AddConceptsParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if len(params.Concepts) == 0 {
			r, _ := errorResult("at least one concept is required")
			return r, nil, nil
		}

		// Resolve domain
		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("domain not found: %v", err))
			return r, nil, nil
		}

		// Merge new concepts into existing graph
		existingSet := make(map[string]bool)
		for _, c := range domain.Graph.Concepts {
			existingSet[c] = true
		}

		added := 0
		for _, c := range params.Concepts {
			if !existingSet[c] {
				domain.Graph.Concepts = append(domain.Graph.Concepts, c)
				existingSet[c] = true
				added++
			}
		}

		// Merge prerequisites
		if domain.Graph.Prerequisites == nil {
			domain.Graph.Prerequisites = make(map[string][]string)
		}
		for concept, prereqs := range params.Prerequisites {
			existing := make(map[string]bool)
			for _, p := range domain.Graph.Prerequisites[concept] {
				existing[p] = true
			}
			for _, p := range prereqs {
				if !existing[p] {
					domain.Graph.Prerequisites[concept] = append(domain.Graph.Prerequisites[concept], p)
				}
			}
		}

		// Persist updated graph
		if err := deps.Store.UpdateDomainGraph(domain.ID, domain.Graph); err != nil {
			r, _ := errorResult(fmt.Sprintf("failed to update domain graph: %v", err))
			return r, nil, nil
		}

		// Initialize concept states for new concepts only (INSERT OR IGNORE)
		for _, concept := range params.Concepts {
			cs := models.NewConceptState(learnerID, concept)
			if err := deps.Store.InsertConceptStateIfNotExists(cs); err != nil {
				r, _ := errorResult(fmt.Sprintf("failed to initialize concept %s: %v", concept, err))
				return r, nil, nil
			}
		}

		r, _ := jsonResult(map[string]interface{}{
			"domain_id":     domain.ID,
			"added":         added,
			"total_concepts": len(domain.Graph.Concepts),
			"message":       fmt.Sprintf("%d nouveaux concepts ajoutes. Total: %d. Progression existante preservee.", added, len(domain.Graph.Concepts)),
		})
		return r, nil, nil
	})
}
