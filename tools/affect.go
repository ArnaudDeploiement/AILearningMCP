package tools

import (
	"context"
	"fmt"
	"time"

	"learning-runtime/engine"
	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RecordAffectParams struct {
	SessionID           string `json:"session_id" jsonschema:"Identifiant unique de la session"`
	Energy              int    `json:"energy,omitempty" jsonschema:"Energie disponible: 1=fatigue, 2=neutre, 3=motive, 4=en feu"`
	Confidence          int    `json:"confidence,omitempty" jsonschema:"Confiance sur le sujet: 1=anxieux, 2=flou, 3=ok, 4=confiant"`
	Satisfaction        int    `json:"satisfaction,omitempty" jsonschema:"Ressenti global (fin de session): 1=frustrant, 2=difficile, 3=bien, 4=flow"`
	PerceivedDifficulty int    `json:"perceived_difficulty,omitempty" jsonschema:"Difficulte percue (fin de session): 1=trop dur, 2=challengeant, 3=ok, 4=trop facile"`
	NextSessionIntent   int    `json:"next_session_intent,omitempty" jsonschema:"Prochaine session: 1=maintenant, 2=demain, 3=cette semaine, 4=je sais pas"`
}

func registerRecordAffect(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "record_affect",
		Description: "Enregistre l'etat emotionnel de l'apprenant. Appeler en debut de session (energy, confidence) et en fin (satisfaction, perceived_difficulty, next_session_intent).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params RecordAffectParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.SessionID == "" {
			r, _ := errorResult("session_id is required")
			return r, nil, nil
		}

		affect := &models.AffectState{
			LearnerID:           learnerID,
			SessionID:           params.SessionID,
			Energy:              params.Energy,
			SubjectConfidence:   params.Confidence,
			Satisfaction:        params.Satisfaction,
			PerceivedDifficulty: params.PerceivedDifficulty,
			NextSessionIntent:   params.NextSessionIntent,
		}

		if err := deps.Store.UpsertAffectState(affect); err != nil {
			r, _ := errorResult(fmt.Sprintf("failed to record affect: %v", err))
			return r, nil, nil
		}

		saved, err := deps.Store.GetAffectBySession(learnerID, params.SessionID)
		if err != nil {
			saved = affect
		}

		result := map[string]interface{}{
			"affect_state": saved,
		}

		// Compute tutor_mode_override from start-of-session affect
		if saved.SubjectConfidence == 1 {
			result["tutor_mode_override"] = "scaffolding"
		} else if saved.Energy == 1 {
			result["tutor_mode_override"] = "lighter"
		}

		// End-of-session: compute calibration_bias_delta
		if params.Satisfaction > 0 && params.PerceivedDifficulty > 0 {
			perceivedAbility := float64(params.PerceivedDifficulty) / 4.0
			sessionInteractions, _ := deps.Store.GetSessionInteractions(learnerID)
			if len(sessionInteractions) > 0 {
				successes := 0
				for _, i := range sessionInteractions {
					if i.Success {
						successes++
					}
				}
				actualRate := float64(successes) / float64(len(sessionInteractions))
				calibDelta := perceivedAbility - actualRate
				result["calibration_bias_delta"] = calibDelta
			}

			// Compute and persist autonomy score
			since := time.Now().UTC().Add(-30 * 24 * time.Hour)
			allInteractions, _ := deps.Store.GetInteractionsSince(learnerID, since)
			allStates, _ := deps.Store.GetConceptStatesByLearner(learnerID)
			calibBias, _ := deps.Store.GetCalibrationBias(learnerID, 20)

			autonomy := engine.ComputeAutonomyMetrics(engine.AutonomyInput{
				Interactions:    allInteractions,
				ConceptStates:   allStates,
				CalibrationBias: calibBias,
				SessionGap:      2 * time.Hour,
			})

			_ = deps.Store.UpdateAffectAutonomyScore(learnerID, params.SessionID, autonomy.Score)
			result["autonomy_score"] = autonomy.Score
		}

		r, _ := jsonResult(result)
		return r, nil, nil
	})
}
