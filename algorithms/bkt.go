package algorithms

type BKTState struct {
	PMastery float64
	PLearn   float64
	PForget  float64
	PSlip    float64
	PGuess   float64
}

const BKTMasteryThreshold = 0.85

func BKTUpdate(state BKTState, correct bool) BKTState {
	var pMasteryGivenObs float64
	if correct {
		pCorrectMastery := 1.0 - state.PSlip
		pCorrectNotMastery := state.PGuess
		pCorrect := pCorrectMastery*state.PMastery + pCorrectNotMastery*(1-state.PMastery)
		pMasteryGivenObs = pCorrectMastery * state.PMastery / pCorrect
	} else {
		pIncorrectMastery := state.PSlip
		pIncorrectNotMastery := 1.0 - state.PGuess
		pIncorrect := pIncorrectMastery*state.PMastery + pIncorrectNotMastery*(1-state.PMastery)
		pMasteryGivenObs = pIncorrectMastery * state.PMastery / pIncorrect
	}
	newPMastery := pMasteryGivenObs*(1-state.PForget) + (1-pMasteryGivenObs)*state.PLearn
	result := state
	result.PMastery = clamp(newPMastery, 0, 1)
	return result
}

// BKTUpdateWithErrorType adjusts BKT parameters based on error type before updating.
// SYNTAX_ERROR: careless mistake — higher slip, less penalizing to mastery.
// KNOWLEDGE_GAP: genuine lack of understanding — lower guess, more penalizing.
// LOGIC_ERROR or empty: standard BKT update.
func BKTUpdateWithErrorType(state BKTState, correct bool, errorType string) BKTState {
	if correct || errorType == "" {
		return BKTUpdate(state, correct)
	}

	adjusted := state
	switch errorType {
	case "SYNTAX_ERROR":
		// Syntax errors indicate carelessness, not lack of knowledge.
		// Temporarily boost PSlip to reduce mastery penalty.
		adjusted.PSlip = clamp(state.PSlip+0.15, 0, 0.5)
	case "KNOWLEDGE_GAP":
		// Genuine knowledge gap — reduce PGuess to penalize more.
		adjusted.PGuess = clamp(state.PGuess-0.10, 0.05, 0.5)
	}
	// LOGIC_ERROR uses standard parameters

	return BKTUpdate(adjusted, correct)
}

func BKTIsMastered(state BKTState) bool {
	return state.PMastery >= BKTMasteryThreshold
}
