package tools

import (
	"context"
	"fmt"
	"time"

	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ImplementationIntentionInput struct {
	Trigger      string `json:"trigger" jsonschema:"Clause 'quand' (ex: 'demain matin au cafe')"`
	Action       string `json:"action" jsonschema:"Clause 'alors je' (ex: 'ferai 1 exercice de derivees')"`
	ScheduledFor string `json:"scheduled_for,omitempty" jsonschema:"ISO 8601 timestamp optionnel (UTC)"`
}

type RecordSessionCloseParams struct {
	DomainID                 string                        `json:"domain_id,omitempty" jsonschema:"ID du domaine (optionnel)"`
	ImplementationIntention  *ImplementationIntentionInput `json:"implementation_intention,omitempty" jsonschema:"Engagement if-then optionnel (Gollwitzer)"`
}

func registerRecordSessionClose(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "record_session_close",
		Description: "Cloture la session : enregistre optionnellement une implementation intention (if-then) et retourne des signaux structures pour composer le message de fin (concepts pratiques, wins, intent prompt, message queue filler).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params RecordSessionCloseParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("record_session_close: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		domain, err := resolveDomain(deps.Store, learnerID, params.DomainID)
		if err != nil || domain == nil {
			deps.Logger.Error("record_session_close: domain resolution failed", "err", err, "learner", learnerID)
			r, _ := errorResult("domain not found")
			return r, nil, nil
		}

		// Persist the implementation intention if provided.
		if params.ImplementationIntention != nil &&
			params.ImplementationIntention.Trigger != "" &&
			params.ImplementationIntention.Action != "" {
			var scheduled time.Time
			if params.ImplementationIntention.ScheduledFor != "" {
				if parsed, perr := time.Parse(time.RFC3339, params.ImplementationIntention.ScheduledFor); perr == nil {
					scheduled = parsed
				}
			}
			if _, err := deps.Store.InsertImplementationIntention(
				learnerID, domain.ID,
				params.ImplementationIntention.Trigger,
				params.ImplementationIntention.Action,
				scheduled,
			); err != nil {
				deps.Logger.Error("record_session_close: insert intention failed", "err", err, "learner", learnerID)
			}
		}

		recap := buildRecapBrief(deps, learnerID, domain)
		r, _ := jsonResult(map[string]any{"recap_brief": recap})
		return r, nil, nil
	})
}

// buildRecapBrief produces session-close signals for Claude.
func buildRecapBrief(deps *Deps, learnerID string, domain *models.Domain) *models.RecapBrief {
	domainSet := make(map[string]bool, len(domain.Graph.Concepts))
	for _, c := range domain.Graph.Concepts {
		domainSet[c] = true
	}

	sessionInteractions, _ := deps.Store.GetSessionInteractions(learnerID)

	practicedSet := map[string]bool{}
	winsSet := map[string]bool{}
	strugglesSet := map[string]bool{}
	for _, i := range sessionInteractions {
		if !domainSet[i.Concept] {
			continue
		}
		practicedSet[i.Concept] = true
		if i.Success {
			winsSet[i.Concept] = true
		} else {
			strugglesSet[i.Concept] = true
		}
	}

	practiced := mapKeys(practicedSet)
	wins := mapKeys(winsSet)
	// "Interesting" struggles = failed but not completely blocked (partial progress signal heuristic:
	// the learner also had a success on the same concept during the session).
	var interesting []string
	for c := range strugglesSet {
		if winsSet[c] {
			interesting = append(interesting, c)
		}
	}

	// Next scheduled review — earliest next_review across domain states.
	states, _ := deps.Store.GetConceptStatesByLearner(learnerID)
	var next string
	var earliest time.Time
	for _, cs := range states {
		if !domainSet[cs.Concept] || cs.NextReview == nil {
			continue
		}
		if earliest.IsZero() || cs.NextReview.Before(earliest) {
			earliest = *cs.NextReview
			next = fmt.Sprintf("%s (%s)", cs.Concept, cs.NextReview.Format("02/01 15:04 UTC"))
		}
	}

	// Prompt for implementation intention if none recorded in last 7 days for any domain.
	has, _ := deps.Store.HasRecentImplementationIntention(learnerID, "", time.Now().UTC().Add(-7*24*time.Hour))
	promptIntent := !has

	instruction := "Clos la session en 2-3 phrases. Mentionne un win tangible ou une belle tentative. " +
		"Si prompt_for_implementation_intention est vrai, pose une question concrete du type 'Quand et ou tu pratiques ensuite ?' " +
		"et attends la reponse pour rappeler record_session_close avec implementation_intention. " +
		"Puis appelle queue_webhook_message 2 fois (daily_motivation pour demain 8h UTC, daily_recap pour demain 21h UTC) " +
		"avec un contenu chaleureux lie au personal_goal - sans KPI brut."

	return &models.RecapBrief{
		ConceptsPracticed:             practiced,
		Wins:                          wins,
		InterestingStruggles:          interesting,
		NextScheduledReview:           next,
		PromptForImplementationIntent: promptIntent,
		Instruction:                   instruction,
	}
}

func mapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
