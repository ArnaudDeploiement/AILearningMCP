package engine

import (
	"fmt"

	"learning-runtime/algorithms"
	"learning-runtime/models"
)

// Route determines the next optimal activity.
// sessionConcepts tracks concepts already practiced in the current session (concept → count).
// Critical alerts (FORGETTING, ZPD_DRIFT) bypass session dedup.
func Route(alerts []models.Alert, frontier []string, states []*models.ConceptState, recentInteractions []*models.Interaction, sessionConcepts map[string]int) models.Activity {
	// Priority 1: FORGETTING critical — always takes precedence, even if practiced this session
	for _, a := range alerts {
		if a.Type == models.AlertForgetting && a.Urgency == models.UrgencyCritical {
			return models.Activity{
				Type: models.ActivityRecall, Concept: a.Concept, DifficultyTarget: 0.65,
				Format: "code_completion", EstimatedMinutes: 8,
				Rationale:    fmt.Sprintf("FSRS prescrit revision · retention a %.0f%%", a.Retention*100),
				PromptForLLM: fmt.Sprintf("Genere un exercice de revision sur %s. Niveau: intermediaire. L'objectif est de reactiver la memoire, pas de tester des connaissances nouvelles. Format: completion de code ou question directe.", a.Concept),
			}
		}
	}
	// Priority 2: ZPD_DRIFT — always takes precedence
	for _, a := range alerts {
		if a.Type == models.AlertZPDDrift {
			return models.Activity{
				Type: models.ActivityRecall, Concept: a.Concept, DifficultyTarget: 0.40,
				Format: "guided_exercise", EstimatedMinutes: 10,
				Rationale:    fmt.Sprintf("ZPD drift detecte · taux d'erreur %.0f%%", a.ErrorRate*100),
				PromptForLLM: fmt.Sprintf("Genere un exercice simplifie sur %s. Reduis la complexite, utilise des indices, et guide l'apprenant pas a pas.", a.Concept),
			}
		}
	}
	// Priority 3: PLATEAU — skip if concept already practiced 2+ times this session
	for _, a := range alerts {
		if a.Type == models.AlertPlateau {
			if sessionConcepts[a.Concept] >= 2 {
				continue
			}
			return models.Activity{
				Type: models.ActivityDebuggingCase, Concept: a.Concept, DifficultyTarget: 0.60,
				Format: "debugging", EstimatedMinutes: 15,
				Rationale:    fmt.Sprintf("plateau detecte depuis %d sessions", a.SessionsStalled),
				PromptForLLM: fmt.Sprintf("Genere un cas de debugging reel sur %s. Presente du code casse a corriger.", a.Concept),
			}
		}
	}
	// Priority 4: OVERLOAD
	for _, a := range alerts {
		if a.Type == models.AlertOverload {
			return models.Activity{Type: models.ActivityRest, Rationale: "session > 45 minutes",
				PromptForLLM: "L'apprenant a travaille plus de 45 minutes. Suggere une pause et un resume."}
		}
	}
	// Priority 5: MASTERY_READY — skip if already practiced this session
	for _, a := range alerts {
		if a.Type == models.AlertMasteryReady {
			if sessionConcepts[a.Concept] >= 1 {
				continue
			}
			return models.Activity{
				Type: models.ActivityMasteryChallenge, Concept: a.Concept, DifficultyTarget: 0.75,
				Format: "build_challenge", EstimatedMinutes: 45,
				Rationale:    "BKT >= 0.85 · mastery challenge eligible",
				PromptForLLM: fmt.Sprintf("Genere un mastery challenge sur %s. L'apprenant doit construire quelque chose de complet. Evalue le transfert, pas la syntaxe.", a.Concept),
			}
		}
	}
	// Priority 6: New concept from frontier — skip concepts already introduced this session
	for _, concept := range frontier {
		if sessionConcepts[concept] >= 1 {
			continue
		}
		return models.Activity{
			Type: models.ActivityNewConcept, Concept: concept, DifficultyTarget: 0.55,
			Format: "introduction", EstimatedMinutes: 15,
			Rationale:    "prerequis valides · nouveau concept accessible",
			PromptForLLM: fmt.Sprintf("Introduis le concept %s. Commence par une explication claire avec un exemple concret, puis propose un premier exercice simple.", concept),
		}
	}
	// Priority 7: Default recall on lowest retention — prefer unpracticed concepts this session
	if len(states) > 0 {
		var bestUnpracticed, bestAny *models.ConceptState
		bestUnpracticedRet, bestAnyRet := 1.1, 1.1
		for _, cs := range states {
			if cs.CardState == "new" {
				continue
			}
			r := algorithms.Retrievability(cs.ElapsedDays, cs.Stability)
			if r < bestAnyRet {
				bestAnyRet = r
				bestAny = cs
			}
			if sessionConcepts[cs.Concept] == 0 && r < bestUnpracticedRet {
				bestUnpracticedRet = r
				bestUnpracticed = cs
			}
		}
		// Prefer unpracticed concept; fall back to any concept
		chosen := bestUnpracticed
		chosenRet := bestUnpracticedRet
		if chosen == nil {
			chosen = bestAny
			chosenRet = bestAnyRet
		}
		if chosen != nil {
			return models.Activity{
				Type: models.ActivityRecall, Concept: chosen.Concept, DifficultyTarget: 0.65,
				Format: "mixed", EstimatedMinutes: 8,
				Rationale:    fmt.Sprintf("revision · retention la plus basse a %.0f%%", chosenRet*100),
				PromptForLLM: fmt.Sprintf("Genere un exercice de revision sur %s. Varie le format.", chosen.Concept),
			}
		}
	}
	return models.Activity{Type: models.ActivityRest, Rationale: "aucune activite disponible",
		PromptForLLM: "Aucune activite planifiee. Demande a l'apprenant quel sujet il souhaite explorer."}
}
