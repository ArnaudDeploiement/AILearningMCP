// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"
	"time"

	"tutor-mcp/algorithms"
	"tutor-mcp/engine"
	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetNextActivityParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, utilisé le dernier domaine si absent)"`
}

func registerGetNextActivity(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_next_activity",
		Description: "Détermine la prochaine activité optimale pour l'apprenant selon son état cognitif. Tient compte de la session en cours pour éviter de répéter le même concept.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetNextActivityParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("get_next_activity: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		// Check if domain exists
		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil || domain == nil {
			r, _ := jsonResult(map[string]interface{}{
				"needs_domain_setup": true,
				"activity": models.Activity{
					Type:         models.ActivitySetupDomain,
					Rationale:    "aucun domaine configuré",
					PromptForLLM: "L'apprenant n'a pas encore de domaine. Analyse son objectif, décompose-le en concepts et appelle init_domain().",
				},
			})
			return r, nil, nil
		}

		states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
		interactions, _ := deps.Store.GetRecentInteractionsByLearner(learnerID, 20)
		sessionStart, _ := deps.Store.GetSessionStart(learnerID)

		// Get session interactions to track what was already practiced
		sessionInteractions, _ := deps.Store.GetSessionInteractions(learnerID)

		// Filter states to only those in the current domain
		domainConcepts := make(map[string]bool)
		for _, c := range domain.Graph.Concepts {
			domainConcepts[c] = true
		}
		var domainStates []*models.ConceptState
		for _, cs := range states {
			if domainConcepts[cs.Concept] {
				domainStates = append(domainStates, cs)
			}
		}

		// Filter interactions to domain concepts
		var domainInteractions []*models.Interaction
		for _, i := range interactions {
			if domainConcepts[i.Concept] {
				domainInteractions = append(domainInteractions, i)
			}
		}

		// Compute alerts (only for domain concepts)
		alerts := engine.ComputeAlerts(domainStates, domainInteractions, sessionStart)

		// Build mastery map for KST frontier
		mastery := make(map[string]float64)
		for _, cs := range domainStates {
			mastery[cs.Concept] = cs.PMastery
		}

		// Compute frontier
		graph := algorithms.KSTGraph{
			Concepts:      domain.Graph.Concepts,
			Prerequisites: domain.Graph.Prerequisites,
		}
		frontier := algorithms.ComputeFrontier(graph, mastery)

		// Build set of concepts already practiced in this session
		sessionConcepts := make(map[string]int)
		for _, i := range sessionInteractions {
			if domainConcepts[i.Concept] {
				sessionConcepts[i.Concept]++
			}
		}

		// Route to next activity (session-aware).
		// Branchement [2] PhaseController : si REGULATION_PHASE=on,
		// l'orchestrateur regulation prend la main ; sinon le router
		// legacy reste en place (cohérent avec OQ-2.4 = no cache, le
		// flag est lu à chaque appel).
		var activity models.Activity
		var routePath string
		if regulationPhaseEnabled() {
			orchActivity, orchErr := engine.Orchestrate(deps.Store, engine.OrchestratorInput{
				LearnerID: learnerID,
				DomainID:  domain.ID,
				Now:       time.Now().UTC(),
				Config:    engine.NewDefaultPhaseConfig(),
			})
			if orchErr != nil {
				// Fallback gracieux : log + legacy. Évite de casser
				// la session sur une erreur orchestrateur.
				deps.Logger.Error("orchestrator failed, falling back to legacy router", "err", orchErr)
				activity = engine.Route(alerts, frontier, domainStates, domainInteractions, sessionConcepts)
				routePath = "legacy_fallback"
			} else {
				activity = orchActivity
				routePath = "orchestrator"
			}
		} else {
			activity = engine.Route(alerts, frontier, domainStates, domainInteractions, sessionConcepts)
			routePath = "legacy"
		}
		// Pipeline decision audit — one line per get_next_activity call.
		// Re-read domain to surface any phase transition the orchestrator
		// just persisted (cheap; same DB connection).
		var loggedPhase string
		if regulationPhaseEnabled() {
			if d, _ := deps.Store.GetDomainByID(domain.ID); d != nil {
				loggedPhase = string(d.Phase)
			}
			if loggedPhase == "" {
				loggedPhase = "INSTRUCTION" // orchestrator's NULL fallback
			}
		} else {
			loggedPhase = "n/a"
		}
		deps.Logger.Info("pipeline decision",
			"route", routePath,
			"learner", learnerID,
			"domain", domain.ID,
			"phase", loggedPhase,
			"activity_type", activity.Type,
			"concept", activity.Concept,
			"rationale", activity.Rationale,
		)

		// Metacognitive mirror
		since := time.Now().UTC().Add(-7 * 24 * time.Hour)
		allInteractions, _ := deps.Store.GetInteractionsSince(learnerID, since)
		calibBias, _ := deps.Store.GetCalibrationBias(learnerID, 20)
		affects, _ := deps.Store.GetRecentAffectStates(learnerID, 10)

		var autonomyScores []float64
		for _, a := range affects {
			autonomyScores = append(autonomyScores, a.AutonomyScore)
		}
		mirrorSessionCount := len(engine.GroupIntoSessionsExported(allInteractions, 2*time.Hour))

		mirror := engine.DetectMirrorPattern(engine.MirrorInput{
			Interactions:    allInteractions,
			ConceptStates:   domainStates,
			AutonomyScores:  autonomyScores,
			CalibrationBias: calibBias,
			SessionCount:    mirrorSessionCount,
		})

		// Tutor mode
		var currentAffect *models.AffectState
		if len(affects) > 0 {
			currentAffect = affects[0]
		}
		tutorMode := engine.ComputeTutorMode(currentAffect, alerts)

		// Apply tutor_mode adjustments to activity difficulty
		switch tutorMode {
		case "lighter":
			activity.DifficultyTarget *= 0.7
			activity.EstimatedMinutes = int(float64(activity.EstimatedMinutes) * 0.6)
			if activity.EstimatedMinutes < 5 {
				activity.EstimatedMinutes = 5
			}
		case "scaffolding":
			activity.DifficultyTarget *= 0.75
		case "recontextualize":
			activity.Rationale += " · recontextualisation demandée"
		}

		// Apply calibration bias adjustment
		// Positive bias = over-estimates → increase difficulty
		// Negative bias = under-estimates → decrease difficulty
		if calibBias != 0 {
			activity.DifficultyTarget += calibBias * 0.1
			if activity.DifficultyTarget < 0.3 {
				activity.DifficultyTarget = 0.3
			}
			if activity.DifficultyTarget > 0.85 {
				activity.DifficultyTarget = 0.85
			}
		}

		// Misconception enrichment for selected concept
		var activeMisconceptions any = []any{}
		var knownMisconceptionTypes any = []string{}

		if activity.Concept != "" {
			if active, err := deps.Store.GetActiveMisconceptions(learnerID, activity.Concept); err == nil && len(active) > 0 {
				activeMisconceptions = active

				// Inject misconceptions into prompt
				misconceptionPrompt := fmt.Sprintf("\nATTENTION : l'apprenant a %d misconception(s) active(s) : ", len(active))
				for i, m := range active {
					if i > 0 {
						misconceptionPrompt += " ; "
					}
					misconceptionPrompt += m.MisconceptionType
					if m.LastErrorDetail != "" {
						misconceptionPrompt += " — " + m.LastErrorDetail
					}
				}
				misconceptionPrompt += ". Cible ces confusions dans ton explication et ton exercice. Ne mentionne pas explicitement les misconceptions — conçois l'exercice pour qu'il les confronte naturellement."
				activity.PromptForLLM += misconceptionPrompt
			}

			if types, err := deps.Store.GetDistinctMisconceptionTypes(learnerID, activity.Concept); err == nil && len(types) > 0 {
				knownMisconceptionTypes = types
			}
		}

		// Motivation layer — compose a context-adaptive brief for this activity.
		// Detect plateau active on the chosen concept from the alerts list.
		plateauActive := false
		for _, a := range alerts {
			if a.Type == models.AlertPlateau && a.Concept == activity.Concept {
				plateauActive = true
				break
			}
		}
		motivationEngine := engine.NewMotivationEngine(deps.Store)
		motivationBrief, _ := motivationEngine.Build(
			learnerID, domain, activity.Concept, activity.Type,
			plateauActive, len(sessionConcepts),
		)

		r, _ := jsonResult(map[string]any{
			"needs_domain_setup":        false,
			"domain_id":                 domain.ID,
			"activity":                  activity,
			"session_concepts_done":     len(sessionConcepts),
			"metacognitive_mirror":      mirror,
			"tutor_mode":               tutorMode,
			"active_misconceptions":     activeMisconceptions,
			"known_misconception_types": knownMisconceptionTypes,
			"motivation_brief":          motivationBrief,
		})
		return r, nil, nil
	})
}
