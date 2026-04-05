package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"learning-runtime/algorithms"
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

func (s *Scheduler) sendDailyMotivation() {
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

		streak, _ := s.store.GetDailyStreak(learner.ID)
		states, _ := s.store.GetConceptStatesByLearner(learner.ID)
		due, _ := s.store.GetConceptsDueForReview(learner.ID)

		// Count mastered concepts
		mastered := 0
		total := len(states)
		for _, cs := range states {
			if cs.PMastery >= algorithms.BKTMasteryThreshold {
				mastered++
			}
		}

		// Compute day number
		dayNumber := int(math.Floor(time.Since(learner.CreatedAt).Hours()/24)) + 1

		msg := formatDailyMotivation(dayNumber, streak, mastered, total, len(due), learner.Objective)
		if err := s.sendDiscordEmbed(learner.WebhookURL, msg); err != nil {
			s.logger.Error("scheduler: motivation webhook", "err", err)
			continue
		}
		s.store.CreateScheduledAlert(learner.ID, "DAILY_MOTIVATION", "", time.Now())
		s.logger.Info("scheduler: motivation sent", "learner", learner.ID, "streak", streak)
	}
}

// ─── Daily Recap (21h) ─────────────────────────────────────────────────────

func (s *Scheduler) sendDailyRecap() {
	learners, err := s.store.GetActiveLearners()
	if err != nil {
		return
	}

	for _, learner := range learners {
		if learner.WebhookURL == "" {
			continue
		}

		todayCount, _ := s.store.GetTodayInteractionCount(learner.ID)
		if todayCount == 0 {
			// No activity today — send a gentle nudge instead of recap
			hoursSinceActive := time.Since(learner.LastActive).Hours()
			if hoursSinceActive > 24 {
				msg := formatInactivityNudge(hoursSinceActive)
				s.sendDiscordEmbed(learner.WebhookURL, msg)
				s.store.CreateScheduledAlert(learner.ID, "INACTIVITY_NUDGE", "", time.Now())
			}
			continue
		}

		successRate, total, _ := s.store.GetTodaySuccessRate(learner.ID)
		streak, _ := s.store.GetDailyStreak(learner.ID)

		// Find concepts worked on today
		states, _ := s.store.GetConceptStatesByLearner(learner.ID)
		mastered := 0
		for _, cs := range states {
			if cs.PMastery >= algorithms.BKTMasteryThreshold {
				mastered++
			}
		}

		msg := formatDailyRecap(total, successRate, streak, mastered, len(states))
		if err := s.sendDiscordEmbed(learner.WebhookURL, msg); err != nil {
			s.logger.Error("scheduler: recap webhook", "err", err)
			continue
		}
		s.store.CreateScheduledAlert(learner.ID, "DAILY_RECAP", "", time.Now())
		s.logger.Info("scheduler: recap sent", "learner", learner.ID, "exercises", total)
	}
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

func formatDailyMotivation(dayNumber, streak, mastered, total, dueCount int, objective string) discordPayload {
	var lines []string

	lines = append(lines, fmt.Sprintf("**Jour %d** de ton apprentissage", dayNumber))

	if streak > 1 {
		lines = append(lines, fmt.Sprintf("🔥 **%d jours consecutifs** — continue !", streak))
	} else if streak == 1 {
		lines = append(lines, "Hier c'etait bien. Enchaine aujourd'hui.")
	} else {
		lines = append(lines, "Nouvelle journee, nouveau depart. Une seule session suffit.")
	}

	if total > 0 {
		pct := float64(mastered) / float64(total) * 100
		lines = append(lines, fmt.Sprintf("📊 Progression : **%d/%d** concepts maitrises (%.0f%%)", mastered, total, pct))
	}

	if dueCount > 0 {
		lines = append(lines, fmt.Sprintf("📋 %d concept(s) a reviser aujourd'hui", dueCount))
	} else {
		lines = append(lines, "✅ Rien a reviser — explore un nouveau concept")
	}

	if objective != "" {
		lines = append(lines, fmt.Sprintf("\n🎯 *%s*", objective))
	}

	return discordPayload{
		Embeds: []discordEmbed{{
			Title:       "☀️ Bonne session",
			Description: strings.Join(lines, "\n"),
			Color:       0x5865F2, // discord blurple
		}},
	}
}

func formatDailyRecap(exerciseCount int, successRate float64, streak, mastered, total int) discordPayload {
	var lines []string

	lines = append(lines, fmt.Sprintf("**%d exercices** aujourd'hui", exerciseCount))

	// Success rate with emoji
	rateEmoji := "🟡"
	if successRate >= 0.8 {
		rateEmoji = "🟢"
	} else if successRate < 0.5 {
		rateEmoji = "🔴"
	}
	lines = append(lines, fmt.Sprintf("%s Taux de reussite : **%.0f%%**", rateEmoji, successRate*100))

	if streak > 1 {
		lines = append(lines, fmt.Sprintf("🔥 Streak : **%d jours**", streak))
	}

	if total > 0 {
		pct := float64(mastered) / float64(total) * 100
		lines = append(lines, fmt.Sprintf("📊 **%d/%d** concepts maitrises (%.0f%%)", mastered, total, pct))
	}

	// Encouragement based on performance
	if successRate >= 0.8 && exerciseCount >= 5 {
		lines = append(lines, "\n💪 Excellente session. Demain on pousse plus loin.")
	} else if successRate >= 0.6 {
		lines = append(lines, "\n👍 Bonne session. La regularite fait la difference.")
	} else {
		lines = append(lines, "\n🧠 Session difficile — c'est normal. Reviens demain, le cerveau consolide pendant la nuit.")
	}

	return discordPayload{
		Embeds: []discordEmbed{{
			Title:       "📊 Recap du jour",
			Description: strings.Join(lines, "\n"),
			Color:       0x57F287, // green
		}},
	}
}

func formatInactivityNudge(hoursSinceActive float64) discordPayload {
	days := int(hoursSinceActive / 24)
	var desc string
	switch {
	case days <= 1:
		desc = "Tu n'as pas pratique aujourd'hui. Meme 5 minutes comptent."
	case days <= 3:
		desc = fmt.Sprintf("Ca fait **%d jours** sans session. Ta retention baisse — une revision rapide suffit pour inverser la courbe.", days)
	default:
		desc = fmt.Sprintf("**%d jours** d'absence. Pas de jugement — reviens quand tu veux. Une seule session relance tout.", days)
	}

	return discordPayload{
		Embeds: []discordEmbed{{
			Title:       "👋 On ne t'oublie pas",
			Description: desc,
			Color:       0xFEE75C, // yellow
		}},
	}
}

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
