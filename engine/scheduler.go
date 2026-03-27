package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"learning-runtime/db"

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
	_, err := s.cron.AddFunc("0 * * * *", s.checkAlerts)
	if err != nil {
		return fmt.Errorf("add cron job: %w", err)
	}
	s.cron.Start()
	s.logger.Info("scheduler started", "interval", "hourly")
	return nil
}

func (s *Scheduler) Stop() { s.cron.Stop() }

func (s *Scheduler) checkAlerts() {
	learners, err := s.store.GetActiveLearners()
	if err != nil {
		s.logger.Error("scheduler: get learners", "err", err)
		return
	}

	for _, learner := range learners {
		if learner.WebhookURL == "" {
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
			avail, err := s.store.GetAvailability(learner.ID)
			if err != nil {
				continue
			}
			if avail.DoNotDisturb {
				continue
			}

			msg := fmt.Sprintf("%s — retention a %.0f%%. %s. Ouvre Claude pour reviser.",
				alert.Concept, alert.Retention*100, alert.RecommendedAction)
			if err := s.sendWebhook(learner.WebhookURL, msg); err != nil {
				s.logger.Error("scheduler: webhook", "err", err)
				continue
			}
			s.store.CreateScheduledAlert(learner.ID, string(alert.Type), alert.Concept, time.Now())
			s.logger.Info("scheduler: webhook sent", "learner", learner.ID, "concept", alert.Concept)
		}
	}
}

func (s *Scheduler) sendWebhook(url, message string) error {
	payload := map[string]string{"content": message}
	body, _ := json.Marshal(payload)
	resp, err := s.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
