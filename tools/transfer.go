// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"

	"tutor-mcp/algorithms"
	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TransferChallengeParams struct {
	ConceptID   string `json:"concept_id" jsonschema:"Le concept à tester en transfert"`
	ContextType string `json:"context_type,omitempty" jsonschema:"Type de contexte: real_world, interview, teaching, debugging, creative (optionnel)"`
	DomainID    string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
}

func registerTransferChallenge(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "transfer_challenge",
		Description: "Génère une situation inédite pour tester le transfert d'un concept maîtrisé hors du contexte initial.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params TransferChallengeParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("transfer_challenge: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.ConceptID == "" {
			r, _ := errorResult("concept_id is required")
			return r, nil, nil
		}

		cs, err := deps.Store.GetConceptState(learnerID, params.ConceptID)
		if err != nil {
			deps.Logger.Error("transfer_challenge: failed to get concept state", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("concept not found: %v", err))
			return r, nil, nil
		}

		bktState := algorithms.BKTState{PMastery: cs.PMastery}
		if !algorithms.BKTIsMastered(bktState) {
			r, _ := jsonResult(map[string]interface{}{
				"eligible": false,
				"mastery":  cs.PMastery,
				"message":  "Concept pas encore maîtrisé. Le transfert challenge requiert BKT >= 0.85.",
			})
			return r, nil, nil
		}

		contextType := params.ContextType
		if contextType == "" {
			contextType = "real_world"
		}

		existingTransfers, _ := deps.Store.GetTransferScores(learnerID, params.ConceptID)
		var testedContexts []string
		for _, tr := range existingTransfers {
			testedContexts = append(testedContexts, fmt.Sprintf("%s (%.0f%%)", tr.ContextType, tr.Score*100))
		}

		promptText := fmt.Sprintf(
			"Génère une situation totalement nouvelle qui teste le transfert du concept '%s' "+
				"dans un contexte de type '%s'. "+
				"La situation ne doit PAS ressembler aux exercices précédents. "+
				"L'objectif: vérifier que l'apprenant peut appliquer ce concept dans un contexte qu'il n'a jamais vu.\n\n"+
				"Après la réponse de l'apprenant, évalue le transfer_score (0-1) et "+
				"appelle record_transfer_result avec le résultat.",
			params.ConceptID, contextType,
		)

		r, _ := jsonResult(map[string]interface{}{
			"eligible":        true,
			"concept_id":      params.ConceptID,
			"context_type":    contextType,
			"prompt_text":     promptText,
			"tested_contexts": testedContexts,
		})
		return r, nil, nil
	})
}

// ─── record_transfer_result ─────────────────────────────────────────────────

type RecordTransferResultParams struct {
	ConceptID   string  `json:"concept_id" jsonschema:"Le concept testé"`
	ContextType string  `json:"context_type" jsonschema:"Type de contexte du challenge"`
	Score       float64 `json:"score" jsonschema:"Score de transfert entre 0 et 1"`
	SessionID   string  `json:"session_id,omitempty" jsonschema:"ID de session (optionnel)"`
}

func registerRecordTransferResult(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "record_transfer_result",
		Description: "Enregistre le résultat d'un transfer challenge.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params RecordTransferResultParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("record_transfer_result: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		record := &models.TransferRecord{
			LearnerID:   learnerID,
			ConceptID:   params.ConceptID,
			ContextType: params.ContextType,
			Score:       params.Score,
			SessionID:   params.SessionID,
		}

		if err := deps.Store.CreateTransferRecord(record); err != nil {
			deps.Logger.Error("record_transfer_result: failed to create transfer record", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to record transfer: %v", err))
			return r, nil, nil
		}

		r, _ := jsonResult(map[string]interface{}{
			"recorded":       true,
			"transfer_score": params.Score,
			"blocked":        params.Score < 0.50,
		})
		return r, nil, nil
	})
}
