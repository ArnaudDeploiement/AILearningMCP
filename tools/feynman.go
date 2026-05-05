// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"

	"tutor-mcp/algorithms"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type FeynmanChallengeParams struct {
	ConceptID string `json:"concept_id" jsonschema:"Le concept à expliquer avec la méthode Feynman"`
	DomainID  string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
}

func registerFeynmanChallenge(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "feynman_challenge",
		Description: "Demande à l'apprenant d'expliquer un concept maîtrisé avec ses propres mots. Le LLM identifie les gaps et les injecte dans le graphe BKT.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params FeynmanChallengeParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("feynman_challenge: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.ConceptID == "" {
			r, _ := errorResult("concept_id is required")
			return r, nil, nil
		}

		cs, err := deps.Store.GetConceptState(learnerID, params.ConceptID)
		if err != nil {
			deps.Logger.Error("feynman_challenge: failed to get concept state", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("concept not found: %v", err))
			return r, nil, nil
		}

		bktState := algorithms.BKTState{PMastery: cs.PMastery}
		if !algorithms.BKTIsMastered(bktState) {
			r, _ := jsonResult(map[string]interface{}{
				"eligible":  false,
				"mastery":   cs.PMastery,
				"threshold": algorithms.MasteryBKT(),
				"message":   "Concept pas encore maitrise. Continue la pratique reguliere.",
			})
			return r, nil, nil
		}

		promptText := fmt.Sprintf(
			"Explique le concept '%s' comme si tu l'enseignais a quelqu'un qui n'y connait rien. "+
				"Pas de jargon technique — utilise des analogies, des exemples concrets. "+
				"L'objectif est de verifier que tu as vraiment compris, pas que tu sais reciter.\n\n"+
				"Apres ton explication, je vais identifier les points flous ou incomplets "+
				"et les transformer en micro-concepts a travailler.",
			params.ConceptID,
		)

		r, _ := jsonResult(map[string]interface{}{
			"eligible":    true,
			"prompt_text": promptText,
			"concept_id":  params.ConceptID,
			"instructions_for_llm": "Apres l'explication de l'apprenant, identifie les gaps conceptuels specifiques. " +
				"Pour chaque gap, genere un label court et une description. " +
				"Demande confirmation a l'apprenant avant d'injecter les gaps dans le graphe via add_concepts(). " +
				"Les nouveaux micro-concepts doivent avoir le concept source comme prerequis.",
			"gap_template": map[string]interface{}{
				"label":          "<nom court du gap>",
				"description":    "<ce qui manque dans l'explication>",
				"initial_pl0":    0.1,
				"source_concept": params.ConceptID,
			},
		})
		return r, nil, nil
	})
}
