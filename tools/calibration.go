// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CalibrationCheckParams struct {
	ConceptID        string  `json:"concept_id" jsonschema:"Le concept à évaluer"`
	PredictedMastery float64 `json:"predicted_mastery" jsonschema:"Auto-évaluation de l'apprenant: 1=aucune maîtrise, 5=maîtrise parfaite"`
	DomainID         string  `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
}

func registerCalibrationCheck(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "calibration_check",
		Description: "Enregistre l'auto-évaluation de l'apprenant sur un concept avant un exercice. Retourne un prediction_id pour comparaison post-exercice.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params CalibrationCheckParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("calibration_check: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.ConceptID == "" {
			r, _ := errorResult("concept_id is required")
			return r, nil, nil
		}

		predicted := (params.PredictedMastery - 1.0) / 4.0
		if predicted < 0 {
			predicted = 0
		}
		if predicted > 1 {
			predicted = 1
		}

		predictionID := generatePredictionID()
		record := &models.CalibrationRecord{
			PredictionID: predictionID,
			LearnerID:    learnerID,
			ConceptID:    params.ConceptID,
			Predicted:    predicted,
		}

		if err := deps.Store.CreateCalibrationPrediction(record); err != nil {
			deps.Logger.Error("calibration_check: failed to create calibration prediction", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to create calibration: %v", err))
			return r, nil, nil
		}

		promptText := fmt.Sprintf(
			"Tu as estimé ta maîtrise de '%s' à %.0f/5. Voyons ça avec un exercice.",
			params.ConceptID, params.PredictedMastery,
		)

		r, _ := jsonResult(map[string]interface{}{
			"prediction_id": predictionID,
			"prompt_text":   promptText,
		})
		return r, nil, nil
	})
}

type RecordCalibrationResultParams struct {
	PredictionID string  `json:"prediction_id" jsonschema:"ID de la prédiction retournée par calibration_check"`
	ActualScore  float64 `json:"actual_score" jsonschema:"Score réel entre 0 et 1 (0=échec total, 1=réussite parfaite)"`
}

func registerRecordCalibrationResult(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "record_calibration_result",
		Description: "Compare la prédiction de l'apprenant avec le résultat réel. Met à jour le biais de calibration.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params RecordCalibrationResultParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("record_calibration_result: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if params.PredictionID == "" {
			r, _ := errorResult("prediction_id is required")
			return r, nil, nil
		}

		record, err := deps.Store.GetCalibrationRecord(params.PredictionID)
		if err != nil {
			deps.Logger.Error("record_calibration_result: calibration record not found", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("prediction not found: %v", err))
			return r, nil, nil
		}
		if record.LearnerID != learnerID {
			deps.Logger.Warn("record_calibration_result: learner mismatch", "prediction_id", params.PredictionID, "learner", learnerID, "owner", record.LearnerID)
			r, _ := errorResult("calibration record not found")
			return r, nil, nil
		}

		delta := record.Predicted - params.ActualScore

		if err := deps.Store.CompleteCalibrationRecord(params.PredictionID, params.ActualScore, delta); err != nil {
			deps.Logger.Error("record_calibration_result: failed to complete calibration record", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to record result: %v", err))
			return r, nil, nil
		}

		bias, _ := deps.Store.GetCalibrationBias(learnerID, 20)

		r, _ := jsonResult(map[string]interface{}{
			"delta":                    delta,
			"calibration_bias_updated": bias,
		})
		return r, nil, nil
	})
}

func generatePredictionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "cal_" + base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
}
