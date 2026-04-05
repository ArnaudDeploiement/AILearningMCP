package tools

import (
	"context"
	"fmt"
	"time"

	"learning-runtime/algorithms"
	"learning-runtime/engine"
	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetCockpitStateParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel). Si absent, affiche tous les domaines actifs."`
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
		Description: "Genere l'etat complet du cockpit: progression par concept, alertes, signaux, prochaine action. Sans domain_id, affiche tous les domaines.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetCockpitStateParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
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
				r, _ := errorResult(fmt.Sprintf("domain not found: %v", err))
				return r, nil, nil
			}
			domains = []*models.Domain{d}
		} else {
			allDomains, err := deps.Store.GetDomainsByLearner(learnerID)
			if err != nil || len(allDomains) == 0 {
				r, _ := errorResult("aucun domaine configure")
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
			nextAction := "continuer la revision"
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

		// Global alerts
		alerts := engine.ComputeAlerts(states, interactions, sessionStart)
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
