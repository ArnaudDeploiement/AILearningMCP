package algorithms

import "testing"

func TestIRTProbability(t *testing.T) {
	tests := []struct {
		theta, difficulty, discrimination, want float64
	}{
		{0, 0, 1, 0.5},
		{1, 0, 1, 0.7311},
		{0, 1, 1, 0.2689},
		{2, 0, 2, 0.9820},
	}
	for _, tt := range tests {
		got := IRTProbability(tt.theta, tt.difficulty, tt.discrimination)
		if !approxEqual(got, tt.want, 0.01) {
			t.Errorf("IRTProbability(%.1f, %.1f, %.1f) = %.4f, want ~%.4f", tt.theta, tt.difficulty, tt.discrimination, got, tt.want)
		}
	}
}

func TestIRTUpdateTheta(t *testing.T) {
	items := []IRTItem{{Difficulty: 0, Discrimination: 1}, {Difficulty: 0.5, Discrimination: 1}}
	newTheta := IRTUpdateTheta(0, items, []bool{true, true})
	if newTheta <= 0 { t.Errorf("theta should increase after all correct, got %f", newTheta) }
	newTheta = IRTUpdateTheta(0, items, []bool{false, false})
	if newTheta >= 0 { t.Errorf("theta should decrease after all incorrect, got %f", newTheta) }
}

func TestIRTIsInZPD(t *testing.T) {
	if !IRTIsInZPD(0.65) { t.Error("0.65 should be in ZPD") }
	if IRTIsInZPD(0.90) { t.Error("0.90 should NOT be in ZPD") }
	if IRTIsInZPD(0.40) { t.Error("0.40 should NOT be in ZPD") }
}
