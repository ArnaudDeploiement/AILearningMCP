package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

type contextKey string

const LearnerIDKey contextKey = "learner_id"

func BearerMiddleware(baseURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		slog.Info("auth middleware", "has_bearer", strings.HasPrefix(authHeader, "Bearer "), "token_len", len(authHeader))
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(
				`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, baseURL,
			))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		learnerID, err := VerifyJWT(tokenStr)
		slog.Info("jwt verify", "learner_id", learnerID, "err", err)
		if err != nil {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(
				`Bearer resource_metadata="%s/.well-known/oauth-protected-resource", error="invalid_token"`, baseURL,
			))
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), LearnerIDKey, learnerID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetLearnerID(ctx context.Context) string {
	id, _ := ctx.Value(LearnerIDKey).(string)
	return id
}
