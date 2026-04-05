package algorithms

import "math"

const (
	pfaBetaSuccess = 0.11
	pfaBetaFailure = -0.11
)

type PFAState struct {
	Successes float64
	Failures  float64
}

func PFAUpdate(state PFAState, success bool) PFAState {
	result := state
	if success { result.Successes++ } else { result.Failures++ }
	return result
}

func PFAScore(state PFAState) float64 {
	return pfaBetaSuccess*state.Successes + pfaBetaFailure*state.Failures
}

func PFADetectPlateau(recentScores []float64, minCount int) bool {
	if len(recentScores) < minCount { return false }
	scores := recentScores[len(recentScores)-minCount:]
	maxDelta := 0.0
	for i := 1; i < len(scores); i++ {
		delta := math.Abs(scores[i] - scores[i-1])
		if delta > maxDelta { maxDelta = delta }
	}
	return maxDelta < 0.011
}
