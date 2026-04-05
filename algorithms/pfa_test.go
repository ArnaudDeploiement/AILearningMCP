package algorithms

import "testing"

func TestPFAUpdate(t *testing.T) {
	state := PFAState{Successes: 0, Failures: 0}
	state = PFAUpdate(state, true)
	if !approxEqual(state.Successes, 1.0, 0.001) { t.Errorf("successes = %f, want 1", state.Successes) }
	score := PFAScore(state)
	if !approxEqual(score, 0.11, 0.01) { t.Errorf("score = %f, want ~0.11", score) }
	state = PFAUpdate(state, false)
	score = PFAScore(state)
	if !approxEqual(score, 0.0, 0.01) { t.Errorf("score = %f, want ~0.0", score) }
}

func TestPFADetectPlateau(t *testing.T) {
	if !PFADetectPlateau([]float64{0.55, 0.56, 0.555, 0.558}, 4) { t.Error("should detect plateau") }
	if PFADetectPlateau([]float64{0.3, 0.45, 0.6, 0.75}, 4) { t.Error("should NOT detect plateau") }
	if PFADetectPlateau([]float64{0.5, 0.51, 0.52}, 4) { t.Error("should NOT detect plateau with < 4") }
}
