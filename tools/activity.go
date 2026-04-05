package tools

import (
	"context"
	"time"

	"learning-runtime/algorithms"
	"learning-runtime/engine"
	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetNextActivityParams struct {
	DomainID string `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel, utilise le dernier domaine si absent)"`
}

func registerGetNextActivity(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_next_activity",
		Description: "Determine la prochaine activite optimale pour l'apprenant selon son etat cognitif. Tient compte de la session en cours pour eviter de repeter le meme concept.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params GetNextActivityParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
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
					Rationale:    "aucun domaine configure",
					PromptForLLM: "L'apprenant n'a pas encore de domaine. Analyse son objectif, decompose-le en concepts et appelle init_domain().",
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

		// Route to next activity (session-aware)
		activity := engine.Route(alerts, frontier, domainStates, domainInteractions, sessionConcepts)

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

		r, _ := jsonResult(map[string]interface{}{
			"needs_domain_setup":    false,
			"domain_id":             domain.ID,
			"activity":              activity,
			"session_concepts_done": len(sessionConcepts),
			"metacognitive_mirror":  mirror,
			"tutor_mode":            tutorMode,
		})
		return r, nil, nil
	})
}
