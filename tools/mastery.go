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

type CheckMasteryParams struct {
	Concept  string `json:"concept" jsonschema:"Le concept à vérifier pour la maîtrise"`
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
}

func registerCheckMastery(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_mastery",
		Description: "Vérifie si un concept est prêt pour le mastery challenge (BKT >= 0.85).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params CheckMasteryParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("check_mastery: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.Concept == "" {
			r, _ := errorResult("concept is required")
			return r, nil, nil
		}

		cs, err := deps.Store.GetConceptState(learnerID, params.Concept)
		if err != nil {
			deps.Logger.Error("check_mastery: failed to get concept state", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("concept state not found: %v", err))
			return r, nil, nil
		}

		bktState := algorithms.BKTState{PMastery: cs.PMastery}
		isMastered := algorithms.BKTIsMastered(bktState)

		if !isMastered {
			r, _ := jsonResult(map[string]interface{}{
				"mastery_ready": false,
				"current_mastery": cs.PMastery,
				"threshold":       algorithms.MasteryBKT(),
				"message":         fmt.Sprintf("Pas encore pret. Maitrise actuelle: %.0f%%, seuil: %.0f%%", cs.PMastery*100, algorithms.MasteryBKT()*100),
			})
			return r, nil, nil
		}

		r, _ := jsonResult(map[string]interface{}{
			"mastery_ready":   true,
			"current_mastery": cs.PMastery,
			"challenge": map[string]interface{}{
				"prompt_for_llm": fmt.Sprintf(
					"Genere un mastery challenge sur %s. "+
						"L'apprenant doit construire quelque chose de complet qui demontre le transfert. "+
						"Evalue: application autonome, gestion des cas limites, qualite du code. "+
						"Ne guide pas — observe si l'apprenant peut appliquer seul.",
					params.Concept,
				),
				"evaluation_criteria": []string{
					"Application autonome sans aide",
					"Gestion correcte des cas limites",
					"Code propre et idiomatique",
					"Explication claire du raisonnement",
				},
			},
		})
		return r, nil, nil
	})
}
