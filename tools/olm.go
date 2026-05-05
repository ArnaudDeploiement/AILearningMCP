// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"context"

	"tutor-mcp/engine"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetOLMSnapshotParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, utilisé le dernier domaine actif si absent)"`
	Scope    string `json:"scope,omitempty" jsonschema:"'session' (défaut, snapshot d'un domaine), 'global' (agrégation multi-domaine), ou 'graph' (OLMGraph complet d'un domaine — utilisé par le cockpit pour switcher entre domaines actifs)"`
}

func registerGetOLMSnapshot(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_olm_snapshot",
		Description: "Retourne un snapshot transparent de l'état d'apprentissage : distribution mastery, concept en focus, signaux métacognitifs actifs, progression vers le goal. Apprenant et tuteur regardent les mêmes données. Appeler avant queue_webhook_message(kind='olm:<domain_id>') ou pour reflet métacognitif en session.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetOLMSnapshotParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("get_olm_snapshot: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.Scope == "global" {
			g, err := engine.BuildGlobalOLMSnapshot(deps.Store, learnerID)
			if err != nil {
				deps.Logger.Error("get_olm_snapshot global: build failed", "err", err, "learner", learnerID)
				r, _ := errorResult(err.Error())
				return r, nil, nil
			}
			r, _ := jsonResult(g)
			return r, nil, nil
		}

		if params.Scope == "graph" {
			g, err := engine.BuildOLMGraph(deps.Store, learnerID, params.DomainID)
			if err != nil {
				deps.Logger.Error("get_olm_snapshot graph: build failed", "err", err, "learner", learnerID)
				r, _ := errorResult(err.Error())
				return r, nil, nil
			}
			r, _ := jsonResult(g)
			return r, nil, nil
		}

		// Default — session scope (existing behavior).
		snap, err := engine.BuildOLMSnapshot(deps.Store, learnerID, params.DomainID)
		if err != nil {
			deps.Logger.Error("get_olm_snapshot: build failed", "err", err, "learner", learnerID)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		r, _ := jsonResult(snap)
		return r, nil, nil
	})
}
