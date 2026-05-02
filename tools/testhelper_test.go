// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"tutor-mcp/auth"
	"tutor-mcp/db"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

// global counter to make DSN names unique across parallel goroutines.
var toolsDSNCounter int64

// setupToolsTest spins up a fresh in-memory SQLite DB, runs migrations and
// pre-creates two test learners (L_owner, L_attacker) used to verify
// authorization rules. Mirrors the canonical setupCalibTest helper.
func setupToolsTest(t *testing.T) (*db.Store, *Deps) {
	t.Helper()
	n := atomic.AddInt64(&toolsDSNCounter, 1)
	dsn := fmt.Sprintf("file:toolsmem_%s_%d?mode=memory&cache=shared", t.Name(), n)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(raw); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"L_owner", "L_attacker"} {
		_, err := raw.Exec(
			`INSERT INTO learners (id, email, password_hash, objective, created_at) VALUES (?, ?, 'hash', 'test', ?)`,
			id, id+"@test.com", now,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { raw.Close() })
	store := db.NewStore(raw)
	deps := &Deps{
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return store, deps
}

// callTool spins up an MCP server with the provided register function, injects
// the given learnerID into the receiving context, then calls the tool with the
// provided arguments. When learnerID is empty no auth context is injected.
func callTool(
	t *testing.T,
	deps *Deps,
	register func(*mcp.Server, *Deps),
	learnerID, name string,
	args any,
) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	register(server, deps)
	if learnerID != "" {
		server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				ctx = context.WithValue(ctx, auth.LearnerIDKey, learnerID)
				return next(ctx, method, req)
			}
		})
	}

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	argsJSON, _ := json.Marshal(args)
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: json.RawMessage(argsJSON),
	})
	if err != nil {
		t.Fatalf("CallTool transport error for %q: %v", name, err)
	}
	return res
}

// resultText extracts the first text content from a CallToolResult.
func resultText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// decodeResult parses the JSON returned in the first text-content block.
func decodeResult(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	out := map[string]any{}
	txt := resultText(res)
	if txt == "" {
		return out
	}
	_ = json.Unmarshal([]byte(txt), &out)
	return out
}
