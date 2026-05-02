// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// helperOKHandler echoes the learner ID injected into context, so tests can
// assert that BearerMiddleware actually populated it.
func helperOKHandler(t *testing.T, wantLearnerID string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := GetLearnerID(r.Context())
		if wantLearnerID != "" && got != wantLearnerID {
			t.Errorf("learner_id in ctx = %q, want %q", got, wantLearnerID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok:"+got)
	})
}

func TestBearerMiddleware_MissingAuthHeader(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	mw := BearerMiddleware("https://test.example", next)

	req := httptest.NewRequest("GET", "/mcp", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("next handler must not be invoked when auth header missing")
	}
	wa := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wa, `resource_metadata="https://test.example/.well-known/oauth-protected-resource"`) {
		t.Fatalf("WWW-Authenticate missing resource_metadata: %q", wa)
	}
	// No invalid_token marker on missing header (only on invalid).
	if strings.Contains(wa, `error="invalid_token"`) {
		t.Fatalf("missing token should NOT produce invalid_token marker: %q", wa)
	}
}

func TestBearerMiddleware_NonBearerScheme(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	mw := BearerMiddleware("https://test.example", next)

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("next must not be called for non-Bearer scheme")
	}
}

func TestBearerMiddleware_InvalidToken(t *testing.T) {
	setTestSecret(t)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	mw := BearerMiddleware("https://test.example", next)

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("next must not be called when token invalid")
	}
	wa := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wa, `error="invalid_token"`) {
		t.Fatalf("expected invalid_token marker, got %q", wa)
	}
	if !strings.Contains(wa, `resource_metadata="https://test.example/.well-known/oauth-protected-resource"`) {
		t.Fatalf("missing resource_metadata in WWW-Authenticate: %q", wa)
	}
}

func TestBearerMiddleware_WrongIssuerToken(t *testing.T) {
	setTestSecret(t)

	tok, err := GenerateJWT("https://other.example", "learner-2")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	mw := BearerMiddleware("https://test.example", next)

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("next must not be called when issuer mismatches")
	}
}

func TestBearerMiddleware_ValidTokenInjectsLearnerID(t *testing.T) {
	setTestSecret(t)

	const learnerID = "learner-xyz"
	tok, err := GenerateJWT("https://test.example", learnerID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	mw := BearerMiddleware("https://test.example", helperOKHandler(t, learnerID))

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ok:"+learnerID) {
		t.Fatalf("body = %q, expected to contain learner id", rec.Body.String())
	}
	// On success, no WWW-Authenticate is set.
	if wa := rec.Header().Get("WWW-Authenticate"); wa != "" {
		t.Fatalf("WWW-Authenticate must not be set on success: %q", wa)
	}
}

func TestBearerMiddleware_EmptyBearerToken(t *testing.T) {
	setTestSecret(t)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	mw := BearerMiddleware("https://test.example", next)

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer ") // technically prefix matches, but token is ""
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Fatal("next must not be called for empty bearer token")
	}
}

func TestGetLearnerID_NoValueReturnsEmpty(t *testing.T) {
	if got := GetLearnerID(context.Background()); got != "" {
		t.Fatalf("GetLearnerID with empty ctx = %q, want empty", got)
	}
}

func TestGetLearnerID_WrongTypeReturnsEmpty(t *testing.T) {
	// Storing a non-string under LearnerIDKey should not panic and should return "".
	ctx := context.WithValue(context.Background(), LearnerIDKey, 12345)
	if got := GetLearnerID(ctx); got != "" {
		t.Fatalf("GetLearnerID with non-string value = %q, want empty", got)
	}
}

func TestGetLearnerID_StringValueReturned(t *testing.T) {
	ctx := context.WithValue(context.Background(), LearnerIDKey, "abc-123")
	if got := GetLearnerID(ctx); got != "abc-123" {
		t.Fatalf("GetLearnerID = %q, want abc-123", got)
	}
}
