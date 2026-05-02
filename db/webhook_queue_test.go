// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package db

import (
	"testing"
	"time"
)

func TestEnqueueWebhookMessage_ValidationErrors(t *testing.T) {
	store := setupTestDB(t)
	now := time.Now().UTC()
	cases := []struct {
		name         string
		kind         string
		content      string
		scheduledFor time.Time
	}{
		{"empty kind", "", "body", now},
		{"empty content", "daily_motivation", "", now},
		{"zero scheduled_for", "daily_motivation", "body", time.Time{}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			id, err := store.EnqueueWebhookMessage("L1", tc.kind, tc.content, tc.scheduledFor, time.Time{}, 0)
			if err == nil {
				t.Fatalf("expected validation error, got id=%d", id)
			}
		})
	}
}

func TestEnqueueWebhookMessage_PersistsRow(t *testing.T) {
	store := setupTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(2 * time.Hour)

	id, err := store.EnqueueWebhookMessage("L1", "daily_motivation", "hello", now, expires, 5)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if id <= 0 {
		t.Fatal("expected positive id")
	}

	// Direct query: row should exist with status='pending', priority=5, content='hello'.
	var (
		kind, content, status string
		priority              int
	)
	if err := store.db.QueryRow(
		`SELECT kind, content, priority, status FROM webhook_message_queue WHERE id = ?`, id,
	).Scan(&kind, &content, &priority, &status); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	if kind != "daily_motivation" || content != "hello" || priority != 5 || status != "pending" {
		t.Errorf("row mismatch: kind=%q content=%q priority=%d status=%q",
			kind, content, priority, status)
	}

	// Enqueue without expires_at - expires column should be NULL.
	id2, err := store.EnqueueWebhookMessage("L1", "reminder", "remind", now, time.Time{}, 0)
	if err != nil {
		t.Fatalf("enqueue no expiry: %v", err)
	}
	var expiresAtNullable any
	if err := store.db.QueryRow(
		`SELECT expires_at FROM webhook_message_queue WHERE id = ?`, id2,
	).Scan(&expiresAtNullable); err != nil {
		t.Fatalf("scan expires_at: %v", err)
	}
	if expiresAtNullable != nil {
		t.Errorf("expected expires_at NULL, got %v", expiresAtNullable)
	}
}

func TestDequeueNextPending(t *testing.T) {
	store := setupTestDB(t)
	now := time.Now().UTC()

	// Out-of-window (before lower bound).
	if _, err := store.EnqueueWebhookMessage(
		"L1", "daily_motivation", "old", now.Add(-2*time.Hour), time.Time{}, 0,
	); err != nil {
		t.Fatalf("enqueue old: %v", err)
	}
	// In-window, low priority.
	idLow, err := store.EnqueueWebhookMessage(
		"L1", "daily_motivation", "low", now, time.Time{}, 1,
	)
	if err != nil {
		t.Fatalf("enqueue low: %v", err)
	}
	// In-window, high priority -> should win.
	idHigh, err := store.EnqueueWebhookMessage(
		"L1", "daily_motivation", "high", now.Add(5*time.Minute), time.Time{}, 9,
	)
	if err != nil {
		t.Fatalf("enqueue high: %v", err)
	}
	// Different kind, should not match.
	if _, err := store.EnqueueWebhookMessage(
		"L1", "reminder", "skip", now, time.Time{}, 99,
	); err != nil {
		t.Fatalf("enqueue other kind: %v", err)
	}
	// Already-expired pending row in window: should NOT dequeue.
	if _, err := store.EnqueueWebhookMessage(
		"L1", "daily_motivation", "stale", now, now.Add(-1*time.Minute), 99,
	); err != nil {
		t.Fatalf("enqueue stale: %v", err)
	}

	got, err := store.DequeueNextPending("L1", "daily_motivation", now, 30*time.Minute)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if got == nil {
		t.Fatal("expected a pending item, got nil")
	}
	if got.ID != idHigh {
		t.Errorf("expected id=%d (high priority), got id=%d", idHigh, got.ID)
	}
	if got.Content != "high" {
		t.Errorf("content = %q want 'high'", got.Content)
	}
	if got.Priority != 9 {
		t.Errorf("priority = %d want 9", got.Priority)
	}
	if got.Status != "pending" {
		t.Errorf("status = %q want 'pending'", got.Status)
	}

	// Mark high-priority as sent; next dequeue should pick the low-priority one.
	sentAt := time.Now().UTC()
	if err := store.MarkWebhookSent(idHigh, sentAt); err != nil {
		t.Fatalf("mark sent: %v", err)
	}
	var status string
	var sentAtScan time.Time
	if err := store.db.QueryRow(
		`SELECT status, sent_at FROM webhook_message_queue WHERE id = ?`, idHigh,
	).Scan(&status, &sentAtScan); err != nil {
		t.Fatalf("scan after send: %v", err)
	}
	if status != "sent" {
		t.Errorf("status after MarkWebhookSent = %q want 'sent'", status)
	}

	got, err = store.DequeueNextPending("L1", "daily_motivation", now, 30*time.Minute)
	if err != nil {
		t.Fatalf("dequeue after send: %v", err)
	}
	if got == nil || got.ID != idLow {
		t.Fatalf("expected idLow=%d, got %+v", idLow, got)
	}

	// Empty case for unknown learner returns (nil, nil).
	got, err = store.DequeueNextPending("L-missing", "daily_motivation", now, 30*time.Minute)
	if err != nil {
		t.Errorf("expected nil err on no rows, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil item, got %+v", got)
	}
}

func TestMarkWebhookFailed(t *testing.T) {
	store := setupTestDB(t)
	now := time.Now().UTC()
	id, err := store.EnqueueWebhookMessage("L1", "reminder", "retry", now, time.Time{}, 0)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := store.MarkWebhookFailed(id); err != nil {
		t.Fatalf("MarkWebhookFailed: %v", err)
	}
	var status string
	if err := store.db.QueryRow(
		`SELECT status FROM webhook_message_queue WHERE id = ?`, id,
	).Scan(&status); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if status != "failed" {
		t.Errorf("status = %q want 'failed'", status)
	}
}

func TestExpirePastWebhookMessages(t *testing.T) {
	store := setupTestDB(t)
	now := time.Now().UTC()

	// One pending row with expires_at in the past.
	idStale, err := store.EnqueueWebhookMessage(
		"L1", "reminder", "stale", now, now.Add(-5*time.Minute), 0,
	)
	if err != nil {
		t.Fatalf("enqueue stale: %v", err)
	}
	// One pending row with expires_at in the future.
	idFresh, err := store.EnqueueWebhookMessage(
		"L1", "reminder", "fresh", now, now.Add(1*time.Hour), 0,
	)
	if err != nil {
		t.Fatalf("enqueue fresh: %v", err)
	}
	// One pending row with no expires_at (NULL) — should not be expired.
	idNoExp, err := store.EnqueueWebhookMessage(
		"L1", "reminder", "noexp", now, time.Time{}, 0,
	)
	if err != nil {
		t.Fatalf("enqueue noexp: %v", err)
	}

	n, err := store.ExpirePastWebhookMessages(now)
	if err != nil {
		t.Fatalf("ExpirePastWebhookMessages: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row affected, got %d", n)
	}
	var statusStale, statusFresh, statusNoExp string
	if err := store.db.QueryRow(
		`SELECT status FROM webhook_message_queue WHERE id = ?`, idStale,
	).Scan(&statusStale); err != nil {
		t.Fatalf("scan stale: %v", err)
	}
	if err := store.db.QueryRow(
		`SELECT status FROM webhook_message_queue WHERE id = ?`, idFresh,
	).Scan(&statusFresh); err != nil {
		t.Fatalf("scan fresh: %v", err)
	}
	if err := store.db.QueryRow(
		`SELECT status FROM webhook_message_queue WHERE id = ?`, idNoExp,
	).Scan(&statusNoExp); err != nil {
		t.Fatalf("scan noexp: %v", err)
	}
	if statusStale != "expired" {
		t.Errorf("stale status = %q want 'expired'", statusStale)
	}
	if statusFresh != "pending" {
		t.Errorf("fresh status = %q want 'pending'", statusFresh)
	}
	if statusNoExp != "pending" {
		t.Errorf("noexp status = %q want 'pending'", statusNoExp)
	}
}

func TestGetPendingWebhookMessages(t *testing.T) {
	store := setupTestDB(t)
	now := time.Now().UTC()

	// Insert three pending rows for L1.
	if _, err := store.EnqueueWebhookMessage("L1", "k", "a", now.Add(1*time.Hour), time.Time{}, 0); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := store.EnqueueWebhookMessage("L1", "k", "b", now.Add(2*time.Hour), time.Time{}, 0); err != nil {
		t.Fatalf("b: %v", err)
	}
	idC, err := store.EnqueueWebhookMessage("L1", "k", "c", now.Add(3*time.Hour), time.Time{}, 0)
	if err != nil {
		t.Fatalf("c: %v", err)
	}
	// Mark one as sent: should not appear in pending list.
	if err := store.MarkWebhookSent(idC, now); err != nil {
		t.Fatalf("mark sent: %v", err)
	}

	got, err := store.GetPendingWebhookMessages("L1")
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(got))
	}
	// Ordering is ASC by scheduled_for.
	if got[0].Content != "a" || got[1].Content != "b" {
		t.Errorf("expected order [a,b], got [%s,%s]", got[0].Content, got[1].Content)
	}

	// Other learner: empty list.
	got, _ = store.GetPendingWebhookMessages("L-other")
	if len(got) != 0 {
		t.Errorf("expected 0 for other learner, got %d", len(got))
	}
}
