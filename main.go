// AI Learning MCP Server
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"learning-runtime/auth"
	"learning-runtime/db"
	"learning-runtime/engine"
	"learning-runtime/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	}))

	port := os.Getenv("PORT")
	if port == "" { port = "3000" }
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" { dbPath = "./data/runtime.db" }
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" { baseURL = fmt.Sprintf("http://localhost:%s", port) }

	// Init JWT
	if err := auth.LoadJWTSecret(); err != nil {
		logger.Error("failed to load JWT secret", "err", err)
		os.Exit(1)
	}

	// Open DB
	os.MkdirAll("data", 0755)
	database, err := db.OpenDB(dbPath)
	if err != nil {
		logger.Error("failed to open database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		logger.Error("failed to migrate database", "err", err)
		os.Exit(1)
	}
	logger.Info("database ready", "path", dbPath)

	store := db.NewStore(database)

	// Create MCP server
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "learning-runtime",
		Version: "1.0.0",
	}, nil)

	// Register tools
	deps := &tools.Deps{Store: store, Logger: logger}
	tools.RegisterTools(mcpServer, deps)

	// Create MCP handler — disable localhost protection (behind Tailscale Funnel)
	// and allow Claude.ai cross-origin requests
	cop := http.NewCrossOriginProtection()
	cop.AddTrustedOrigin("https://claude.ai")
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
		CrossOriginProtection:      cop,
	})

	// OAuth server
	oauthServer := auth.NewOAuthServer(store, baseURL, logger)

	// HTTP mux
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := database.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy","error":"database unreachable"}`))
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Rate limiters (in-process, per-IP)
	authLimiter := auth.NewRateLimiter(10.0/60, 10)       // 10/min for auth endpoints
	registerLimiter := auth.NewRateLimiter(5.0/60, 5)     // 5/min for client registration
	mcpLimiter := auth.NewRateLimiter(1, 60)              // 60/min for MCP API
	defer authLimiter.Stop()
	defer registerLimiter.Stop()
	defer mcpLimiter.Stop()

	// OAuth routes — rate-limit sensitive endpoints
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthServer.HandleAuthServerMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", oauthServer.HandleProtectedResourceMetadata)
	mux.Handle("GET /authorize", auth.RateLimitMiddleware(authLimiter, http.HandlerFunc(oauthServer.HandleAuthorizeGet)))
	mux.Handle("POST /authorize", auth.RateLimitMiddleware(authLimiter, http.HandlerFunc(oauthServer.HandleAuthorizePost)))
	mux.Handle("POST /token", auth.RateLimitMiddleware(authLimiter, http.HandlerFunc(oauthServer.HandleToken)))
	mux.Handle("POST /register", auth.RateLimitMiddleware(registerLimiter, http.HandlerFunc(oauthServer.HandleRegister)))

	// MCP route (auth + rate limit protected)
	mux.Handle("/mcp", auth.RateLimitMiddleware(mcpLimiter, auth.BearerMiddleware(baseURL, mcpHandler)))

	// Start scheduler
	scheduler := engine.NewScheduler(store, logger)
	if err := scheduler.Start(); err != nil {
		logger.Error("failed to start scheduler", "err", err)
		os.Exit(1)
	}
	defer scheduler.Stop()

	// Wrap with CORS + request logging
	handler := requestLogger(logger, corsMiddleware(
		[]string{"https://claude.ai", baseURL},
		mux,
	))

	logger.Info("learning runtime starting", "port", port, "base_url", baseURL)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ua", r.UserAgent())
	})
}

func corsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug": return slog.LevelDebug
	case "warn": return slog.LevelWarn
	case "error": return slog.LevelError
	default: return slog.LevelInfo
	}
}
