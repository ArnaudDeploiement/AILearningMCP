package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"learning-runtime/db"
)

// AuthCode holds the authorization code state.
type AuthCode struct {
	LearnerID     string
	CodeChallenge string
	ExpiresAt     time.Time
}

// OAuthServer implements the OAuth 2.1 authorization server.
type OAuthServer struct {
	store   *db.Store
	baseURL string
	codes   map[string]*AuthCode
	codesMu sync.Mutex
	logger  *slog.Logger
}

// NewOAuthServer creates a new OAuthServer.
func NewOAuthServer(store *db.Store, baseURL string, logger *slog.Logger) *OAuthServer {
	return &OAuthServer{
		store:   store,
		baseURL: baseURL,
		codes:   make(map[string]*AuthCode),
		logger:  logger,
	}
}

// RegisterRoutes registers all OAuth endpoints on the given mux.
func (s *OAuthServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleAuthServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", s.handleProtectedResourceMetadata)
	mux.HandleFunc("GET /authorize", s.handleAuthorizeGet)
	mux.HandleFunc("POST /authorize", s.handleAuthorizePost)
	mux.HandleFunc("POST /token", s.handleToken)
	mux.HandleFunc("POST /register", s.handleDynamicClientRegistration)
}

func (s *OAuthServer) handleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	meta := map[string]interface{}{
		"issuer":                                s.baseURL,
		"authorization_endpoint":                s.baseURL + "/authorize",
		"token_endpoint":                        s.baseURL + "/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"learner"},
		"registration_endpoint":                    s.baseURL + "/register",
		"token_endpoint_auth_methods_supported":    []string{"none"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

func (s *OAuthServer) handleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	meta := map[string]interface{}{
		"resource":              s.baseURL + "/mcp",
		"authorization_servers": []string{s.baseURL},
		"scopes_supported":      []string{"learner"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

func (s *OAuthServer) handleAuthorizeGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data := authPageData{
		ClientID:            q.Get("client_id"),
		RedirectURI:         q.Get("redirect_uri"),
		ResponseType:        q.Get("response_type"),
		State:               q.Get("state"),
		CodeChallenge:       q.Get("code_challenge"),
		CodeChallengeMethod: q.Get("code_challenge_method"),
		Scope:               q.Get("scope"),
	}
	renderAuthPage(w, data, "")
}

func (s *OAuthServer) handleAuthorizePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	objective := r.FormValue("objective")
	webhookURL := r.FormValue("webhook_url")

	// OAuth hidden fields
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")

	data := authPageData{
		ClientID:            r.FormValue("client_id"),
		RedirectURI:         redirectURI,
		ResponseType:        r.FormValue("response_type"),
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: r.FormValue("code_challenge_method"),
		Scope:               r.FormValue("scope"),
	}

	if email == "" || password == "" {
		renderAuthPage(w, data, "Email and password are required.")
		return
	}

	var learnerID string

	// Check if learner exists.
	existing, err := s.store.GetLearnerByEmail(email)
	if err != nil {
		// Learner does not exist — register.
		if objective == "" {
			renderAuthPage(w, data, "Objective is required for new accounts.")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			s.logger.Error("bcrypt hash failed", "err", err)
			renderAuthPage(w, data, "Internal error. Please try again.")
			return
		}
		learner, err := s.store.CreateLearner(email, string(hash), objective, webhookURL)
		if err != nil {
			s.logger.Error("create learner failed", "err", err)
			renderAuthPage(w, data, "Could not create account. Please try again.")
			return
		}
		learnerID = learner.ID
	} else {
		// Learner exists — login.
		if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(password)); err != nil {
			renderAuthPage(w, data, "Invalid email or password.")
			return
		}
		learnerID = existing.ID
	}

	// Generate auth code.
	code, err := generateCode()
	if err != nil {
		s.logger.Error("generate code failed", "err", err)
		renderAuthPage(w, data, "Internal error. Please try again.")
		return
	}

	s.codesMu.Lock()
	s.codes[code] = &AuthCode{
		LearnerID:     learnerID,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(5 * time.Minute),
	}
	s.codesMu.Unlock()

	// Redirect to redirect_uri with code and state.
	redirectURL := redirectURI + "?code=" + code
	if state != "" {
		redirectURL += "&state=" + state
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *OAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
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
	clientID := r.FormValue("client_id")

	s.logger.Info("token exchange attempt", "code_len", len(code), "verifier_len", len(codeVerifier), "client_id", clientID)

	if code == "" || codeVerifier == "" {
		s.logger.Error("token exchange: missing code or verifier")
		writeTokenError(w, "invalid_request", http.StatusBadRequest)
		return
	}

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

	// Verify PKCE: SHA256(code_verifier) == code_challenge (base64url, no padding).
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	s.logger.Info("PKCE check", "stored_challenge", authCode.CodeChallenge, "computed", computed, "match", computed == authCode.CodeChallenge)
	if computed != authCode.CodeChallenge {
		s.logger.Error("token exchange: PKCE mismatch")
		writeTokenError(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	accessToken, err := GenerateJWT(authCode.LearnerID)
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

	accessToken, err := GenerateJWT(rt.LearnerID)
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

// handleDynamicClientRegistration implements RFC 7591.
// Claude.ai must register as an OAuth client before starting the auth flow.
func (s *OAuthServer) handleDynamicClientRegistration(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_client_metadata"}`, http.StatusBadRequest)
		return
	}

	s.logger.Info("dynamic client registration request", "body", req)

	// Generate a client_id for this client
	clientID, err := generateCode()
	if err != nil {
		http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
		return
	}

	// Echo back all client metadata + add our fields (RFC 7591 compliance)
	resp := map[string]interface{}{
		"client_id":                  clientID,
		"client_id_issued_at":        time.Now().Unix(),
		"client_name":               req["client_name"],
		"redirect_uris":             req["redirect_uris"],
		"grant_types":               []string{"authorization_code", "refresh_token"},
		"response_types":            []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      "learner",
	}

	s.logger.Info("dynamic client registered", "client_id", clientID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func generateCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
