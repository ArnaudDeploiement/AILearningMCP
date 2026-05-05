// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"
	"time"

	"tutor-mcp/algorithms"
	"tutor-mcp/assets"
	"tutor-mcp/engine"
	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// cockpitResourceURI is the MCP Apps resource URI for the cockpit UI.
// Used by the registered Resource (registerCockpitResource), the open_cockpit
// Tool's _meta.ui.resourceUri, and the CallToolResult's _meta.ui.resourceUri.
const cockpitResourceURI = "ui://cockpit"

// cockpitUIMeta returns a fresh _meta payload pointing at the cockpit
// resource — used both on the Tool.Meta (so clients see the resource URI
// before calling) and on CallToolResult.Meta (so the client knows which
// resource to fetch after calling).
func cockpitUIMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"resourceUri": cockpitResourceURI,
			"visibility":  []string{"model", "app"},
		},
	}
}

type GetCockpitStateParams struct {
	DomainID        string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel). Si absent, affiche tous les domaines actifs."`
	IncludeArchived bool   `json:"include_archived,omitempty" jsonschema:"Si true, inclut les domaines archivés dans la réponse."`
}

type conceptProgress struct {
	Concept   string  `json:"concept"`
	Mastery   float64 `json:"mastery"`
	Retention float64 `json:"retention"`
	Status    string  `json:"status"`
	CardState string  `json:"card_state"`
}

type domainCockpit struct {
	DomainID       string            `json:"domain_id"`
	Name           string            `json:"name"`
	Archived       bool              `json:"archived"`
	TotalConcepts  int               `json:"total_concepts"`
	MasteredCount  int               `json:"mastered_count"`
	ProgressPct    float64           `json:"progress_percent"`
	Concepts       []conceptProgress `json:"concepts"`
	RetentionAlerts []map[string]interface{} `json:"retention_alerts"`
	NextAction     string            `json:"next_action"`
}

func registerGetCockpitState(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_cockpit_state",
		Description: "Retourne l'état brut du cockpit en JSON (progression par concept, alertes, signaux, prochaine action). USAGE PROGRAMMATIQUE — scripts d'éval, reporting. NE PAS UTILISER quand l'apprenant demande d'ouvrir/voir/afficher son cockpit : utiliser open_cockpit qui rend une UI native.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetCockpitStateParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("get_cockpit_state: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
		interactions, _ := deps.Store.GetRecentInteractionsByLearner(learnerID, 20)
		sessionStart, _ := deps.Store.GetSessionStart(learnerID)

		// Build global maps
		stateMap := make(map[string]*models.ConceptState)
		for _, cs := range states {
			stateMap[cs.Concept] = cs
		}

		// Determine which domains to show
		var domains []*models.Domain
		if params.DomainID != "" {
			d, err := deps.Store.GetDomainByID(params.DomainID)
			if err != nil {
				deps.Logger.Error("get_cockpit_state: failed to get domain", "err", err, "learner", learnerID)
				r, _ := errorResult(fmt.Sprintf("domain not found: %v", err))
				return r, nil, nil
			}
			if d.LearnerID != learnerID {
				r, _ := errorResult("domain not found")
				return r, nil, nil
			}
			domains = []*models.Domain{d}
		} else {
			allDomains, err := deps.Store.GetDomainsByLearner(learnerID, params.IncludeArchived)
			if err != nil {
				deps.Logger.Error("get_cockpit_state: failed to get domains", "err", err, "learner", learnerID)
				r, _ := errorResult("aucun domaine configuré")
				return r, nil, nil
			}
			if len(allDomains) == 0 {
				r, _ := errorResult("aucun domaine configuré")
				return r, nil, nil
			}
			domains = allDomains
		}

		// Build cockpit for each domain
		var domainCockpits []domainCockpit
		totalMastered := 0
		totalConcepts := 0

		for _, domain := range domains {
			mastery := make(map[string]float64)
			for _, c := range domain.Graph.Concepts {
				if cs, ok := stateMap[c]; ok {
					mastery[c] = cs.PMastery
				}
			}

			graph := algorithms.KSTGraph{
				Concepts:      domain.Graph.Concepts,
				Prerequisites: domain.Graph.Prerequisites,
			}

			var concepts []conceptProgress
			masteredCount := 0
			for _, concept := range domain.Graph.Concepts {
				status := algorithms.ConceptStatus(graph, mastery, concept)
				cs := stateMap[concept]

				cp := conceptProgress{
					Concept:   concept,
					Mastery:   mastery[concept],
					Retention: 1.0,
					Status:    status,
					CardState: "new",
				}

				if cs != nil {
					cp.CardState = cs.CardState
					elapsed := cs.ElapsedDays
					if cs.LastReview != nil {
						elapsed = int(time.Since(*cs.LastReview).Hours() / 24)
					}
					cp.Retention = algorithms.Retrievability(elapsed, cs.Stability)
				}

				if status == "done" {
					masteredCount++
				}
				concepts = append(concepts, cp)
			}

			// Retention alerts for this domain
			var retentionAlerts []map[string]interface{}
			for _, cp := range concepts {
				if cp.Retention < 0.50 && cp.CardState != "new" {
					color := "orange"
					if cp.Retention < 0.30 {
						color = "rouge"
					}
					retentionAlerts = append(retentionAlerts, map[string]interface{}{
						"concept":   cp.Concept,
						"retention": cp.Retention,
						"color":     color,
					})
				}
			}
			if retentionAlerts == nil {
				retentionAlerts = []map[string]interface{}{}
			}

			// Next action for this domain
			frontier := algorithms.ComputeFrontier(graph, mastery)
			nextAction := "continuer la révision"
			if len(frontier) > 0 {
				nextAction = fmt.Sprintf("nouveau concept: %s", frontier[0])
			}

			progressPct := 0.0
			if len(domain.Graph.Concepts) > 0 {
				progressPct = float64(masteredCount) / float64(len(domain.Graph.Concepts)) * 100
			}

			domainCockpits = append(domainCockpits, domainCockpit{
				DomainID:        domain.ID,
				Name:            domain.Name,
				Archived:        domain.Archived,
				TotalConcepts:   len(domain.Graph.Concepts),
				MasteredCount:   masteredCount,
				ProgressPct:     progressPct,
				Concepts:        concepts,
				RetentionAlerts: retentionAlerts,
				NextAction:      nextAction,
			})

			totalMastered += masteredCount
			totalConcepts += len(domain.Graph.Concepts)
		}

		// Global alerts — restrict to concepts that belong to a domain shown
		// in this response. Otherwise orphan states from deleted domains (which
		// intentionally preserve concept_states / interactions) would surface as
		// alerts on concepts the user no longer has.
		shownConcepts := make(map[string]bool)
		for _, d := range domains {
			for _, c := range d.Graph.Concepts {
				shownConcepts[c] = true
			}
		}
		alertStates := filterStatesByConcepts(states, shownConcepts)
		alertInteractions := filterInteractionsByConcepts(interactions, shownConcepts)
		alerts := engine.ComputeAlerts(alertStates, alertInteractions, sessionStart)
		if alerts == nil {
			alerts = []models.Alert{}
		}

		// Global trajectory signal
		signal := "stable"
		if len(interactions) >= 3 {
			recentSuccesses := 0
			window := interactions
			if len(window) > 5 {
				window = window[:5]
			}
			for _, i := range window {
				if i.Success {
					recentSuccesses++
				}
			}
			rate := float64(recentSuccesses) / float64(len(window))
			if rate >= 0.8 {
				signal = "positive"
			} else if rate < 0.4 {
				signal = "declining"
			}
		}

		// Global progress
		globalProgress := 0.0
		if totalConcepts > 0 {
			globalProgress = float64(totalMastered) / float64(totalConcepts) * 100
		}

		// Autonomy metrics
		since := time.Now().UTC().Add(-30 * 24 * time.Hour)
		allInteractions, _ := deps.Store.GetInteractionsSince(learnerID, since)
		calibBias, _ := deps.Store.GetCalibrationBias(learnerID, 20)

		autonomy := engine.ComputeAutonomyMetrics(engine.AutonomyInput{
			Interactions:    allInteractions,
			ConceptStates:   states,
			CalibrationBias: calibBias,
			SessionGap:      2 * time.Hour,
		})

		affects, _ := deps.Store.GetRecentAffectStates(learnerID, 10)
		var autonomyScores []float64
		var affectLastN []interface{}
		for _, a := range affects {
			autonomyScores = append(autonomyScores, a.AutonomyScore)
			affectLastN = append(affectLastN, a)
		}
		if affectLastN == nil {
			affectLastN = []interface{}{}
		}
		autonomy.Trend = engine.ComputeAutonomyTrendExported(autonomyScores)
		dependencyTrend := autonomy.Trend

		r, _ := jsonResult(map[string]interface{}{
			"domains":          domainCockpits,
			"total_concepts":   totalConcepts,
			"total_mastered":   totalMastered,
			"global_progress":  globalProgress,
			"alerts":           alerts,
			"signal":           signal,
			"autonomy_score":   autonomy.Score,
			"calibration_bias": calibBias,
			"affect_last_n":    affectLastN,
			"dependency_trend": dependencyTrend,
		})
		return r, nil, nil
	})
}

type OpenCockpitParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, dernier domaine actif si absent)"`
}

func registerOpenCockpit(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "open_cockpit",
		Description: "OUTIL À UTILISER quand l'apprenant demande d'ouvrir/voir/afficher/montrer son cockpit. Rend une UI MCP App native dans la conversation (Claude Desktop, claude.ai) — focus card du moment, KCs alternatifs cliquables (l'apprenant peut épingler un concept comme nouveau focus via le tool pick_concept), signaux métacognitifs, progression du domaine. NE PAS reformuler le résultat en texte : la UI s'affiche d'elle-même via _meta.ui.resourceUri. Pour les clients sans MCP Apps, le tool retourne aussi un résumé texte de fallback.",
		Meta: cockpitUIMeta(),
	}, func(ctx context.Context, req *mcp.CallToolRequest, params OpenCockpitParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("open_cockpit: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		graph, err := engine.BuildOLMGraph(deps.Store, learnerID, params.DomainID)
		if err != nil {
			deps.Logger.Error("open_cockpit: build graph failed", "err", err, "learner", learnerID)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		// Text fallback for clients without MCP Apps support — reuses the
		// webhook formatter so cockpit and webhook show the same prose.
		fallback := engine.FormatOLMEmbed(graph.OLMSnapshot)

		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: fallback.Description}},
			StructuredContent: graph,
			Meta: cockpitUIMeta(),
		}, nil, nil
	})
}

// registerCockpitResource serves the cockpit HTML at ui://cockpit. The HTML
// is embedded via assets.FS — see assets/embed.go.
func registerCockpitResource(server *mcp.Server, deps *Deps) {
	server.AddResource(&mcp.Resource{
		URI:         cockpitResourceURI,
		Name:        "cockpit",
		Title:       "Cockpit d'apprentissage",
		Description: "Interface MCP App rendue par le client (Claude Desktop, claude.ai). Carte cognitive interactive de l'apprenant pour la session courante et le modèle global.",
		MIMEType:    "text/html;profile=mcp-app",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := ""
		if req != nil && req.Params != nil {
			uri = req.Params.URI
		}
		deps.Logger.Info("cockpit resource read", "uri", uri)
		body, err := assets.FS.ReadFile("cockpit.html")
		if err != nil {
			deps.Logger.Error("cockpit resource: read embedded html", "err", err)
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      cockpitResourceURI,
				MIMEType: "text/html;profile=mcp-app",
				Text:     string(body),
			}},
		}, nil
	})
}
