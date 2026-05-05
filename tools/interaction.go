// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"
	"time"

	"tutor-mcp/algorithms"
	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RecordInteractionParams struct {
	Concept             string  `json:"concept" jsonschema:"Le concept concerné"`
	ActivityType        string  `json:"activity_type" jsonschema:"Type d'activité (RECALL_EXERCISE, NEW_CONCEPT, etc.)"`
	Success             bool    `json:"success" jsonschema:"L'exercice a été réussi"`
	ResponseTimeSeconds float64 `json:"response_time_seconds" jsonschema:"Temps de réponse en secondes"`
	Confidence          float64 `json:"confidence" jsonschema:"Confiance estimée entre 0 et 1"`
	ErrorType           string  `json:"error_type,omitempty" jsonschema:"Type d'erreur si échec: SYNTAX_ERROR, LOGIC_ERROR, KNOWLEDGE_GAP (optionnel)"`
	Notes               string  `json:"notes" jsonschema:"Notes optionnelles sur l'interaction"`
	DomainID            string  `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
	HintsRequested      int     `json:"hints_requested,omitempty" jsonschema:"Nombre d'indices demandés pendant l'échange (optionnel, défaut 0)"`
	SelfInitiated       bool    `json:"self_initiated,omitempty" jsonschema:"true si la session a démarré sans alerte webhook"`
	CalibrationID       string  `json:"calibration_id,omitempty" jsonschema:"ID de la prédiction de calibration associée (optionnel)"`
	MisconceptionType   string  `json:"misconception_type,omitempty" jsonschema:"Label libre de la misconception détectée (optionnel, ignoré si success=true)"`
	MisconceptionDetail string  `json:"misconception_detail,omitempty" jsonschema:"Description de la misconception en une phrase (optionnel)"`
}

func registerRecordInteraction(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "record_interaction",
		Description: "Enregistre le résultat d'un exercice et met à jour l'état cognitif de l'apprenant. Supporte error_type pour ajuster le BKT selon le type d'erreur.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params RecordInteractionParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("record_interaction: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.Concept == "" {
			r, _ := errorResult("concept is required")
			return r, nil, nil
		}

		// Get or create concept state — used for proactive review check AND algorithm updates
		cs, err := deps.Store.GetConceptState(learnerID, params.Concept)
		if err != nil {
			cs = models.NewConceptState(learnerID, params.Concept)
		}

		// Create interaction record
		interaction := &models.Interaction{
			LearnerID:      learnerID,
			Concept:        params.Concept,
			ActivityType:   params.ActivityType,
			Success:        params.Success,
			ResponseTime:   int(params.ResponseTimeSeconds),
			Confidence:     params.Confidence,
			ErrorType:      params.ErrorType,
			Notes:          params.Notes,
			HintsRequested: params.HintsRequested,
			SelfInitiated:  params.SelfInitiated,
			CalibrationID:  params.CalibrationID,
		}

		// Misconception fields — only stored on failures
		if !params.Success && params.MisconceptionType != "" {
			interaction.MisconceptionType = params.MisconceptionType
			interaction.MisconceptionDetail = params.MisconceptionDetail
		}

		// Check proactive review before recording
		if cs.NextReview != nil && cs.NextReview.After(time.Now().UTC()) && cs.CardState != "new" {
			interaction.IsProactiveReview = true
		}

		if err := deps.Store.CreateInteraction(interaction); err != nil {
			deps.Logger.Error("record_interaction: failed to create interaction", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to create interaction: %v", err))
			return r, nil, nil
		}

		// BKT update — error-type-aware
		bktState := algorithms.BKTState{
			PMastery: cs.PMastery,
			PLearn:   cs.PLearn,
			PForget:  cs.PForget,
			PSlip:    cs.PSlip,
			PGuess:   cs.PGuess,
		}
		bktState = algorithms.BKTUpdateWithErrorType(bktState, params.Success, params.ErrorType)
		cs.PMastery = bktState.PMastery

		// FSRS ReviewCard
		rating := algorithms.Good
		if !params.Success {
			rating = algorithms.Again
		} else if params.Confidence >= 0.9 {
			rating = algorithms.Easy
		} else if params.Confidence < 0.5 {
			rating = algorithms.Hard
		}

		var lastReview time.Time
		if cs.LastReview != nil {
			lastReview = *cs.LastReview
		}
		fsrsCard := algorithms.FSRSCard{
			Stability:     cs.Stability,
			Difficulty:    cs.Difficulty,
			ElapsedDays:   cs.ElapsedDays,
			ScheduledDays: cs.ScheduledDays,
			Reps:          cs.Reps,
			Lapses:        cs.Lapses,
			State:         algorithms.CardState(cs.CardState),
			LastReview:    lastReview,
		}
		now := time.Now().UTC()
		fsrsCard = algorithms.ReviewCard(fsrsCard, rating, now)
		cs.Stability = fsrsCard.Stability
		cs.Difficulty = fsrsCard.Difficulty
		cs.ElapsedDays = fsrsCard.ElapsedDays
		cs.ScheduledDays = fsrsCard.ScheduledDays
		cs.Reps = fsrsCard.Reps
		cs.Lapses = fsrsCard.Lapses
		cs.CardState = string(fsrsCard.State)
		cs.LastReview = &now
		nextReview := now.Add(time.Duration(fsrsCard.ScheduledDays) * 24 * time.Hour)
		cs.NextReview = &nextReview

		// IRT UpdateTheta
		item := algorithms.IRTItem{
			Difficulty:     algorithms.FSRSDifficultyToIRT(cs.Difficulty),
			Discrimination: 1.0,
		}
		cs.Theta = algorithms.IRTUpdateTheta(cs.Theta, []algorithms.IRTItem{item}, []bool{params.Success})

		// PFA Update
		pfaState := algorithms.PFAState{
			Successes: cs.PFASuccesses,
			Failures:  cs.PFAFailures,
		}
		pfaState = algorithms.PFAUpdate(pfaState, params.Success)
		cs.PFASuccesses = pfaState.Successes
		cs.PFAFailures = pfaState.Failures

		// Persist updated concept state
		if err := deps.Store.UpsertConceptState(cs); err != nil {
			deps.Logger.Error("record_interaction: failed to upsert concept state", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to update concept state: %v", err))
			return r, nil, nil
		}

		// Update last active
		_ = deps.Store.UpdateLastActive(learnerID)

		deps.Logger.Info("interaction recorded",
			"learner", learnerID,
			"concept", params.Concept,
			"activity_type", params.ActivityType,
			"success", params.Success,
			"hints_requested", params.HintsRequested,
			"self_initiated", params.SelfInitiated,
			"new_mastery", cs.PMastery,
			"new_theta", cs.Theta,
			"reps", cs.Reps,
		)

		// Compute engagement signal
		engagementSignal := "stable"
		if params.Confidence >= 0.8 && params.Success {
			engagementSignal = "positive"
		} else if !params.Success && params.Confidence < 0.3 {
			engagementSignal = "declining"
		}

		// Compute cognitive signals from session patterns
		sessionInteractions, _ := deps.Store.GetSessionInteractions(learnerID)
		fatigueSignal, frustrationSignal := computeCognitiveSignals(sessionInteractions)

		nextReviewHours := float64(fsrsCard.ScheduledDays) * 24.0

		r, _ := jsonResult(map[string]interface{}{
			"updated":              true,
			"new_mastery":          cs.PMastery,
			"next_review_in_hours": nextReviewHours,
			"engagement_signal":    engagementSignal,
			"fatigue_signal":       fatigueSignal,
			"frustration_signal":   frustrationSignal,
		})
		return r, nil, nil
	})
}

// computeCognitiveSignals analyzes session interaction patterns for fatigue and frustration.
func computeCognitiveSignals(sessionInteractions []*models.Interaction) (fatigue string, frustration string) {
	fatigue = "none"
	frustration = "none"

	if len(sessionInteractions) < 3 {
		return
	}

	// Fatigue: declining accuracy + increasing response time in last N interactions
	// Look at the most recent 5 interactions (they're sorted newest-first)
	window := sessionInteractions
	if len(window) > 5 {
		window = window[:5]
	}

	recentSuccesses := 0
	recentTotalTime := 0
	for _, i := range window {
		if i.Success {
			recentSuccesses++
		}
		recentTotalTime += i.ResponseTime
	}
	recentRate := float64(recentSuccesses) / float64(len(window))
	avgRecentTime := float64(recentTotalTime) / float64(len(window))

	// Compare with earlier interactions if available
	if len(sessionInteractions) >= 6 {
		earlier := sessionInteractions[len(window):]
		if len(earlier) > 5 {
			earlier = earlier[:5]
		}
		earlySuccesses := 0
		earlyTotalTime := 0
		for _, i := range earlier {
			if i.Success {
				earlySuccesses++
			}
			earlyTotalTime += i.ResponseTime
		}
		earlyRate := float64(earlySuccesses) / float64(len(earlier))
		avgEarlyTime := float64(earlyTotalTime) / float64(len(earlier))

		// Fatigue: accuracy drops AND response time increases
		if recentRate < earlyRate-0.2 && avgRecentTime > avgEarlyTime*1.3 {
			fatigue = "high"
		} else if recentRate < earlyRate-0.1 || avgRecentTime > avgEarlyTime*1.2 {
			fatigue = "moderate"
		}
	}

	// Frustration: consecutive failures + low confidence
	consecutiveFailures := 0
	lowConfidenceCount := 0
	for _, i := range window {
		if !i.Success {
			consecutiveFailures++
			if i.Confidence < 0.3 {
				lowConfidenceCount++
			}
		} else {
			break
		}
	}

	if consecutiveFailures >= 3 && lowConfidenceCount >= 2 {
		frustration = "high"
	} else if consecutiveFailures >= 2 && lowConfidenceCount >= 1 {
		frustration = "moderate"
	}

	return
}
