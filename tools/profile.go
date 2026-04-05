package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type UpdateLearnerProfileParams struct {
	Device          string  `json:"device,omitempty" jsonschema:"Appareil principal (ex: laptop, phone, tablet)"`
	Background      string  `json:"background,omitempty" jsonschema:"Contexte professionnel ou academique de l'apprenant"`
	Style           string  `json:"learning_style,omitempty" jsonschema:"Style d'apprentissage prefere (ex: visuel, pratique, theorique)"`
	Objective       string  `json:"objective,omitempty" jsonschema:"Objectif d'apprentissage mis a jour"`
	Language        string  `json:"language,omitempty" jsonschema:"Langue preferee pour les exercices"`
	Level           string  `json:"level,omitempty" jsonschema:"Niveau actuel (debutant, intermediaire, avance)"`
	CalibrationBias float64 `json:"calibration_bias,omitempty" jsonschema:"Biais de calibration (positif=sur-estime, negatif=sous-estime)"`
	AffectBaseline  string  `json:"affect_baseline,omitempty" jsonschema:"Baseline emotionnelle de l'apprenant"`
	AutonomyScore   float64 `json:"autonomy_score,omitempty" jsonschema:"Score d'autonomie actuel (0-1)"`
}

func registerUpdateLearnerProfile(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_learner_profile",
		Description: "Met a jour les metadonnees persistantes de l'apprenant (device, background, style, objectif, niveau). Seuls les champs fournis sont modifies.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params UpdateLearnerProfileParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		learner, err := deps.Store.GetLearnerByID(learnerID)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("learner not found: %v", err))
			return r, nil, nil
		}

		// Load existing profile
		profile := make(map[string]interface{})
		if learner.ProfileJSON != "" && learner.ProfileJSON != "{}" {
			_ = json.Unmarshal([]byte(learner.ProfileJSON), &profile)
		}

		// Merge only non-empty fields
		updated := 0
		if params.Device != "" {
			profile["device"] = params.Device
			updated++
		}
		if params.Background != "" {
			profile["background"] = params.Background
			updated++
		}
		if params.Style != "" {
			profile["learning_style"] = params.Style
			updated++
		}
		if params.Language != "" {
			profile["language"] = params.Language
			updated++
		}
		if params.Level != "" {
			profile["level"] = params.Level
			updated++
		}
		if params.CalibrationBias != 0 {
			profile["calibration_bias"] = params.CalibrationBias
			updated++
		}
		if params.AffectBaseline != "" {
			profile["affect_baseline"] = params.AffectBaseline
			updated++
		}
		if params.AutonomyScore != 0 {
			profile["autonomy_score"] = params.AutonomyScore
			updated++
		}

		// Update objective on learner record if provided
		if params.Objective != "" {
			profile["objective"] = params.Objective
			updated++
		}

		profileJSON, _ := json.Marshal(profile)
		if err := deps.Store.UpdateLearnerProfile(learnerID, string(profileJSON)); err != nil {
			r, _ := errorResult(fmt.Sprintf("failed to update profile: %v", err))
			return r, nil, nil
		}

		r, _ := jsonResult(map[string]interface{}{
			"updated":       true,
			"fields_changed": updated,
			"profile":       profile,
		})
		return r, nil, nil
	})
}
