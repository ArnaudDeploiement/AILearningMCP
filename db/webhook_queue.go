package db

import (
	"database/sql"
	"fmt"
	"time"

	"learning-runtime/models"
)

// EnqueueWebhookMessage persists a scheduled, LLM-authored webhook nudge.
// Returns the inserted row ID.
func (s *Store) EnqueueWebhookMessage(learnerID, kind, content string, scheduledFor, expiresAt time.Time, priority int) (int64, error) {
	if kind == "" {
		return 0, fmt.Errorf("kind is required")
	}
	if content == "" {
		return 0, fmt.Errorf("content is required")
	}
	if scheduledFor.IsZero() {
		return 0, fmt.Errorf("scheduled_for is required")
	}
	var expires any
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC()
	}
	result, err := s.db.Exec(
		`INSERT INTO webhook_message_queue (learner_id, kind, scheduled_for, expires_at, content, priority, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
		learnerID, kind, scheduledFor.UTC(), expires, content, priority, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue webhook message: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get webhook queue id: %w", err)
	}
	return id, nil
}

// DequeueNextPending returns the highest-priority pending message for a learner/kind
// whose scheduled_for is within [now-window, now+window] and not expired.
// Returns (nil, nil) if nothing is pending.
func (s *Store) DequeueNextPending(learnerID, kind string, now time.Time, window time.Duration) (*models.WebhookQueueItem, error) {
	lower := now.Add(-window).UTC()
	upper := now.Add(window).UTC()
	row := s.db.QueryRow(
		`SELECT id, learner_id, kind, scheduled_for, expires_at, content, priority, status, created_at, sent_at
		 FROM webhook_message_queue
		 WHERE learner_id = ? AND kind = ? AND status = 'pending'
		   AND scheduled_for BETWEEN ? AND ?
		   AND (expires_at IS NULL OR expires_at > ?)
		 ORDER BY priority DESC, scheduled_for ASC
		 LIMIT 1`,
		learnerID, kind, lower, upper, now.UTC(),
	)
	item, err := scanWebhookQueueItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue webhook message: %w", err)
	}
	return item, nil
}

// MarkWebhookSent marks the given queue item as sent at `now`.
func (s *Store) MarkWebhookSent(id int64, now time.Time) error {
	_, err := s.db.Exec(
		`UPDATE webhook_message_queue SET status = 'sent', sent_at = ? WHERE id = ?`,
		now.UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("mark webhook sent: %w", err)
	}
	return nil
}

// MarkWebhookFailed marks the given queue item as failed (will not be retried).
func (s *Store) MarkWebhookFailed(id int64) error {
	_, err := s.db.Exec(
		`UPDATE webhook_message_queue SET status = 'failed' WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark webhook failed: %w", err)
	}
	return nil
}

// ExpirePastWebhookMessages marks any pending message whose expires_at is in the past as 'expired'.
// Returns the number of rows updated.
func (s *Store) ExpirePastWebhookMessages(now time.Time) (int64, error) {
	result, err := s.db.Exec(
		`UPDATE webhook_message_queue SET status = 'expired'
		 WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at < ?`,
		now.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("expire past webhook messages: %w", err)
	}
	return result.RowsAffected()
}

// GetPendingWebhookMessages returns all pending messages (for monitoring / debugging).
func (s *Store) GetPendingWebhookMessages(learnerID string) ([]*models.WebhookQueueItem, error) {
	rows, err := s.db.Query(
		`SELECT id, learner_id, kind, scheduled_for, expires_at, content, priority, status, created_at, sent_at
		 FROM webhook_message_queue
		 WHERE learner_id = ? AND status = 'pending'
		 ORDER BY scheduled_for ASC`,
		learnerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending webhook messages: %w", err)
	}
	defer rows.Close()

	var out []*models.WebhookQueueItem
	for rows.Next() {
		item, err := scanWebhookQueueItemRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// scanWebhookQueueItem is used for QueryRow results.
func scanWebhookQueueItem(row *sql.Row) (*models.WebhookQueueItem, error) {
	item := &models.WebhookQueueItem{}
	var expiresAt, sentAt sql.NullTime
	err := row.Scan(
		&item.ID, &item.LearnerID, &item.Kind, &item.ScheduledFor,
		&expiresAt, &item.Content, &item.Priority, &item.Status,
		&item.CreatedAt, &sentAt,
	)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		item.ExpiresAt = &t
	}
	if sentAt.Valid {
		t := sentAt.Time
		item.SentAt = &t
	}
	return item, nil
}

// scanWebhookQueueItemRows is used for Query results (iteration).
func scanWebhookQueueItemRows(rows *sql.Rows) (*models.WebhookQueueItem, error) {
	item := &models.WebhookQueueItem{}
	var expiresAt, sentAt sql.NullTime
	err := rows.Scan(
		&item.ID, &item.LearnerID, &item.Kind, &item.ScheduledFor,
		&expiresAt, &item.Content, &item.Priority, &item.Status,
		&item.CreatedAt, &sentAt,
	)
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		item.ExpiresAt = &t
	}
	if sentAt.Valid {
		t := sentAt.Time
		item.SentAt = &t
	}
	return item, nil
}
