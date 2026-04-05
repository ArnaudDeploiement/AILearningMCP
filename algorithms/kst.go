package algorithms

const KSTMasteryThreshold = 0.70

type KSTGraph struct {
	Concepts      []string            `json:"concepts"`
	Prerequisites map[string][]string `json:"prerequisites"`
}

func ComputeFrontier(graph KSTGraph, mastery map[string]float64) []string {
	var frontier []string
	for _, concept := range graph.Concepts {
		if mastery[concept] >= KSTMasteryThreshold { continue }
		prereqs := graph.Prerequisites[concept]
		allMet := true
		for _, prereq := range prereqs {
			if mastery[prereq] < KSTMasteryThreshold { allMet = false; break }
		}
		if allMet { frontier = append(frontier, concept) }
	}
	return frontier
}

func ConceptStatus(graph KSTGraph, mastery map[string]float64, concept string) string {
	if mastery[concept] >= KSTMasteryThreshold { return "done" }
	prereqs := graph.Prerequisites[concept]
	for _, prereq := range prereqs {
		if mastery[prereq] < KSTMasteryThreshold { return "locked" }
	}
	return "current"
}
