// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type QueueWebhookMessageParams struct {
	Kind         string `json:"kind" jsonschema:"Type de nudge : daily_motivation | daily_recap | reactivation | reminder"`
	ScheduledFor string `json:"scheduled_for" jsonschema:"ISO 8601 timestamp UTC de la fenêtre de tir (ex: 2026-04-13T08:00:00Z)"`
	ExpiresAt    string `json:"expires_at,omitempty" jsonschema:"ISO 8601 timestamp UTC après lequel le message ne doit plus être envoyé"`
	Content      string `json:"content" jsonschema:"Contenu Markdown prêt à poster sur le webhook Discord (max ~300 caractères recommandé)"`
	Priority     int    `json:"priority,omitempty" jsonschema:"Priorité (plus grand = plus prioritaire, défaut 0)"`
}

const maxWebhookContentLen = 1500

func registerQueueWebhookMessage(server *mcp.Server, deps *Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "queue_webhook_message",
		Description: "Met en queue un message de nudge que le scheduler postera sur le webhook Discord de l'apprenant à la fenêtre voulue. Le LLM compose le texte (chaleureux, sans KPI brut) ; le scheduler dispatche.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, params QueueWebhookMessageParams) (*mcp.CallToolResult, any, error) {
		learnerID, err := getLearnerID(ctx)
		if err != nil {
			deps.Logger.Error("queue_webhook_message: auth failed", "err", err)
			r, _ := errorResult(err.Error())
			return r, nil, nil
		}

		if !validWebhookKind(params.Kind) {
			r, _ := errorResult(fmt.Sprintf("invalid kind %q (expected daily_motivation | daily_recap | reactivation | reminder)", params.Kind))
			return r, nil, nil
		}
		if params.ScheduledFor == "" {
			r, _ := errorResult("scheduled_for is required")
			return r, nil, nil
		}
		if params.Content == "" {
			r, _ := errorResult("content is required")
			return r, nil, nil
		}
		if len(params.Content) > maxWebhookContentLen {
			r, _ := errorResult(fmt.Sprintf("content too long (%d chars, max %d)", len(params.Content), maxWebhookContentLen))
			return r, nil, nil
		}

		scheduledFor, err := time.Parse(time.RFC3339, params.ScheduledFor)
		if err != nil {
			r, _ := errorResult(fmt.Sprintf("scheduled_for must be RFC3339 (got %q)", params.ScheduledFor))
			return r, nil, nil
		}

		var expiresAt time.Time
		if params.ExpiresAt != "" {
			parsed, perr := time.Parse(time.RFC3339, params.ExpiresAt)
			if perr != nil {
				r, _ := errorResult(fmt.Sprintf("expires_at must be RFC3339 (got %q)", params.ExpiresAt))
				return r, nil, nil
			}
			expiresAt = parsed
		}

		id, err := deps.Store.EnqueueWebhookMessage(learnerID, params.Kind, params.Content, scheduledFor, expiresAt, params.Priority)
		if err != nil {
			deps.Logger.Error("queue_webhook_message: enqueue failed", "err", err, "learner", learnerID)
			r, _ := errorResult(fmt.Sprintf("failed to enqueue: %v", err))
			return r, nil, nil
		}

		r, _ := jsonResult(map[string]any{
			"queue_id":      id,
			"kind":          params.Kind,
			"scheduled_for": scheduledFor.UTC().Format(time.RFC3339),
		})
		return r, nil, nil
	})
}

func validWebhookKind(k string) bool {
	switch k {
	case models.WebhookKindDailyMotivation,
		models.WebhookKindDailyRecap,
		models.WebhookKindReactivation,
		models.WebhookKindReminder:
		return true
	}
	// olm:<domain_id> — used by the OLM dispatch to allow one queued
	// message per domain, since a learner can have multiple active domains
	// each with its own state snapshot.
	if strings.HasPrefix(k, "olm:") && len(k) > len("olm:") {
		return true
	}
	return false
}
