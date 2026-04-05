# Infrastructure Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden the learning runtime infrastructure — persist auth state in DB, add rate limiting, add webhook retry, add DB indexes, add cleanup jobs.

**Architecture:** All persistence uses SQLite via the existing `db.Store` pattern. Rate limiting is in-process (token bucket per IP). Webhook retry is synchronous with exponential backoff. No external dependencies added.

**Tech Stack:** Go 1.25, SQLite (modernc.org/sqlite), `database/sql`, `net/http`, `github.com/robfig/cron/v3`

**Spec:** `docs/superpowers/specs/2026-04-05-infrastructure-hardening-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `db/schema.sql` | Modify | Add oauth_codes, oauth_clients tables + indexes |
| `db/migrations.go` | Modify | Add new tables + indexes to idempotent migrations |
| `db/store.go` | Modify | Add 5 new methods: CreateAuthCode, ConsumeAuthCode, CleanupExpiredCodes, CreateOAuthClient, CleanupExpiredRefreshTokens |
| `auth/oauth.go` | Modify | Remove in-memory maps, use Store for auth codes and clients |
| `auth/ratelimit.go` | Create | Token bucket rate limiter + HTTP middleware |
| `main.go` | Modify | Wire rate limiters to endpoints |
| `engine/scheduler.go` | Modify | Add webhook retry helper, add hourly cleanup job |

---

### Task 1: Schema — Add oauth_codes, oauth_clients tables and indexes

**Files:**
- Modify: `db/schema.sql`
- Modify: `db/migrations.go`

- [ ] **Step 1: Add new tables to schema.sql**

Append before the closing of the file, after the `scheduled_alerts` table:

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

- [ ] **Step 2: Add indexes to schema.sql**

Append at the end of `schema.sql`:

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

- [ ] **Step 3: Add idempotent migrations for existing databases**

In `db/migrations.go`, add the new CREATE TABLE and CREATE INDEX statements to the `alterMigrations` slice. These use `IF NOT EXISTS` so they're safe to re-run:

```go
alterMigrations := []string{
    `ALTER TABLE learners ADD COLUMN profile_json TEXT DEFAULT '{}'`,
    `ALTER TABLE interactions ADD COLUMN error_type TEXT DEFAULT ''`,
}
for _, m := range alterMigrations {
    _, _ = db.Exec(m)
}

// Idempotent table + index creation for existing databases
idempotentMigrations := []string{
    `CREATE TABLE IF NOT EXISTS oauth_codes (
        code           TEXT PRIMARY KEY,
        learner_id     TEXT NOT NULL REFERENCES learners(id),
        code_challenge TEXT NOT NULL,
        expires_at     DATETIME NOT NULL,
        created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
    )`,
    `CREATE TABLE IF NOT EXISTS oauth_clients (
        client_id      TEXT PRIMARY KEY,
        client_name    TEXT DEFAULT '',
        redirect_uris  TEXT DEFAULT '[]',
        created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
    )`,
    `CREATE INDEX IF NOT EXISTS idx_concept_states_learner ON concept_states(learner_id)`,
    `CREATE INDEX IF NOT EXISTS idx_concept_states_review ON concept_states(learner_id, next_review)`,
    `CREATE INDEX IF NOT EXISTS idx_interactions_learner_created ON interactions(learner_id, created_at)`,
    `CREATE INDEX IF NOT EXISTS idx_interactions_learner_concept ON interactions(learner_id, concept, created_at)`,
    `CREATE INDEX IF NOT EXISTS idx_scheduled_alerts_learner_type ON scheduled_alerts(learner_id, alert_type, created_at)`,
    `CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires ON oauth_codes(expires_at)`,
}
for _, m := range idempotentMigrations {
    if _, err := db.Exec(m); err != nil {
        return fmt.Errorf("idempotent migration: %w", err)
    }
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add db/schema.sql db/migrations.go
git commit -m "feat: add oauth_codes, oauth_clients tables and DB indexes"
```

---

### Task 2: Store — Auth code and client persistence methods

**Files:**
- Modify: `db/store.go`

- [ ] **Step 1: Add CreateAuthCode method**

Add after the Scheduled Alerts section in `db/store.go`:

```go
// ─── OAuth Persistence ──────────────────────────────────────────────────────

func (s *Store) CreateAuthCode(code, learnerID, codeChallenge string, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO oauth_codes (code, learner_id, code_challenge, expires_at) VALUES (?, ?, ?, ?)`,
		code, learnerID, codeChallenge, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create auth code: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Add ConsumeAuthCode method**

This does SELECT + DELETE atomically. The auth code is single-use.

```go
// ConsumeAuthCode retrieves and deletes an auth code in one operation.
// Returns nil if the code does not exist or is expired.
func (s *Store) ConsumeAuthCode(code string) (*AuthCode, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	ac := &AuthCode{}
	err = tx.QueryRow(
		`SELECT code, learner_id, code_challenge, expires_at FROM oauth_codes WHERE code = ?`,
		code,
	).Scan(&ac.Code, &ac.LearnerID, &ac.CodeChallenge, &ac.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("consume auth code: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM oauth_codes WHERE code = ?`, code); err != nil {
		return nil, fmt.Errorf("delete auth code: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit consume auth code: %w", err)
	}
	return ac, nil
}
```

- [ ] **Step 3: Add the AuthCode struct to db/store.go**

Add at the top of the OAuth section:

```go
// AuthCode holds the authorization code state (persisted in DB).
type AuthCode struct {
	Code          string
	LearnerID     string
	CodeChallenge string
	ExpiresAt     time.Time
}
```

- [ ] **Step 4: Add CreateOAuthClient method**

```go
func (s *Store) CreateOAuthClient(clientID, clientName, redirectURIs string) error {
	_, err := s.db.Exec(
		`INSERT INTO oauth_clients (client_id, client_name, redirect_uris) VALUES (?, ?, ?)`,
		clientID, clientName, redirectURIs,
	)
	if err != nil {
		return fmt.Errorf("create oauth client: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Add cleanup methods**

```go
func (s *Store) CleanupExpiredCodes() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM oauth_codes WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup expired codes: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) CleanupExpiredRefreshTokens() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM refresh_tokens WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup expired refresh tokens: %w", err)
	}
	return result.RowsAffected()
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add db/store.go
git commit -m "feat: store methods for auth code and client persistence"
```

---

### Task 3: OAuth — Migrate from in-memory to DB persistence

**Files:**
- Modify: `auth/oauth.go`

- [ ] **Step 1: Remove in-memory state from OAuthServer**

Replace the struct and constructor. Remove `sync` import. The `codes` map and `codesMu` mutex are gone. Add `db` import alias if not present.

Old struct (`auth/oauth.go:26-32`):
```go
type OAuthServer struct {
	store   *db.Store
	baseURL string
	codes   map[string]*AuthCode
	codesMu sync.Mutex
	logger  *slog.Logger
}
```

New struct:
```go
type OAuthServer struct {
	store   *db.Store
	baseURL string
	logger  *slog.Logger
}
```

Old constructor (`auth/oauth.go:35-42`):
```go
func NewOAuthServer(store *db.Store, baseURL string, logger *slog.Logger) *OAuthServer {
	return &OAuthServer{
		store:   store,
		baseURL: baseURL,
		codes:   make(map[string]*AuthCode),
		logger:  logger,
	}
}
```

New constructor:
```go
func NewOAuthServer(store *db.Store, baseURL string, logger *slog.Logger) *OAuthServer {
	return &OAuthServer{
		store:   store,
		baseURL: baseURL,
		logger:  logger,
	}
}
```

- [ ] **Step 2: Remove the AuthCode struct from auth/oauth.go**

Delete lines 18-23 (`auth/oauth.go`):
```go
// AuthCode holds the authorization code state.
type AuthCode struct {
	LearnerID     string
	CodeChallenge string
	ExpiresAt     time.Time
}
```

This type is now defined in `db/store.go`.

- [ ] **Step 3: Update handleAuthorizePost to use Store**

Replace the in-memory write (`auth/oauth.go:165-171`):

```go
s.codesMu.Lock()
s.codes[code] = &AuthCode{
    LearnerID:     learnerID,
    CodeChallenge: codeChallenge,
    ExpiresAt:     time.Now().Add(5 * time.Minute),
}
s.codesMu.Unlock()
```

With:

```go
if err := s.store.CreateAuthCode(code, learnerID, codeChallenge, time.Now().Add(5*time.Minute)); err != nil {
    s.logger.Error("create auth code failed", "err", err)
    renderAuthPage(w, data, "Internal error. Please try again.")
    return
}
```

- [ ] **Step 4: Update handleAuthorizationCodeGrant to use Store**

Replace the in-memory read+delete (`auth/oauth.go:212-222`):

```go
s.codesMu.Lock()
authCode, ok := s.codes[code]
if ok {
    delete(s.codes, code)
}
s.codesMu.Unlock()

if !ok || time.Now().After(authCode.ExpiresAt) {
    s.logger.Error("token exchange: code not found or expired", "found", ok)
    writeTokenError(w, "invalid_grant", http.StatusBadRequest)
    return
}
```

With:

```go
authCode, err := s.store.ConsumeAuthCode(code)
if err != nil || time.Now().After(authCode.ExpiresAt) {
    s.logger.Error("token exchange: code not found or expired", "err", err)
    writeTokenError(w, "invalid_grant", http.StatusBadRequest)
    return
}
```

- [ ] **Step 5: Update handleDynamicClientRegistration to persist client**

In `handleDynamicClientRegistration` (`auth/oauth.go:306-341`), after generating `clientID`, add persistence. Insert after `clientID, err := generateCode()` error check:

```go
// Persist the client registration
redirectURIs := "[]"
if uris, ok := req["redirect_uris"]; ok {
    if b, err := json.Marshal(uris); err == nil {
        redirectURIs = string(b)
    }
}
clientName := ""
if name, ok := req["client_name"].(string); ok {
    clientName = name
}
if err := s.store.CreateOAuthClient(clientID, clientName, redirectURIs); err != nil {
    s.logger.Error("persist client registration failed", "err", err)
    http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
    return
}
```

- [ ] **Step 6: Remove the `sync` import**

The `sync` package was only used for `codesMu`. Remove it from the imports.

- [ ] **Step 7: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 8: Run existing tests**

Run: `go test ./... -v`
Expected: all existing tests still pass (algorithms + engine)

- [ ] **Step 9: Commit**

```bash
git add auth/oauth.go
git commit -m "feat: migrate OAuth auth codes and clients from memory to DB"
```

---

### Task 4: Rate Limiter — Token bucket implementation

**Files:**
- Create: `auth/ratelimit.go`

- [ ] **Step 1: Create auth/ratelimit.go with RateLimiter**

```go
package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// RateLimiter implements a per-IP token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   int     // max tokens
	stop    chan struct{}
}

// NewRateLimiter creates a rate limiter. rate is tokens/second, burst is max tokens.
// Starts a background goroutine to purge stale entries.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		stop:    make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Allow consumes one token for the given IP. Returns false if the bucket is empty.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		rl.buckets[ip] = &bucket{tokens: float64(rl.burst) - 1, lastTime: now}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastTime = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Stop shuts down the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, b := range rl.buckets {
				if b.lastTime.Before(cutoff) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

// RateLimitMiddleware wraps an http.Handler with rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded.
func RateLimitMiddleware(limiter *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !limiter.Allow(ip) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, `{"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add auth/ratelimit.go
git commit -m "feat: in-process token bucket rate limiter with cleanup"
```

---

### Task 5: Wire rate limiters to endpoints in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Create rate limiter instances and wrap endpoints**

In `main.go`, after creating `oauthServer` (line 75) and before registering routes, create the limiters:

```go
// Rate limiters (in-process, per-IP)
authLimiter := auth.NewRateLimiter(10.0/60, 10)       // 10/min for auth endpoints
registerLimiter := auth.NewRateLimiter(5.0/60, 5)     // 5/min for client registration
mcpLimiter := auth.NewRateLimiter(1, 60)              // 60/min for MCP API
defer authLimiter.Stop()
defer registerLimiter.Stop()
defer mcpLimiter.Stop()
```

- [ ] **Step 2: Change OAuth route registration to use rate-limited handlers**

Replace `oauthServer.RegisterRoutes(mux)` with individual route registration that wraps sensitive endpoints:

```go
// OAuth routes — rate-limit sensitive endpoints
mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthServer.HandleAuthServerMetadata)
mux.HandleFunc("GET /.well-known/oauth-protected-resource", oauthServer.HandleProtectedResourceMetadata)
mux.HandleFunc("GET /authorize", oauthServer.HandleAuthorizeGet)
mux.Handle("POST /authorize", auth.RateLimitMiddleware(authLimiter, http.HandlerFunc(oauthServer.HandleAuthorizePost)))
mux.Handle("POST /token", auth.RateLimitMiddleware(authLimiter, http.HandlerFunc(oauthServer.HandleToken)))
mux.Handle("POST /register", auth.RateLimitMiddleware(registerLimiter, http.HandlerFunc(oauthServer.HandleRegister)))
```

- [ ] **Step 3: Export OAuthServer handler methods**

In `auth/oauth.go`, rename the handler methods from unexported to exported so `main.go` can reference them individually:

- `handleAuthServerMetadata` → `HandleAuthServerMetadata`
- `handleProtectedResourceMetadata` → `HandleProtectedResourceMetadata`
- `handleAuthorizeGet` → `HandleAuthorizeGet`
- `handleAuthorizePost` → `HandleAuthorizePost`
- `handleToken` → `HandleToken`
- `handleAuthorizationCodeGrant` stays private (called by HandleToken)
- `handleRefreshTokenGrant` stays private (called by HandleToken)
- `handleDynamicClientRegistration` → `HandleRegister`

Update the `RegisterRoutes` method to use the new names (keep it for backwards compat but main.go won't call it):

```go
func (s *OAuthServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.HandleAuthServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.HandleProtectedResourceMetadata)
	mux.HandleFunc("GET /authorize", s.HandleAuthorizeGet)
	mux.HandleFunc("POST /authorize", s.HandleAuthorizePost)
	mux.HandleFunc("POST /token", s.HandleToken)
	mux.HandleFunc("POST /register", s.HandleRegister)
}
```

- [ ] **Step 4: Wrap MCP route with rate limiter**

Replace line 90 in `main.go`:

```go
mux.Handle("/mcp", auth.BearerMiddleware(baseURL, mcpHandler))
```

With:

```go
mux.Handle("/mcp", auth.RateLimitMiddleware(mcpLimiter, auth.BearerMiddleware(baseURL, mcpHandler)))
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Run existing tests**

Run: `go test ./... -v`
Expected: all existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add main.go auth/oauth.go
git commit -m "feat: wire rate limiters to auth, register, and MCP endpoints"
```

---

### Task 6: Webhook retry with exponential backoff

**Files:**
- Modify: `engine/scheduler.go`

- [ ] **Step 1: Add the retry helper**

Add after the `sendWebhook` method in `engine/scheduler.go`:

```go
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
```

- [ ] **Step 2: Add `strconv` to imports**

Add `"strconv"` to the import block in `engine/scheduler.go`.

- [ ] **Step 3: Rewrite sendDiscordEmbed to use doWithRetry**

Replace the existing `sendDiscordEmbed` method:

```go
func (s *Scheduler) sendDiscordEmbed(url string, payload discordPayload) error {
	body, _ := json.Marshal(payload)
	return s.doWithRetry(url, body)
}
```

- [ ] **Step 4: Rewrite sendWebhook to use doWithRetry**

Replace the existing `sendWebhook` method:

```go
func (s *Scheduler) sendWebhook(url, message string) error {
	body, _ := json.Marshal(map[string]string{"content": message})
	return s.doWithRetry(url, body)
}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 6: Run existing tests**

Run: `go test ./... -v`
Expected: all existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add engine/scheduler.go
git commit -m "feat: webhook retry with exponential backoff and 429 handling"
```

---

### Task 7: Scheduler cleanup job for expired codes and tokens

**Files:**
- Modify: `engine/scheduler.go`

- [ ] **Step 1: Add the cleanup method**

Add after the `sendDailyRecap` method:

```go
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
```

- [ ] **Step 2: Register the cleanup cron job**

In the `Start()` method, add after the daily recap job (before `s.cron.Start()`):

```go
// Cleanup expired auth codes and refresh tokens: hourly
if _, err := s.cron.AddFunc("0 * * * *", s.cleanupExpiredData); err != nil {
    return fmt.Errorf("add cleanup job: %w", err)
}
```

- [ ] **Step 3: Update the Start() log message**

Replace the logger line:

```go
s.logger.Info("scheduler started", "jobs", "critical(30m), reviews(9/13/19h), motivation(8h), recap(21h), cleanup(1h)")
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Run all tests**

Run: `go test ./... -v`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add engine/scheduler.go
git commit -m "feat: hourly cleanup job for expired auth codes and refresh tokens"
```

---

### Task 8: Final verification and integration commit

**Files:**
- None new — verify everything works together

- [ ] **Step 1: Run full build**

Run: `go build -o /dev/null ./...`
Expected: clean build, no errors

- [ ] **Step 2: Run all tests**

Run: `go test ./... -v -count=1`
Expected: all tests pass (algorithms + engine)

- [ ] **Step 3: Verify the binary starts**

Run: `LOG_LEVEL=debug go run . &` then `curl http://localhost:3000/health` then kill the process.
Expected: `{"status":"ok"}` and log output showing scheduler started with all 5 jobs.

- [ ] **Step 4: Push to remote**

```bash
git push origin main
```
