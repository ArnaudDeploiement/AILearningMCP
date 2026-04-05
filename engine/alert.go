package engine

import (
	"fmt"
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
