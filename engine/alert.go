package engine

import (
	"fmt"
	"math"
	"time"

	"learning-runtime/algorithms"
	"learning-runtime/models"
)

func ComputeAlerts(states []*models.ConceptState, recentInteractions []*models.Interaction, sessionStart time.Time) []models.Alert {
	var alerts []models.Alert

	for _, cs := range states {
		if cs.CardState == "new" {
			continue
		}

		// FORGETTING: FSRS retention < 40%
		elapsed := cs.ElapsedDays
		if cs.LastReview != nil {
			elapsed = int(time.Since(*cs.LastReview).Hours() / 24)
		}
		retention := algorithms.Retrievability(elapsed, cs.Stability)
		if retention < 0.40 {
			urgency := models.UrgencyWarning
			if retention < 0.30 {
				urgency = models.UrgencyCritical
			}
			hoursLeft := 0.0
			if retention > 0.30 {
				hoursLeft = (retention - 0.30) / 0.01 * 2
			}
			alerts = append(alerts, models.Alert{
				Type:               models.AlertForgetting,
				Concept:            cs.Concept,
				Urgency:            urgency,
				Retention:          retention,
				HoursUntilCritical: hoursLeft,
				RecommendedAction:  fmt.Sprintf("revision immediate · %d minutes", estimateReviewMinutes(cs)),
			})
		}

		// MASTERY_READY: BKT >= 0.85
		if cs.PMastery >= algorithms.BKTMasteryThreshold {
			alerts = append(alerts, models.Alert{
				Type:              models.AlertMasteryReady,
				Concept:           cs.Concept,
				Urgency:           models.UrgencyInfo,
				RecommendedAction: "mastery challenge disponible",
			})
		}
	}

	// ZPD_DRIFT: 3+ consecutive failures on same concept (check from most recent)
	conceptFailStreaks := make(map[string]int)
	conceptProcessed := make(map[string]bool)
	for _, i := range recentInteractions {
		if conceptProcessed[i.Concept] {
			continue
		}
		if !i.Success {
			conceptFailStreaks[i.Concept]++
		} else {
			conceptProcessed[i.Concept] = true
		}
	}
	for concept, streak := range conceptFailStreaks {
		if streak >= 3 {
			errorRate := float64(streak) / float64(streak+1)
			alerts = append(alerts, models.Alert{
				Type:              models.AlertZPDDrift,
				Concept:           concept,
				Urgency:           models.UrgencyWarning,
				ErrorRate:         errorRate,
				RecommendedAction: "reduire la difficulte",
			})
		}
	}

	// PLATEAU: PFA score stagnation
	conceptInteractions := groupByConcept(recentInteractions)
	for concept, interactions := range conceptInteractions {
		if len(interactions) >= 4 {
			var scores []float64
			state := algorithms.PFAState{}
			for _, i := range interactions {
				state = algorithms.PFAUpdate(state, i.Success)
				scores = append(scores, algorithms.PFAScore(state))
			}
			if algorithms.PFADetectPlateau(scores, 4) {
				alerts = append(alerts, models.Alert{
					Type:              models.AlertPlateau,
					Concept:           concept,
					Urgency:           models.UrgencyWarning,
					SessionsStalled:   len(interactions),
					RecommendedAction: "changer de format · cas reel a debugger",
				})
			}
		}
	}

	// OVERLOAD: session > 45 min
	if !sessionStart.IsZero() && time.Since(sessionStart) > 45*time.Minute {
		alerts = append(alerts, models.Alert{
			Type:              models.AlertOverload,
			Urgency:           models.UrgencyInfo,
			RecommendedAction: "pause recommandee",
		})
	}

	return alerts
}

func estimateReviewMinutes(cs *models.ConceptState) int {
	if cs.Lapses > 2 {
		return 12
	}
	return 8
}

func groupByConcept(interactions []*models.Interaction) map[string][]*models.Interaction {
	m := make(map[string][]*models.Interaction)
	for _, i := range interactions {
		m[i.Concept] = append(m[i.Concept], i)
	}
	return m
}

// MetacognitiveAlertOptions holds optional data for metacognitive alerts.
type MetacognitiveAlertOptions struct {
	ConceptStates   []*models.ConceptState
	TransferRecords []*models.TransferRecord
}

type MetacognitiveAlertOption func(*MetacognitiveAlertOptions)

func WithTransferData(states []*models.ConceptState, transfers []*models.TransferRecord) MetacognitiveAlertOption {
	return func(o *MetacognitiveAlertOptions) {
		o.ConceptStates = states
		o.TransferRecords = transfers
	}
}

// ComputeMetacognitiveAlerts computes the 4 new metacognitive alerts.
func ComputeMetacognitiveAlerts(
	autonomyScores []float64,
	calibrationBias float64,
	recentAffects []*models.AffectState,
	interactions []*models.Interaction,
	opts ...MetacognitiveAlertOption,
) []models.Alert {
	var options MetacognitiveAlertOptions
	for _, o := range opts {
		o(&options)
	}

	var alerts []models.Alert

	// DEPENDENCY_INCREASING: autonomy_score declining over 3 consecutive sessions
	if len(autonomyScores) >= 3 {
		declining := true
		for i := 0; i < 2; i++ {
			if autonomyScores[i] >= autonomyScores[i+1] {
				declining = false
				break
			}
		}
		if declining {
			alerts = append(alerts, models.Alert{
				Type:              models.AlertDependencyIncreasing,
				Urgency:           models.UrgencyWarning,
				RecommendedAction: "miroir metacognitif active",
			})
		}
	}

	// CALIBRATION_DIVERGING: |calibration_bias| > 1.5
	if math.Abs(calibrationBias) > 1.5 {
		direction := "sur-estimation"
		if calibrationBias < 0 {
			direction = "sous-estimation"
		}
		alerts = append(alerts, models.Alert{
			Type:              models.AlertCalibrationDiverging,
			Urgency:           models.UrgencyWarning,
			RecommendedAction: fmt.Sprintf("calibration divergente: %s persistante", direction),
		})
	}

	// AFFECT_NEGATIVE: satisfaction ≤ 2 on 2 consecutive sessions
	if len(recentAffects) >= 2 {
		if recentAffects[0].Satisfaction > 0 && recentAffects[0].Satisfaction <= 2 &&
			recentAffects[1].Satisfaction > 0 && recentAffects[1].Satisfaction <= 2 {
			alerts = append(alerts, models.Alert{
				Type:              models.AlertAffectNegative,
				Urgency:           models.UrgencyWarning,
				RecommendedAction: "adapter le tutor_mode",
			})
		}
		// Also on perceived_difficulty = 1 on 2 consecutive
		if recentAffects[0].PerceivedDifficulty == 1 && recentAffects[1].PerceivedDifficulty == 1 {
			found := false
			for _, a := range alerts {
				if a.Type == models.AlertAffectNegative {
					found = true
					break
				}
			}
			if !found {
				alerts = append(alerts, models.Alert{
					Type:              models.AlertAffectNegative,
					Urgency:           models.UrgencyWarning,
					RecommendedAction: "adapter le tutor_mode",
				})
			}
		}
	}

	// TRANSFER_BLOCKED: BKT >= 0.85 but transfer_score < 0.50 on 2+ contexts
	if options.ConceptStates != nil && options.TransferRecords != nil {
		mastered := make(map[string]bool)
		for _, cs := range options.ConceptStates {
			if cs.PMastery >= 0.85 {
				mastered[cs.Concept] = true
			}
		}
		transferByConcept := make(map[string][]*models.TransferRecord)
		for _, tr := range options.TransferRecords {
			transferByConcept[tr.ConceptID] = append(transferByConcept[tr.ConceptID], tr)
		}
		for concept := range mastered {
			records := transferByConcept[concept]
			lowContexts := 0
			for _, tr := range records {
				if tr.Score < 0.50 {
					lowContexts++
				}
			}
			if lowContexts >= 2 {
				alerts = append(alerts, models.Alert{
					Type:              models.AlertTransferBlocked,
					Concept:           concept,
					Urgency:           models.UrgencyWarning,
					RecommendedAction: "feynman challenge recommande",
				})
			}
		}
	}

	return alerts
}
