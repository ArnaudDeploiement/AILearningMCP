package engine

import (
	"fmt"
	"math"
	"sort"
	"time"

	"learning-runtime/models"
)

// AutonomyInput holds the data needed to compute autonomy metrics.
type AutonomyInput struct {
	Interactions    []*models.Interaction
	ConceptStates   []*models.ConceptState
	CalibrationBias float64
	SessionGap      time.Duration // default: 2h
}

// ComputeAutonomyMetrics computes the four autonomy components.
// Each component is 25% of the final score.
func ComputeAutonomyMetrics(input AutonomyInput) models.AutonomyMetrics {
	now := time.Now().UTC()

	// 1. Initiative rate: % of sessions self-initiated
	initiativeRate := 0.0
	sessions := groupIntoSessions(input.Interactions, input.SessionGap)
	if len(sessions) > 0 {
		selfInitCount := 0
		for _, s := range sessions {
			if len(s) > 0 && s[0].SelfInitiated {
				selfInitCount++
			}
		}
		initiativeRate = float64(selfInitCount) / float64(len(sessions))
	}

	// 2. Calibration accuracy: 1 - |bias| clamped to [0, 1]
	calibrationAccuracy := 1.0 - math.Min(math.Abs(input.CalibrationBias), 1.0)

	// 3. Hint independence: 1 - (hints on mastered / total on mastered)
	hintIndependence := 1.0
	if len(input.ConceptStates) > 0 {
		masteredConcepts := make(map[string]bool)
		for _, cs := range input.ConceptStates {
			if cs.PMastery >= 0.80 {
				masteredConcepts[cs.Concept] = true
			}
		}
		if len(masteredConcepts) > 0 {
			totalOnMastered := 0
			hintsOnMastered := 0
			for _, i := range input.Interactions {
				if masteredConcepts[i.Concept] {
					totalOnMastered++
					hintsOnMastered += i.HintsRequested
				}
			}
			if totalOnMastered > 0 {
				hintRatio := float64(hintsOnMastered) / float64(totalOnMastered)
				hintIndependence = 1.0 - math.Min(hintRatio, 1.0)
			}
		}
	}

	// 4. Proactive review rate: % of review interactions that were proactive
	proactiveRate := 0.0
	reviewCount := 0
	proactiveCount := 0
	for _, i := range input.Interactions {
		if i.ActivityType != "NEW_CONCEPT" && i.ActivityType != "REST" && i.ActivityType != "SETUP_DOMAIN" {
			reviewCount++
			if i.IsProactiveReview {
				proactiveCount++
			}
		}
	}
	if reviewCount > 0 {
		proactiveRate = float64(proactiveCount) / float64(reviewCount)
	}

	score := (initiativeRate + calibrationAccuracy + hintIndependence + proactiveRate) / 4.0

	return models.AutonomyMetrics{
		Score:               score,
		InitiativeRate:      initiativeRate,
		CalibrationAccuracy: calibrationAccuracy,
		HintIndependence:    hintIndependence,
		ProactiveReviewRate: proactiveRate,
		ComputedAt:          now,
	}
}

// groupIntoSessions splits interactions into sessions separated by gaps > sessionGap.
// Interactions are sorted oldest-first internally.
func groupIntoSessions(interactions []*models.Interaction, gap time.Duration) [][]*models.Interaction {
	if len(interactions) == 0 {
		return nil
	}

	sorted := make([]*models.Interaction, len(interactions))
	copy(sorted, interactions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	var sessions [][]*models.Interaction
	var current []*models.Interaction

	for _, i := range sorted {
		if len(current) > 0 && i.CreatedAt.Sub(current[len(current)-1].CreatedAt) > gap {
			sessions = append(sessions, current)
			current = nil
		}
		current = append(current, i)
	}
	if len(current) > 0 {
		sessions = append(sessions, current)
	}
	return sessions
}

// GroupIntoSessionsExported is the exported version of groupIntoSessions.
func GroupIntoSessionsExported(interactions []*models.Interaction, gap time.Duration) [][]*models.Interaction {
	return groupIntoSessions(interactions, gap)
}

// computeAutonomyTrend compares the last 5 scores to the 5 before that.
// scores are in newest-first order.
func computeAutonomyTrend(scores []float64) string {
	if len(scores) < 6 {
		return "stable"
	}

	recentN := 5
	if len(scores) < 10 {
		recentN = len(scores) / 2
	}

	recentSum := 0.0
	for i := 0; i < recentN; i++ {
		recentSum += scores[i]
	}
	recentAvg := recentSum / float64(recentN)

	previousSum := 0.0
	previousN := recentN
	if len(scores) < 2*recentN {
		previousN = len(scores) - recentN
	}
	for i := recentN; i < recentN+previousN; i++ {
		previousSum += scores[i]
	}
	previousAvg := previousSum / float64(previousN)

	diff := recentAvg - previousAvg
	if diff > 0.05 {
		return "improving"
	}
	if diff < -0.05 {
		return "declining"
	}
	return "stable"
}

// ComputeAutonomyTrendExported is the exported version of computeAutonomyTrend.
func ComputeAutonomyTrendExported(scores []float64) string {
	return computeAutonomyTrend(scores)
}

// ─── Metacognitive Mirror ───────────────────────────────────────────────────

// MirrorInput holds data for metacognitive mirror pattern detection.
type MirrorInput struct {
	Interactions    []*models.Interaction
	ConceptStates   []*models.ConceptState
	AutonomyScores  []float64 // newest-first from affect_states.autonomy_score
	CalibrationBias float64
	SessionCount    int
}

// DetectMirrorPattern returns a mirror message if a dependency pattern is consolidated
// over 3+ sessions. Returns nil if no pattern detected.
func DetectMirrorPattern(input MirrorInput) *models.MirrorMessage {
	if input.SessionCount < 3 {
		return nil
	}

	// Priority 1: dependency_increasing — autonomy declining over 3 consecutive sessions
	if len(input.AutonomyScores) >= 3 {
		declining := true
		for i := 0; i < len(input.AutonomyScores)-1 && i < 2; i++ {
			if input.AutonomyScores[i] >= input.AutonomyScores[i+1] {
				declining = false
				break
			}
		}
		if declining {
			return &models.MirrorMessage{
				Pattern:      "dependency_increasing",
				Message:      "Ton score d'autonomie a baisse sur les 3 dernieres sessions.",
				OpenQuestion: "Est-ce que tu sens que tu as besoin de plus de guidage en ce moment, ou est-ce que tu voudrais essayer de travailler plus en autonomie ?",
			}
		}
	}

	// Priority 2: hint_overuse — hints on mastered concepts (BKT >= 0.80)
	masteredConcepts := make(map[string]bool)
	for _, cs := range input.ConceptStates {
		if cs.PMastery >= 0.80 {
			masteredConcepts[cs.Concept] = true
		}
	}
	if len(masteredConcepts) > 0 {
		hintsOnMastered := 0
		totalOnMastered := 0
		for _, i := range input.Interactions {
			if masteredConcepts[i.Concept] {
				totalOnMastered++
				hintsOnMastered += i.HintsRequested
			}
		}
		if totalOnMastered >= 5 && float64(hintsOnMastered)/float64(totalOnMastered) > 0.5 {
			return &models.MirrorMessage{
				Pattern:      "hint_overuse",
				Message:      "Tu demandes souvent des indices sur des concepts que tu maitrises deja.",
				OpenQuestion: "Est-ce que c'est par reflexe, ou est-ce qu'il y a un aspect de ces concepts qui te semble encore flou ?",
			}
		}
	}

	// Priority 3: no_initiative — all sessions started after alert (no self_initiated)
	if input.SessionCount >= 3 {
		hasSelfInitiated := false
		for _, i := range input.Interactions {
			if i.SelfInitiated {
				hasSelfInitiated = true
				break
			}
		}
		if !hasSelfInitiated {
			return &models.MirrorMessage{
				Pattern:      "no_initiative",
				Message:      "Toutes tes sessions recentes ont ete declenchees par une notification.",
				OpenQuestion: "Est-ce que tu preferes que le systeme te rappelle, ou est-ce que tu aimerais definir tes propres moments d'apprentissage ?",
			}
		}
	}

	// Priority 4: calibration_drift — bias increasing over 5 sessions
	if math.Abs(input.CalibrationBias) > 1.0 && input.SessionCount >= 5 {
		direction := "sur-estimer"
		if input.CalibrationBias < 0 {
			direction = "sous-estimer"
		}
		return &models.MirrorMessage{
			Pattern:      "calibration_drift",
			Message:      fmt.Sprintf("Tu as tendance a %s ton niveau de facon recurrente.", direction),
			OpenQuestion: "Est-ce que tu veux qu'on travaille avec des auto-evaluations plus frequentes pour affiner ta perception ?",
		}
	}

	return nil
}

// ─── Tutor Mode ─────────────────────────────────────────────────────────────

// ComputeTutorMode determines the tutor communication mode from affect and alerts.
func ComputeTutorMode(affect *models.AffectState, alerts []models.Alert) string {
	if affect == nil {
		return "normal"
	}

	hasAffectNegative := false
	for _, a := range alerts {
		if a.Type == models.AlertAffectNegative {
			hasAffectNegative = true
			break
		}
	}

	// Affect negative: bored (high energy + low satisfaction) → recontextualize
	if hasAffectNegative && affect.Energy >= 3 && affect.Satisfaction <= 2 {
		return "recontextualize"
	}

	// Affect negative: frustration → lighter
	if hasAffectNegative && affect.Satisfaction <= 2 {
		return "lighter"
	}

	// Start-of-session: anxious → scaffolding
	if affect.SubjectConfidence == 1 {
		return "scaffolding"
	}

	// Start-of-session: fatigued → lighter
	if affect.Energy == 1 {
		return "lighter"
	}

	return "normal"
}
