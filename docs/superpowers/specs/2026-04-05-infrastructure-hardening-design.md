# Infrastructure Hardening Design

**Date:** 2026-04-05
**Status:** Approved
**Scope:** Auth persistence, rate limiting, webhook retry, DB indexes, cleanup jobs

## Context

The learning runtime is an open-source MCP server. Infrastructure must work for any deployer without external dependencies (no Redis, no message queue). SQLite is the only persistence layer.

Current gaps:
- Auth codes stored in-memory (lost on restart)
- OAuth client registrations not persisted
- No rate limiting on any endpoint
- Webhook delivery is fire-and-forget (no retry)
- No DB indexes beyond primary keys
- No cleanup of expired tokens/codes

## 1. Auth Codes and Clients in DB

### New Tables

```sql
CREATE TABLE IF NOT EXISTS oauth_codes (
    code           TEXT PRIMARY KEY,
    learner_id     TEXT NOT NULL REFERENCES learners(id),
    code_challenge TEXT NOT NULL,
    expires_at     DATETIME NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id      TEXT PRIMARY KEY,
    client_name    TEXT DEFAULT '',
    redirect_uris  TEXT DEFAULT '[]',
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### New Store Methods

- `CreateAuthCode(code, learnerID, challenge string, expiresAt time.Time) error`
- `ConsumeAuthCode(code string) (*AuthCode, error)` — SELECT + DELETE in one transaction
- `CleanupExpiredCodes() error` — DELETE WHERE expires_at < now
- `CreateOAuthClient(clientID, name, redirectURIs string) error`

### Changes to auth/oauth.go

- Remove `codes map[string]*AuthCode` and `codesMu sync.Mutex` from OAuthServer struct
- `handleAuthorizePost`: replace map write with `store.CreateAuthCode()`
- `handleAuthorizationCodeGrant`: replace map read+delete with `store.ConsumeAuthCode()`
- `handleDynamicClientRegistration`: add `store.CreateOAuthClient()` call

### Cleanup Job

Add to scheduler: hourly job `cleanupExpiredData()` that runs:
- `store.CleanupExpiredCodes()` — delete expired auth codes
- `store.CleanupExpiredRefreshTokens()` — delete refresh tokens past expires_at

## 2. Rate Limiting

### New File: auth/ratelimit.go

In-process token bucket rate limiter per IP. No external dependencies.

```
type RateLimiter struct {
    mu       sync.Mutex
    buckets  map[string]*bucket
    rate     float64        // tokens per second
    burst    int            // max tokens
}

type bucket struct {
    tokens    float64
    lastTime  time.Time
}
```

- `NewRateLimiter(rate float64, burst int) *RateLimiter`
- `Allow(ip string) bool` — consumes a token, returns false if empty
- Background goroutine purges stale entries (no activity > 10 min) every minute

### Middleware: auth/ratelimit.go

```
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler
```

Returns 429 with `Retry-After` header when limit exceeded.

### Endpoint Limits

| Endpoint | Rate | Burst | Rationale |
|----------|------|-------|-----------|
| POST /token | 10/min | 10 | Anti brute-force |
| POST /authorize | 10/min | 10 | Anti credential stuffing |
| POST /register | 5/min | 5 | Anti client spam |
| /mcp | 60/min | 60 | API protection |

### Integration in main.go

Create separate RateLimiter instances per limit tier. Wrap handlers:

```go
authLimiter := auth.NewRateLimiter(10.0/60, 10)
registerLimiter := auth.NewRateLimiter(5.0/60, 5)
mcpLimiter := auth.NewRateLimiter(1, 60)
```

## 3. Webhook Retry with Exponential Backoff

### Change in engine/scheduler.go

Replace `sendDiscordEmbed` with retry logic:

- 4 attempts: immediate, +1s, +5s, +25s
- Stop on 4xx (except 429) — client error, no point retrying
- On 429: read `Retry-After` header, wait accordingly
- Log each retry attempt at WARN level

No new tables. No persistent queue.

### Also update `sendWebhook` (plain text variant) with same retry logic

Extract retry into a shared helper:

```go
func (s *Scheduler) doWithRetry(url string, body []byte) error
```

Both `sendDiscordEmbed` and `sendWebhook` call this helper.

## 4. Database Indexes

Add to `db/schema.sql` and `db/migrations.go`:

```sql
CREATE INDEX IF NOT EXISTS idx_concept_states_learner
    ON concept_states(learner_id);

CREATE INDEX IF NOT EXISTS idx_concept_states_review
    ON concept_states(learner_id, next_review);

CREATE INDEX IF NOT EXISTS idx_interactions_learner_created
    ON interactions(learner_id, created_at);

CREATE INDEX IF NOT EXISTS idx_interactions_learner_concept
    ON interactions(learner_id, concept, created_at);

CREATE INDEX IF NOT EXISTS idx_scheduled_alerts_learner_type
    ON scheduled_alerts(learner_id, alert_type, created_at);

CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires
    ON oauth_codes(expires_at);
```

These cover:
- Scheduler queries (GetConceptsDueForReview, GetTodayInteractionCount)
- Session queries (GetSessionInteractions, GetRecentInteractions)
- Alert dedup (WasAlertSentToday)
- Cleanup jobs (expired codes)

## 5. Cleanup of Expired Refresh Tokens

### New Store Method

- `CleanupExpiredRefreshTokens() error` — DELETE WHERE expires_at < now

### Integration

Added to the same hourly cleanup job as expired auth codes (section 1).

## Files Changed

| File | Change |
|------|--------|
| `db/schema.sql` | Add oauth_codes, oauth_clients tables + indexes |
| `db/migrations.go` | Add new tables + indexes to ALTER migrations |
| `db/store.go` | Add CreateAuthCode, ConsumeAuthCode, CleanupExpiredCodes, CreateOAuthClient, CleanupExpiredRefreshTokens |
| `auth/oauth.go` | Remove in-memory maps, use Store methods |
| `auth/ratelimit.go` | New file: RateLimiter + middleware |
| `main.go` | Wire rate limiters to endpoints |
| `engine/scheduler.go` | Add retry logic to webhooks, add hourly cleanup job |

## Design Decisions

- **No Redis/external deps**: SQLite handles everything. Open-source friendly.
- **Rate limiting is in-process**: deployers needing distributed rate limiting use a reverse proxy.
- **Webhook retry is synchronous**: keeps code simple. Acceptable because cron jobs run in background goroutines and Discord webhooks are fast.
- **Profile-aware prompts rejected**: Claude already has conversation context + profile via get_learner_context(). Injecting into PromptForLLM is redundant.
