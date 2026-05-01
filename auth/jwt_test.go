// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func setTestSecret(t *testing.T) {
	t.Helper()
	raw := []byte("test-secret-32-bytes-for-hs256-ok")
	t.Setenv("JWT_SECRET", base64.StdEncoding.EncodeToString(raw))
	if err := LoadJWTSecret(); err != nil {
		t.Fatalf("load jwt secret: %v", err)
	}
}

func TestVerifyJWT_AcceptsValidIssuerAndAudience(t *testing.T) {
	setTestSecret(t)
	tok, err := GenerateJWT("https://issuer.example", "learner-1")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	sub, err := VerifyJWT(tok, "https://issuer.example")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if sub != "learner-1" {
		t.Fatalf("subject = %q, want learner-1", sub)
	}
}

func TestVerifyJWT_RejectsWrongIssuer(t *testing.T) {
	setTestSecret(t)
	tok, _ := GenerateJWT("https://issuer.example", "learner-1")
	if _, err := VerifyJWT(tok, "https://other.example"); err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestVerifyJWT_RejectsMissingAudience(t *testing.T) {
	setTestSecret(t)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "learner-1",
			Issuer:    "https://issuer.example",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Scope: "learner",
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := VerifyJWT(tok, "https://issuer.example"); err == nil {
		t.Fatal("expected error for missing audience")
	}
}

func TestVerifyJWT_RejectsAlgNone(t *testing.T) {
	// alg=none token, hand-crafted: header.payload.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x","iss":"https://issuer.example","aud":"learning-runtime/mcp","exp":99999999999}`))
	tok := header + "." + payload + "."

	setTestSecret(t)
	if _, err := VerifyJWT(tok, "https://issuer.example"); err == nil {
		t.Fatal("alg=none must be rejected")
	}
}

func TestMain(m *testing.M) {
	// Ensure tests don't accidentally inherit a JWT_SECRET from the host env.
	os.Unsetenv("JWT_SECRET")
	os.Exit(m.Run())
}
