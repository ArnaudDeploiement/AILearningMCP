// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

// AI Learning MCP Server
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"tutor-mcp/auth"
	"tutor-mcp/db"
	"tutor-mcp/engine"
	"tutor-mcp/tools"

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
		Name:    "tutor-mcp",
		Version: "0.2.0",
	}, nil)

	// Register tools
	deps := &tools.Deps{Store: store, Logger: logger}
	tools.RegisterTools(mcpServer, deps)

	// Create MCP handler — disable localhost protection (server is reached via a public reverse proxy)
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
	mcpLimiter := auth.NewRateLimiter(20.0/60, 20)        // 20/min for MCP API
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

	// Wrap with recovery + request logging + security headers + CORS.
	// Order: recovery outermost so panics in any inner middleware are caught.
	handler := recoveryMiddleware(logger, requestLogger(logger, securityHeaders(baseURL, corsMiddleware(
		[]string{"https://claude.ai", baseURL},
		mux,
	))))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	shutdownErr := make(chan error, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		shutdownErr <- server.Shutdown(ctx)
	}()

	logger.Info("tutor mcp starting", "port", port, "base_url", baseURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
	if err := <-shutdownErr; err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	logger.Info("server stopped cleanly")
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders applies baseline browser-security headers on every response.
// HSTS is only emitted when BASE_URL is HTTPS so local HTTP development isn't pinned.
func securityHeaders(baseURL string, next http.Handler) http.Handler {
	hsts := strings.HasPrefix(baseURL, "https://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if hsts {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware turns a panic in any downstream handler into a 500
// instead of taking the whole process down. The stack trace is logged, never
// returned to the client.
func recoveryMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered",
					"path", r.URL.Path,
					"method", r.Method,
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"internal_error"}`))
			}
		}()
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
