// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/bcrypt"

	"learning-runtime/db"
)

// OAuthServer implements the OAuth 2.1 authorization server.
type OAuthServer struct {
	store   *db.Store
	baseURL string
	logger  *slog.Logger
}

// NewOAuthServer creates a new OAuthServer.
func NewOAuthServer(store *db.Store, baseURL string, logger *slog.Logger) *OAuthServer {
	return &OAuthServer{
		store:   store,
		baseURL: baseURL,
		logger:  logger,
	}
}

// RegisterRoutes registers all OAuth endpoints on the given mux.
func (s *OAuthServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.HandleAuthServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.HandleProtectedResourceMetadata)
	mux.HandleFunc("GET /authorize", s.HandleAuthorizeGet)
	mux.HandleFunc("POST /authorize", s.HandleAuthorizePost)
	mux.HandleFunc("POST /token", s.HandleToken)
	mux.HandleFunc("POST /register", s.HandleRegister)
}

func (s *OAuthServer) HandleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	meta := map[string]interface{}{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/authorize",
		"token_endpoint":                        s.baseURL + "/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"learner"},
		"registration_endpoint":                    s.baseURL + "/register",
		"token_endpoint_auth_methods_supported":    []string{"none", "client_secret_basic", "client_secret_post"},
		// RFC 9207: we include iss in authorization responses.
		"authorization_response_iss_parameter_supported": true,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

func (s *OAuthServer) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	meta := map[string]interface{}{
		"resource":              s.baseURL + "/mcp",
		"authorization_servers": []string{s.baseURL},
		"scopes_supported":      []string{"learner"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

// validateRedirectURI checks that the supplied redirectURI is strictly equal
// to one of the URIs registered for the given clientID. No prefix / wildcard.
func (s *OAuthServer) validateRedirectURI(clientID, redirectURI string) error {
	if clientID == "" || redirectURI == "" {
		return fmt.Errorf("missing client_id or redirect_uri")
	}
	client, err := s.store.GetOAuthClient(clientID)
	if err != nil {
		return fmt.Errorf("unknown client")
	}
	var registered []string
	if err := json.Unmarshal([]byte(client.RedirectURIs), &registered); err != nil {
		return fmt.Errorf("malformed registration")
	}
	for _, u := range registered {
		if u == redirectURI {
			return nil
		}
	}
	return fmt.Errorf("redirect_uri not registered")
}

func (s *OAuthServer) HandleAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")

	if err := s.validateRedirectURI(clientID, redirectURI); err != nil {
		s.logger.Debug("authorize GET: redirect_uri rejected", "err", err, "client_id", clientID)
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	if err := s.requirePKCEForPublicClient(clientID, codeChallenge, codeChallengeMethod); err != nil {
		s.logger.Debug("authorize GET: PKCE missing for public client", "err", err, "client_id", clientID)
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	s.logger.Info("authorize GET", "client_id", clientID, "state_len", len(q.Get("state")))

	csrfToken, err := generateCSRFToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		Path:     "/authorize",
		MaxAge:   3600,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	data := authPageData{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		ResponseType:        q.Get("response_type"),
		State:               q.Get("state"),
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scope:               q.Get("scope"),
		CSRFToken:           csrfToken,
	}
	renderAuthPage(w, data, "", "login")
}

// requirePKCEForPublicClient enforces RFC 9700 §2.1.1: public clients
// (no stored secret) MUST use PKCE with S256. Confidential clients are
// still allowed to skip PKCE — they authenticate via client_secret.
func (s *OAuthServer) requirePKCEForPublicClient(clientID, codeChallenge, method string) error {
	if clientID == "" {
		return fmt.Errorf("missing client_id")
	}
	client, err := s.store.GetOAuthClient(clientID)
	if err != nil {
		return fmt.Errorf("unknown client")
	}
	if client.ClientSecretHash != "" {
		// Confidential client: PKCE optional.
		return nil
	}
	if codeChallenge == "" {
		return fmt.Errorf("code_challenge required for public clients")
	}
	if method != "S256" {
		return fmt.Errorf("code_challenge_method must be S256")
	}
	return nil
}

func (s *OAuthServer) HandleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	cookie, cerr := r.Cookie("csrf_token")
	formCSRF := r.FormValue("csrf_token")
	if cerr != nil || cookie.Value == "" || formCSRF == "" ||
		subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(formCSRF)) != 1 {
		http.Error(w, "forbidden: csrf check failed", http.StatusForbidden)
		return
	}

	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	if err := s.validateRedirectURI(clientID, redirectURI); err != nil {
		s.logger.Debug("authorize POST: redirect_uri rejected", "err", err, "client_id", clientID)
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	if err := s.requirePKCEForPublicClient(clientID, codeChallenge, codeChallengeMethod); err != nil {
		s.logger.Debug("authorize POST: PKCE missing for public client", "err", err, "client_id", clientID)
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	mode := r.FormValue("mode") // "login" or "register"
	email := r.FormValue("email")
	password := r.FormValue("password")

	state := r.FormValue("state")

	data := authPageData{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		ResponseType:        r.FormValue("response_type"),
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scope:               r.FormValue("scope"),
		CSRFToken:           formCSRF,
	}

	if email == "" || password == "" {
		renderAuthPage(w, data, "Email and password are required.", mode)
		return
	}

	var learnerID string

	if mode == "register" {
		// Registration flow
		passwordConfirm := r.FormValue("password_confirm")
		if password != passwordConfirm {
			renderAuthPage(w, data, "Passwords do not match.", "register")
			return
		}
		if len(password) < 6 {
			renderAuthPage(w, data, "Password must be at least 6 characters.", "register")
			return
		}

		// Check if email already taken
		if existing, _ := s.store.GetLearnerByEmail(email); existing != nil {
			renderAuthPage(w, data, "An account with this email already exists.", "register")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			s.logger.Error("bcrypt hash failed", "err", err)
			renderAuthPage(w, data, "Internal error. Please try again.", "register")
			return
		}
		learner, err := s.store.CreateLearner(email, string(hash), "", "")
		if err != nil {
			s.logger.Error("create learner failed", "err", err)
			renderAuthPage(w, data, "Could not create account. Please try again.", "register")
			return
		}
		learnerID = learner.ID
	} else {
		// Login flow
		existing, err := s.store.GetLearnerByEmail(email)
		if err != nil {
			renderAuthPage(w, data, "Invalid email or password.", "login")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(password)); err != nil {
			renderAuthPage(w, data, "Invalid email or password.", "login")
			return
		}
		learnerID = existing.ID
	}

	// Generate auth code
	code, err := generateCode()
	if err != nil {
		s.logger.Error("generate code failed", "err", err)
		renderAuthPage(w, data, "Internal error. Please try again.", mode)
		return
	}

	if err := s.store.CreateAuthCode(code, learnerID, codeChallenge, clientID, time.Now().Add(5*time.Minute)); err != nil {
		s.logger.Error("create auth code failed", "err", err)
		renderAuthPage(w, data, "Internal error. Please try again.", mode)
		return
	}

	u, err := url.Parse(redirectURI)
	if err != nil {
		s.logger.Error("parse redirect_uri failed", "err", err)
		renderAuthPage(w, data, "Internal error. Please try again.", mode)
		return
	}
	qv := u.Query()
	qv.Set("code", code)
	if state != "" {
		qv.Set("state", state)
	}
	// RFC 9207 mix-up mitigation: clients (e.g. Mistral) may require iss in the callback.
	qv.Set("iss", s.baseURL)
	u.RawQuery = qv.Encode()
	s.logger.Info("authorize POST redirect", "state_len", len(state), "redirect_host", u.Host)
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// extractClientCredentials returns (client_id, client_secret) using HTTP Basic
// (client_secret_basic) first, then falling back to form fields (client_secret_post).
// Either or both may be empty if not supplied.
func extractClientCredentials(r *http.Request) (string, string) {
	if id, secret, ok := r.BasicAuth(); ok {
		return id, secret
	}
	return r.FormValue("client_id"), r.FormValue("client_secret")
}

// verifyClientAuth enforces secret-based authentication for confidential clients.
// Public clients (empty stored hash) pass through and rely on PKCE.
// The stored hash is a bcrypt digest; CompareHashAndPassword is constant-time.
func verifyClientAuth(client *db.OAuthClient, suppliedSecret string) error {
	if client.ClientSecretHash == "" {
		return nil
	}
	if suppliedSecret == "" {
		return fmt.Errorf("invalid_client")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(suppliedSecret)); err != nil {
		return fmt.Errorf("invalid_client")
	}
	return nil
}

func (s *OAuthServer) HandleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleAuthorizationCodeGrant(w, r)
	case "refresh_token":
		s.handleRefreshTokenGrant(w, r)
	default:
		http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
	}
}

func (s *OAuthServer) handleAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	clientID, clientSecret := extractClientCredentials(r)

	s.logger.Debug("token exchange attempt", "code_len", len(code), "verifier_len", len(codeVerifier), "client_id", clientID)

	if code == "" || codeVerifier == "" || clientID == "" {
		s.logger.Debug("token exchange: missing code, verifier or client_id")
		writeTokenError(w, "invalid_request", http.StatusBadRequest)
		return
	}

	client, err := s.store.GetOAuthClient(clientID)
	if err != nil {
		s.logger.Debug("token exchange: unknown client", "client_id", clientID)
		writeTokenError(w, "invalid_client", http.StatusUnauthorized)
		return
	}
	if err := verifyClientAuth(client, clientSecret); err != nil {
		s.logger.Debug("token exchange: client auth failed", "client_id", clientID)
		writeTokenError(w, "invalid_client", http.StatusUnauthorized)
		return
	}

	authCode, err := s.store.ConsumeAuthCode(code, clientID)
	if err != nil || time.Now().After(authCode.ExpiresAt) {
		s.logger.Debug("token exchange: code not found or expired", "err", err)
		writeTokenError(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	// Verify PKCE: SHA256(code_verifier) == code_challenge (base64url, no padding).
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	if subtle.ConstantTimeCompare([]byte(computed), []byte(authCode.CodeChallenge)) != 1 {
		s.logger.Debug("token exchange: PKCE mismatch")
		writeTokenError(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	accessToken, err := GenerateJWT(s.baseURL, authCode.LearnerID)
	if err != nil {
		s.logger.Error("generate jwt failed", "err", err)
		writeTokenError(w, "server_error", http.StatusInternalServerError)
		return
	}

	rt, err := s.store.CreateRefreshToken(authCode.LearnerID)
	if err != nil {
		s.logger.Error("create refresh token failed", "err", err)
		writeTokenError(w, "server_error", http.StatusInternalServerError)
		return
	}

	writeTokenResponse(w, accessToken, rt.Token)
}

func (s *OAuthServer) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		writeTokenError(w, "invalid_request", http.StatusBadRequest)
		return
	}

	// Confidential clients must authenticate on refresh too. Public clients
	// (no client_id supplied) keep the existing PKCE-only flow.
	clientID, clientSecret := extractClientCredentials(r)
	if clientID != "" {
		client, err := s.store.GetOAuthClient(clientID)
		if err != nil {
			writeTokenError(w, "invalid_client", http.StatusUnauthorized)
			return
		}
		if err := verifyClientAuth(client, clientSecret); err != nil {
			writeTokenError(w, "invalid_client", http.StatusUnauthorized)
			return
		}
	}

	rt, err := s.store.GetRefreshToken(refreshToken)
	if err != nil {
		writeTokenError(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	// Delete old refresh token (rotation).
	if err := s.store.DeleteRefreshToken(refreshToken); err != nil {
		s.logger.Error("delete refresh token failed", "err", err)
		writeTokenError(w, "server_error", http.StatusInternalServerError)
		return
	}

	accessToken, err := GenerateJWT(s.baseURL, rt.LearnerID)
	if err != nil {
		s.logger.Error("generate jwt failed", "err", err)
		writeTokenError(w, "server_error", http.StatusInternalServerError)
		return
	}

	newRT, err := s.store.CreateRefreshToken(rt.LearnerID)
	if err != nil {
		s.logger.Error("create refresh token failed", "err", err)
		writeTokenError(w, "server_error", http.StatusInternalServerError)
		return
	}

	writeTokenResponse(w, accessToken, newRT.Token)
}

func writeTokenResponse(w http.ResponseWriter, accessToken, refreshToken string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  accessToken,
		"token_type":    "bearer",
		"expires_in":    86400,
		"refresh_token": refreshToken,
		"scope":         "learner",
	})
}

func writeTokenError(w http.ResponseWriter, errCode string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": errCode})
}

// validateRegistrationRedirectURIs enforces https-or-loopback and rejects
// private IPs to prevent SSRF / open-redirect through client registration.
func validateRegistrationRedirectURIs(uris []string) error {
	if len(uris) > 5 {
		return fmt.Errorf("too many redirect_uris (max 5)")
	}
	for _, raw := range uris {
		if len(raw) > 512 {
			return fmt.Errorf("redirect_uri too long (max 512 chars)")
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("invalid redirect_uri: %w", err)
		}
		host := u.Hostname()
		if host == "localhost" || host == "127.0.0.1" {
			continue
		}
		if u.Scheme != "https" {
			return fmt.Errorf("redirect_uri must use https (got %q)", u.Scheme)
		}
		if ip := net.ParseIP(host); ip != nil {
			if isPrivateIP(ip) {
				return fmt.Errorf("redirect_uri points to private IP range")
			}
		}
	}
	return nil
}

var privateCIDRs = func() []*net.IPNet {
	blocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
	}
	var out []*net.IPNet
	for _, b := range blocks {
		_, n, err := net.ParseCIDR(b)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}()

func isPrivateIP(ip net.IP) bool {
	for _, n := range privateCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// HandleRegister implements RFC 7591 dynamic client registration.
// Claude.ai must register as an OAuth client before starting the auth flow.
func (s *OAuthServer) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_client_metadata"}`, http.StatusBadRequest)
		return
	}

	clientName := ""
	if name, ok := req["client_name"].(string); ok {
		clientName = name
	}

	// RFC 7591: confidential clients announce a secret-based auth method.
	authMethod := "none"
	if m, ok := req["token_endpoint_auth_method"].(string); ok && m != "" {
		authMethod = m
	}
	confidential := authMethod == "client_secret_basic" || authMethod == "client_secret_post"

	s.logger.Info("dynamic client registration request", "client_name", clientName, "auth_method", authMethod, "raw_keys", mapKeys(req))

	var uris []string
	if raw, ok := req["redirect_uris"]; ok {
		if arr, ok := raw.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					uris = append(uris, s)
				}
			}
		}
	}
	s.logger.Info("registration redirect_uris", "uris", uris)
	if len(uris) == 0 {
		writeRegistrationError(w, "invalid_redirect_uri", "at least one redirect_uri required")
		return
	}
	if err := validateRegistrationRedirectURIs(uris); err != nil {
		writeRegistrationError(w, "invalid_redirect_uri", err.Error())
		return
	}

	clientID, err := generateCode()
	if err != nil {
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}

	redirectURIsJSON, err := json.Marshal(uris)
	if err != nil {
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}

	var clientSecret, secretHash string
	if confidential {
		clientSecret, err = generateCode()
		if err != nil {
			http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
			return
		}
		hash, herr := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
		if herr != nil {
			s.logger.Error("bcrypt client secret failed", "err", herr)
			http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
			return
		}
		secretHash = string(hash)
	}

	if err := s.store.CreateOAuthClientWithSecret(clientID, clientName, string(redirectURIsJSON), secretHash); err != nil {
		s.logger.Error("persist client registration failed", "err", err)
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}

	// Echo back all client metadata + add our fields (RFC 7591 compliance)
	resp := map[string]interface{}{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":                clientName,
		"redirect_uris":              uris,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": authMethod,
		"scope":                      "learner",
	}
	if confidential {
		resp["client_secret"] = clientSecret
		// 0 = secret never expires (RFC 7591 §3.2.1)
		resp["client_secret_expires_at"] = 0
	}

	// RFC 7592 hint fields. Some clients (e.g. Mistral Le Chat) reject the
	// registration response with "Missing oauth2 metadata secrets" when these
	// are absent, even though they never call the configuration endpoint.
	registrationToken, err := generateCode()
	if err == nil {
		resp["registration_access_token"] = registrationToken
		resp["registration_client_uri"] = s.baseURL + "/register/" + clientID
	}

	s.logger.Info("dynamic client registered", "client_id", clientID, "confidential", confidential)

	// RFC 6749 §5.1: responses with credentials must not be cached.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func mapKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func writeRegistrationError(w http.ResponseWriter, errCode, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

func generateCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
