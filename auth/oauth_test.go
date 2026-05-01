package auth

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"learning-runtime/db"

	_ "modernc.org/sqlite"
)

var testDBCounter int

func newTestServer(t *testing.T) (*OAuthServer, *db.Store) {
	t.Helper()
	testDBCounter++
	dsn := fmt.Sprintf("file:oauth_memdb_%s_%d?mode=memory&cache=shared", t.Name(), testDBCounter)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(sqldb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { sqldb.Close() })

	store := db.NewStore(sqldb)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewOAuthServer(store, "https://test.example", logger), store
}

func seedClient(t *testing.T, store *db.Store, clientID, redirectURI string) {
	t.Helper()
	if err := store.CreateOAuthClient(clientID, "Test Client", fmt.Sprintf(`[%q]`, redirectURI)); err != nil {
		t.Fatalf("seed client: %v", err)
	}
}

func seedLearner(t *testing.T, store *db.Store, email, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	l, err := store.CreateLearner(email, string(hash), "", "")
	if err != nil {
		t.Fatalf("create learner: %v", err)
	}
	return l.ID
}

// ─── validateRegistrationRedirectURIs ────────────────────────────────────────

func TestValidateRegistrationRedirectURIs(t *testing.T) {
	cases := []struct {
		name    string
		uris    []string
		wantErr bool
	}{
		{"https accepted", []string{"https://claude.ai/cb"}, false},
		{"http public rejected", []string{"http://example.com/cb"}, true},
		{"localhost http accepted", []string{"http://localhost:8080/cb"}, false},
		{"127.0.0.1 http accepted", []string{"http://127.0.0.1:8080/cb"}, false},
		{"private 10/8 rejected", []string{"https://10.0.0.1/cb"}, true},
		{"private 192.168 rejected", []string{"https://192.168.1.1/cb"}, true},
		{"private 172.16 rejected", []string{"https://172.16.5.5/cb"}, true},
		{"link-local 169.254 rejected", []string{"https://169.254.169.254/cb"}, true},
		{"public IP https accepted", []string{"https://8.8.8.8/cb"}, false},
		{"too many uris", []string{"https://a/1", "https://a/2", "https://a/3", "https://a/4", "https://a/5", "https://a/6"}, true},
		{"too long uri", []string{"https://a.example/" + strings.Repeat("x", 513)}, true},
		{"malformed uri", []string{"://bad"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRegistrationRedirectURIs(tc.uris)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		ip      string
		private bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"fc00::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700::1", false},
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("bad test IP: %s", tc.ip)
			}
			if got := isPrivateIP(ip); got != tc.private {
				t.Fatalf("isPrivateIP(%s) = %v, want %v", tc.ip, got, tc.private)
			}
		})
	}
}

// ─── redirect_uri enforcement on GET + POST ─────────────────────────────────

func TestAuthorizeGet_UnregisteredRedirectURI(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	req := httptest.NewRequest("GET", "/authorize?client_id=cid&redirect_uri=https://attacker.example/evil&response_type=code", nil)
	rec := httptest.NewRecorder()
	s.HandleAuthorizeGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("unexpected redirect: %s", loc)
	}
}

func TestAuthorizeGet_ValidRedirectURI_SetsCSRFCookie(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	req := httptest.NewRequest("GET", "/authorize?client_id=cid&redirect_uri=https://good.example/cb&response_type=code&state=xyz&code_challenge=abc&code_challenge_method=S256", nil)
	rec := httptest.NewRecorder()
	s.HandleAuthorizeGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c
		}
	}
	if csrfCookie == nil {
		t.Fatal("csrf_token cookie not set")
	}
	if !csrfCookie.HttpOnly || !csrfCookie.Secure || csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags wrong: %+v", csrfCookie)
	}
	if csrfCookie.Path != "/authorize" {
		t.Fatalf("cookie path = %s", csrfCookie.Path)
	}
	if !strings.Contains(rec.Body.String(), csrfCookie.Value) {
		t.Fatal("csrf token not rendered in page body")
	}
}

func TestAuthorizePost_UnregisteredRedirectURI(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	form := url.Values{}
	form.Set("csrf_token", "tkn")
	form.Set("mode", "login")
	form.Set("client_id", "cid")
	form.Set("redirect_uri", "https://attacker.example/evil")
	form.Set("email", "u@e.com")
	form.Set("password", "password123")

	req := httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "tkn"})
	rec := httptest.NewRecorder()
	s.HandleAuthorizePost(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("unexpected redirect: %s", loc)
	}
}

// ─── CSRF on POST /authorize ───────────────────────────────────────────────

func TestAuthorizePost_NoCSRFCookie(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	form := url.Values{}
	form.Set("csrf_token", "field-only")
	form.Set("mode", "login")
	form.Set("client_id", "cid")
	form.Set("redirect_uri", "https://good.example/cb")

	req := httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.HandleAuthorizePost(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthorizePost_CSRFMismatch(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	form := url.Values{}
	form.Set("csrf_token", "field-value")
	form.Set("mode", "login")
	form.Set("client_id", "cid")
	form.Set("redirect_uri", "https://good.example/cb")

	req := httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "cookie-value-different"})
	rec := httptest.NewRecorder()
	s.HandleAuthorizePost(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthorizeGet_RendersStrictCSPWithoutInlineHandlers(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	req := httptest.NewRequest("GET", "/authorize?client_id=cid&redirect_uri=https://good.example/cb&response_type=code&code_challenge=abc&code_challenge_method=S256", nil)
	rec := httptest.NewRecorder()
	s.HandleAuthorizeGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("missing Content-Security-Policy header")
	}
	for _, must := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'self'", "'nonce-"} {
		if !strings.Contains(csp, must) {
			t.Fatalf("CSP missing %q: %s", must, csp)
		}
	}
	body := rec.Body.String()
	if strings.Contains(body, "onclick=") {
		t.Fatal("inline onclick handler still present in rendered page")
	}
}

func TestAuthorizeGet_PublicClientWithoutPKCERejected(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	req := httptest.NewRequest("GET", "/authorize?client_id=cid&redirect_uri=https://good.example/cb&response_type=code&state=xyz", nil)
	rec := httptest.NewRecorder()
	s.HandleAuthorizeGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public client without code_challenge should be rejected, got %d", rec.Code)
	}
}

func TestAuthorizeGet_PublicClientWithPlainPKCERejected(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")

	req := httptest.NewRequest("GET", "/authorize?client_id=cid&redirect_uri=https://good.example/cb&response_type=code&code_challenge=abc&code_challenge_method=plain", nil)
	rec := httptest.NewRecorder()
	s.HandleAuthorizeGet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("plain PKCE method should be rejected, got %d", rec.Code)
	}
}

func TestAuthorizePost_CSRFMatch_InvalidCreds(t *testing.T) {
	s, store := newTestServer(t)
	seedClient(t, store, "cid", "https://good.example/cb")
	seedLearner(t, store, "u@example.com", "correct-password")

	form := url.Values{}
	form.Set("csrf_token", "matching-token")
	form.Set("mode", "login")
	form.Set("client_id", "cid")
	form.Set("redirect_uri", "https://good.example/cb")
	form.Set("code_challenge", "abc")
	form.Set("code_challenge_method", "S256")
	form.Set("email", "u@example.com")
	form.Set("password", "wrong-password")

	req := httptest.NewRequest("POST", "/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "matching-token"})
	rec := httptest.NewRecorder()
	s.HandleAuthorizePost(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("should not be 403 (csrf passed), got 403")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for invalid creds", rec.Code)
	}
}
