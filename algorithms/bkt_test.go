package algorithms

import "testing"

func TestBKTUpdateCorrect(t *testing.T) {
	state := BKTState{PMastery: 0.5, PLearn: 0.3, PForget: 0.05, PSlip: 0.1, PGuess: 0.2}
	updated := BKTUpdate(state, true)
	if !approxEqual(updated.PMastery, 0.8318, 0.01) {
		t.Errorf("PMastery after correct = %f, want ~0.8318", updated.PMastery)
	}
}

func TestBKTUpdateIncorrect(t *testing.T) {
	state := BKTState{PMastery: 0.5, PLearn: 0.3, PForget: 0.05, PSlip: 0.1, PGuess: 0.2}
	updated := BKTUpdate(state, false)
	if !approxEqual(updated.PMastery, 0.3722, 0.01) {
		t.Errorf("PMastery after incorrect = %f, want ~0.3722", updated.PMastery)
	}
}

func TestBKTMasteryThreshold(t *testing.T) {
	state := BKTState{PMastery: 0.86}
	if !BKTIsMastered(state) { t.Error("0.86 should be mastered") }
	state.PMastery = 0.84
	if BKTIsMastered(state) { t.Error("0.84 should NOT be mastered") }
}

func TestBKTConvergence(t *testing.T) {
	state := BKTState{PMastery: 0.1, PLearn: 0.3, PForget: 0.05, PSlip: 0.1, PGuess: 0.2}
	for i := 0; i < 10; i++ { state = BKTUpdate(state, true) }
	if state.PMastery < 0.85 { t.Errorf("after 10 correct, PMastery = %f, want >= 0.85", state.PMastery) }
}
