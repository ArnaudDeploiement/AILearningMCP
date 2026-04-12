package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"learning-runtime/db"
	"learning-runtime/models"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	store  *db.Store
	cron   *cron.Cron
	logger *slog.Logger
	client *http.Client
}

func NewScheduler(store *db.Store, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store: store, cron: cron.New(), logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Scheduler) Start() error {
	// Critical alerts: every 30 min
	if _, err := s.cron.AddFunc("*/30 * * * *", s.checkCriticalAlerts); err != nil {
		return fmt.Errorf("add critical alerts job: %w", err)
	}
	// Review reminders: 3x/day (9h, 13h, 19h UTC)
	if _, err := s.cron.AddFunc("0 9,13,19 * * *", s.sendReviewReminders); err != nil {
		return fmt.Errorf("add review reminders job: %w", err)
	}
	// Daily motivation: once at 8h UTC
	if _, err := s.cron.AddFunc("0 8 * * *", s.sendDailyMotivation); err != nil {
		return fmt.Errorf("add daily motivation job: %w", err)
	}
	// End-of-day recap: once at 21h UTC
	if _, err := s.cron.AddFunc("0 21 * * *", s.sendDailyRecap); err != nil {
		return fmt.Errorf("add daily recap job: %w", err)
	}
	// Cleanup expired auth codes and refresh tokens: hourly
	if _, err := s.cron.AddFunc("0 * * * *", s.cleanupExpiredData); err != nil {
		return fmt.Errorf("add cleanup job: %w", err)
	}

	s.cron.Start()
	s.logger.Info("scheduler started", "jobs", "critical(30m), reviews(9/13/19h), motivation(8h), recap(21h), cleanup(1h)")
	return nil
}

func (s *Scheduler) Stop() { s.cron.Stop() }

// ─── Critical Alerts (every 30min) ──────────────────────────────────────────

func (s *Scheduler) checkCriticalAlerts() {
	learners, err := s.store.GetActiveLearners()
	if err != nil {
		s.logger.Error("scheduler: get learners", "err", err)
		return
	}

	for _, learner := range learners {
		if learner.WebhookURL == "" {
			continue
		}
		avail, _ := s.store.GetAvailability(learner.ID)
		if avail != nil && avail.DoNotDisturb {
			continue
		}

		states, err := s.store.GetConceptStatesByLearner(learner.ID)
		if err != nil {
			continue
		}
		interactions, _ := s.store.GetRecentInteractionsByLearner(learner.ID, 20)
		alerts := ComputeAlerts(states, interactions, time.Time{})

		for _, alert := range alerts {
			if alert.Urgency != "critical" {
				continue
			}
			// Dedup: don't send same alert type twice in one day
			sent, _ := s.store.WasAlertSentToday(learner.ID, string(alert.Type))
			if sent {
				continue
			}

			msg := formatCriticalAlert(alert)
			if err := s.sendDiscordEmbed(learner.WebhookURL, msg); err != nil {
				s.logger.Error("scheduler: critical webhook", "err", err, "learner", learner.ID)
				continue
			}
			s.store.CreateScheduledAlert(learner.ID, string(alert.Type), alert.Concept, time.Now())
			s.logger.Info("scheduler: critical alert sent", "learner", learner.ID, "type", alert.Type)
		}

		// Metacognitive alerts
		affects, _ := s.store.GetRecentAffectStates(learner.ID, 5)
		calibBias, _ := s.store.GetCalibrationBias(learner.ID, 20)

		var autonomyScores []float64
		for _, a := range affects {
			autonomyScores = append(autonomyScores, a.AutonomyScore)
		}

		metaAlerts := ComputeMetacognitiveAlerts(autonomyScores, calibBias, affects, interactions)
		for _, alert := range metaAlerts {
			sent, _ := s.store.WasAlertSentToday(learner.ID, string(alert.Type))
			if sent {
				continue
			}
			msg := formatMetacognitiveAlert(alert)
			if err := s.sendDiscordEmbed(learner.WebhookURL, msg); err != nil {
				s.logger.Error("scheduler: metacognitive webhook", "err", err, "learner", learner.ID)
				continue
			}
			s.store.CreateScheduledAlert(learner.ID, string(alert.Type), alert.Concept, time.Now())
			s.logger.Info("scheduler: metacognitive alert sent", "learner", learner.ID, "type", alert.Type)
		}
	}
}

// ─── Review Reminders (3x/day) ──────────────────────────────────────────────

func (s *Scheduler) sendReviewReminders() {
	learners, err := s.store.GetActiveLearners()
	if err != nil {
		return
	}

	for _, learner := range learners {
		if learner.WebhookURL == "" {
			continue
		}
		avail, _ := s.store.GetAvailability(learner.ID)
		if avail != nil && avail.DoNotDisturb {
			continue
		}

		// Don't remind if already sent a review reminder today
		sent, _ := s.store.WasAlertSentToday(learner.ID, "REVIEW_REMINDER")
		if sent {
			continue
		}

		// Check for concepts due
		due, _ := s.store.GetConceptsDueForReview(learner.ID)
		if len(due) == 0 {
			continue
		}

		// Check if already studied today
		todayCount, _ := s.store.GetTodayInteractionCount(learner.ID)
		if todayCount > 0 {
			continue // already active, no need to nag
		}

		// Check inactivity duration
		hoursSinceActive := time.Since(learner.LastActive).Hours()
		if hoursSinceActive < 4 {
			continue // was active recently
		}

		msg := formatReviewReminder(due, hoursSinceActive)
		if err := s.sendDiscordEmbed(learner.WebhookURL, msg); err != nil {
			s.logger.Error("scheduler: review webhook", "err", err)
			continue
		}
		s.store.CreateScheduledAlert(learner.ID, "REVIEW_REMINDER", strings.Join(due, ","), time.Now())
		s.logger.Info("scheduler: review reminder sent", "learner", learner.ID, "due", len(due))
	}
}

// ─── Daily Motivation (8h) ──────────────────────────────────────────────────
//
// The scheduler no longer composes motivational text. Claude authors messages
// during sessions via queue_webhook_message, the scheduler dispatches from the
// queue. If the queue is empty for a learner, we fall back to a sober one-liner
// (no KPIs, no analytics bulletin tone).

func (s *Scheduler) sendDailyMotivation() {
	s.dispatchQueued("daily_motivation", "DAILY_MOTIVATION", fallbackDailyMotivation)
}

// ─── Daily Recap (21h) ─────────────────────────────────────────────────────

func (s *Scheduler) sendDailyRecap() {
	s.dispatchQueued("daily_recap", "DAILY_RECAP", fallbackDailyRecap)
}

// dispatchQueued pulls the best pending webhook message for each active learner
// (kind + ±30min scheduling window) and posts it. Falls back to a sober template
// when the queue is empty.
func (s *Scheduler) dispatchQueued(kind, alertTag string, fallback func(*models.Learner) discordPayload) {
	learners, err := s.store.GetActiveLearners()
	if err != nil {
		s.logger.Error("scheduler: dispatch get learners", "err", err, "kind", kind)
		return
	}
	now := time.Now().UTC()

	for _, learner := range learners {
		if learner.WebhookURL == "" {
			continue
		}
		avail, _ := s.store.GetAvailability(learner.ID)
		if avail != nil && avail.DoNotDisturb {
			continue
		}

		// Dedup at the alert layer — don't fire twice in one day.
		sent, _ := s.store.WasAlertSentToday(learner.ID, alertTag)
		if sent {
			continue
		}

		item, err := s.store.DequeueNextPending(learner.ID, kind, now, 30*time.Minute)
		if err != nil {
			s.logger.Error("scheduler: dequeue", "err", err, "learner", learner.ID, "kind", kind)
			continue
		}

		var payload discordPayload
		var source string
		if item != nil && item.Content != "" {
			payload = discordPayload{Embeds: []discordEmbed{{
				Title:       queueKindTitle(kind),
				Description: item.Content,
				Color:       queueKindColor(kind),
			}}}
			source = "queue"
		} else if fallback != nil {
			payload = fallback(learner)
			source = "fallback"
		} else {
			continue
		}

		if err := s.sendDiscordEmbed(learner.WebhookURL, payload); err != nil {
			s.logger.Error("scheduler: dispatch webhook", "err", err, "learner", learner.ID, "kind", kind)
			if item != nil {
				_ = s.store.MarkWebhookFailed(item.ID)
			}
			continue
		}
		if item != nil {
			_ = s.store.MarkWebhookSent(item.ID, now)
		}
		s.store.CreateScheduledAlert(learner.ID, alertTag, "", time.Now())
		s.logger.Info("scheduler: dispatched", "learner", learner.ID, "kind", kind, "source", source)
	}
}

// queueKindTitle returns the embed title for a given webhook kind.
func queueKindTitle(kind string) string {
	switch kind {
	case "daily_motivation":
		return "☀️ Bonjour"
	case "daily_recap":
		return "🌙 Ce soir"
	case "reactivation":
		return "👋 Reprends quand tu veux"
	case "reminder":
		return "📚 Note"
	}
	return "✉️ Message"
}

func queueKindColor(kind string) int {
	switch kind {
	case "daily_motivation":
		return 0x5865F2
	case "daily_recap":
		return 0x57F287
	case "reactivation":
		return 0xFEE75C
	}
	return 0x99AAB5
}

// ─── Sober fallback templates (no KPI, no analytics) ─────────────────────────

func fallbackDailyMotivation(_ *models.Learner) discordPayload {
	return discordPayload{Embeds: []discordEmbed{{
		Title:       "☀️ Bonjour",
		Description: "Meme 5 minutes aujourd'hui, ca tient la trajectoire. Reviens quand tu veux.",
		Color:       0x5865F2,
	}}}
}

func fallbackDailyRecap(_ *models.Learner) discordPayload {
	return discordPayload{Embeds: []discordEmbed{{
		Title:       "🌙 Ce soir",
		Description: "Si tu passes, on continue. Sinon, a demain.",
		Color:       0x57F287,
	}}}
}

// ─── Cleanup (hourly) ─────────────────────────────────────────────────────

func (s *Scheduler) cleanupExpiredData() {
	codes, err := s.store.CleanupExpiredCodes()
	if err != nil {
		s.logger.Error("scheduler: cleanup codes", "err", err)
	} else if codes > 0 {
		s.logger.Info("scheduler: cleaned expired codes", "count", codes)
	}

	tokens, err := s.store.CleanupExpiredRefreshTokens()
	if err != nil {
		s.logger.Error("scheduler: cleanup tokens", "err", err)
	} else if tokens > 0 {
		s.logger.Info("scheduler: cleaned expired refresh tokens", "count", tokens)
	}

	expired, err := s.store.ExpirePastWebhookMessages(time.Now().UTC())
	if err != nil {
		s.logger.Error("scheduler: expire webhook queue", "err", err)
	} else if expired > 0 {
		s.logger.Info("scheduler: expired webhook messages", "count", expired)
	}
}

// ─── Message Formatting ─────────────────────────────────────────────────────

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

func formatMetacognitiveAlert(alert models.Alert) discordPayload {
	title := ""
	color := 0xFFA500 // orange

	switch alert.Type {
	case models.AlertDependencyIncreasing:
		title = "📉 Autonomie en baisse"
	case models.AlertCalibrationDiverging:
		title = "🎯 Calibration divergente"
	case models.AlertAffectNegative:
		title = "😔 Sessions difficiles"
	case models.AlertTransferBlocked:
		title = fmt.Sprintf("🔒 Transfert bloque: %s", alert.Concept)
	default:
		title = string(alert.Type)
	}

	return discordPayload{
		Embeds: []discordEmbed{{
			Title:       title,
			Description: alert.RecommendedAction,
			Color:       color,
		}},
	}
}

func formatCriticalAlert(alert models.Alert) discordPayload {
	return discordPayload{
		Embeds: []discordEmbed{{
			Title:       fmt.Sprintf("🚨 %s en danger", alert.Concept),
			Description: fmt.Sprintf("Retention a **%.0f%%** — %s\n\nOuvre Claude pour reviser maintenant.", alert.Retention*100, alert.RecommendedAction),
			Color:       0xFF0000, // red
		}},
	}
}

func formatReviewReminder(due []string, hoursSinceActive float64) discordPayload {
	conceptList := due
	if len(conceptList) > 3 {
		conceptList = conceptList[:3]
	}
	desc := fmt.Sprintf("**%d concept(s)** a reviser :", len(due))
	for _, c := range conceptList {
		desc += fmt.Sprintf("\n→ %s", c)
	}
	if len(due) > 3 {
		desc += fmt.Sprintf("\n... et %d autres", len(due)-3)
	}
	desc += "\n\n10 minutes suffisent pour garder ta progression."

	return discordPayload{
		Embeds: []discordEmbed{{
			Title:       "📚 Revision disponible",
			Description: desc,
			Color:       0xFFA500, // orange
		}},
	}
}

// Note: formatDailyMotivation / formatDailyRecap / formatInactivityNudge have been
// removed. Daily motivation and recap are now authored by Claude via queue_webhook_message
// and dispatched by dispatchQueued (with sober Go fallbacks when the queue is empty).

// ─── Discord Webhook ─────────────────────────────────────────────────────────

func (s *Scheduler) sendDiscordEmbed(url string, payload discordPayload) error {
	body, _ := json.Marshal(payload)
	return s.doWithRetry(url, body)
}

// sendWebhook sends a plain text message (kept for backwards compatibility).
func (s *Scheduler) sendWebhook(url, message string) error {
	body, _ := json.Marshal(map[string]string{"content": message})
	return s.doWithRetry(url, body)
}

// doWithRetry posts body to url with exponential backoff.
// 4 attempts: immediate, +1s, +5s, +25s.
// Stops on 4xx (except 429). Respects Discord Retry-After header on 429.
func (s *Scheduler) doWithRetry(url string, body []byte) error {
	delays := []time.Duration{0, 1 * time.Second, 5 * time.Second, 25 * time.Second}
	var lastErr error
	for attempt, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			s.logger.Warn("webhook network error", "attempt", attempt+1, "err", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode < 400 {
			return nil
		}
		lastErr = fmt.Errorf("webhook returned %d", resp.StatusCode)
		// 429: respect Retry-After
		if resp.StatusCode == 429 {
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil && secs > 0 && secs <= 60 {
					s.logger.Warn("webhook rate limited, waiting", "retry_after", secs)
					time.Sleep(time.Duration(secs) * time.Second)
				}
			}
			continue
		}
		// 4xx (not 429): client error, don't retry
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
		s.logger.Warn("webhook retry", "attempt", attempt+1, "status", resp.StatusCode)
	}
	return lastErr
}
