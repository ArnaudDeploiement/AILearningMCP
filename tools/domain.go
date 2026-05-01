// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input caps for domain-management tools. These bound the cost of a single
// MCP call and stop a misbehaving client from pushing arbitrarily large
// strings or graphs into SQLite.
const (
	maxDomainNameLen        = 200
	maxPersonalGoalLen      = 2000
	maxConceptNameLen       = 200
	maxConceptsPerCall      = 500
	maxPrereqEntriesPerNode = 20
	maxValueFramingLen      = 2000
)

// validateConcepts enforces the size caps on a concept list and its
// prerequisite graph. Returns the first error found (one short message).
func validateConcepts(concepts []string, prereqs map[string][]string) error {
	if len(concepts) > maxConceptsPerCall {
		return fmt.Errorf("too many concepts: %d (max %d)", len(concepts), maxConceptsPerCall)
	}
	for _, c := range concepts {
		if c == "" {
			return fmt.Errorf("empty concept name")
		}
		if len(c) > maxConceptNameLen {
			return fmt.Errorf("concept name too long (max %d chars)", maxConceptNameLen)
		}
	}
	if len(prereqs) > maxConceptsPerCall {
		return fmt.Errorf("too many prerequisite entries: %d (max %d)", len(prereqs), maxConceptsPerCall)
	}
	for k, vs := range prereqs {
		if len(k) > maxConceptNameLen {
			return fmt.Errorf("prerequisite key too long (max %d chars)", maxConceptNameLen)
		}
		if len(vs) > maxPrereqEntriesPerNode {
			return fmt.Errorf("too many prerequisites for %q (max %d)", k, maxPrereqEntriesPerNode)
		}
		for _, v := range vs {
			if len(v) > maxConceptNameLen {
				return fmt.Errorf("prerequisite value too long (max %d chars)", maxConceptNameLen)
			}
		}
	}
	return nil
}

func validateValueFramings(vf *ValueFramingsInput) error {
	if vf == nil {
		return nil
	}
	for _, f := range []string{vf.Financial, vf.Employment, vf.Intellectual, vf.Innovation} {
		if len(f) > maxValueFramingLen {
			return fmt.Errorf("value_framing too long (max %d chars)", maxValueFramingLen)
		}
	}
	return nil
}

type ValueFramingsInput struct {
	Financial    string `json:"financial,omitempty" jsonschema:"Gain financier (1-2 phrases)"`
	Employment   string `json:"employment,omitempty" jsonschema:"Gain employabilite / carriere (1-2 phrases)"`
	Intellectual string `json:"intellectual,omitempty" jsonschema:"Gain intellectuel / beau raisonnement (1-2 phrases)"`
	Innovation   string `json:"innovation,omitempty" jsonschema:"Gain creation / innovation (1-2 phrases)"`
}

type InitDomainParams struct {
	Name          string              `json:"name" jsonschema:"Nom du domaine d'apprentissage"`
	Concepts      []string            `json:"concepts" jsonschema:"Liste des concepts du domaine"`
	Prerequisites map[string][]string `json:"prerequisites" jsonschema:"Graphe de prerequis (concept -> liste de prerequis)"`
	PersonalGoal  string              `json:"personal_goal,omitempty" jsonschema:"Objectif personnel de l'apprenant dans ce domaine (optionnel)"`
	ValueFramings *ValueFramingsInput `json:"value_framings,omitempty" jsonschema:"4 axes de valeur (financier/emploi/intellectuel/innovation). 1-2 phrases authored par axe. Optionnel — peut etre rempli a la volee par la suite."`
}

func registerInitDomain(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "init_domain",
		Description: "Initialise un domaine d'apprentissage avec ses concepts et prerequis. Ne detruit pas la progression existante.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params InitDomainParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("init_domain: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.Name == "" {
			r, _ := errorResult("name is required")
			return r, nil, nil
		}
		if len(params.Name) > maxDomainNameLen {
			r, _ := errorResult(fmt.Sprintf("name too long (max %d chars)", maxDomainNameLen))
			return r, nil, nil
		}
		if len(params.PersonalGoal) > maxPersonalGoalLen {
			r, _ := errorResult(fmt.Sprintf("personal_goal too long (max %d chars)", maxPersonalGoalLen))
			return r, nil, nil
		}
		if len(params.Concepts) == 0 {
			r, _ := errorResult("at least one concept is required")
			return r, nil, nil
		}
		if err := validateConcepts(params.Concepts, params.Prerequisites); err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}
		if err := validateValueFramings(params.ValueFramings); err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		graph := models.KnowledgeSpace{
			Concepts:      params.Concepts,
			Prerequisites: params.Prerequisites,
		}
		if graph.Prerequisites == nil {
			graph.Prerequisites = make(map[string][]string)
		}

		valueFramingsJSON := ""
		if params.ValueFramings != nil {
			vf := models.DomainValueFramings{
				Financial:    params.ValueFramings.Financial,
				Employment:   params.ValueFramings.Employment,
				Intellectual: params.ValueFramings.Intellectual,
				Innovation:   params.ValueFramings.Innovation,
			}
			if buf, merr := json.Marshal(vf); merr == nil {
				valueFramingsJSON = string(buf)
			}
		}

		domain, err := deps.Store.CreateDomainWithValueFramings(learnerID, params.Name, params.PersonalGoal, graph, valueFramingsJSON)
		if err != nil {
			deps.Logger.Error("init_domain: failed to create domain", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to create domain: %v", err))
			return r, nil, nil
		}

		// Initialize ConceptState for each concept — INSERT OR IGNORE preserves existing progress
		for _, concept := range params.Concepts {
			cs := models.NewConceptState(learnerID, concept)
			if err := deps.Store.InsertConceptStateIfNotExists(cs); err != nil {
				deps.Logger.Error("init_domain: failed to initialize concept state", "err", err, "learner", learnerID, "concept", concept)
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
			deps.Logger.Error("add_concepts: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if len(params.Concepts) == 0 {
			r, _ := errorResult("at least one concept is required")
			return r, nil, nil
		}
		if err := validateConcepts(params.Concepts, params.Prerequisites); err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		// Resolve domain
		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil {
			deps.Logger.Error("add_concepts: failed to resolve domain", "err", err, "learner", learnerID)
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
			deps.Logger.Error("add_concepts: failed to update domain graph", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to update domain graph: %v", err))
			return r, nil, nil
		}

		// Initialize concept states for new concepts only (INSERT OR IGNORE)
		for _, concept := range params.Concepts {
			cs := models.NewConceptState(learnerID, concept)
			if err := deps.Store.InsertConceptStateIfNotExists(cs); err != nil {
				deps.Logger.Error("add_concepts: failed to initialize concept state", "err", err, "learner", learnerID, "concept", concept)
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
