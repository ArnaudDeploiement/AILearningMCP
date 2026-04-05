package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

func InitJWTSecret() error {
	secret := os.Getenv("JWT_SECRET")
	if secret != "" {
		jwtSecret = []byte(secret)
		return nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	jwtSecret = b
	secretPath := os.Getenv("DB_PATH")
	if secretPath == "" {
		secretPath = "."
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	return os.WriteFile(secretPath+".jwt_secret", []byte(encoded), 0600)
}

func LoadJWTSecret() error {
	secret := os.Getenv("JWT_SECRET")
	if secret != "" {
		jwtSecret = []byte(secret)
		return nil
	}
	secretPath := os.Getenv("DB_PATH")
	if secretPath == "" {
		secretPath = "."
	}
	data, err := os.ReadFile(secretPath + ".jwt_secret")
	if err != nil {
		return InitJWTSecret()
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return InitJWTSecret()
	}
	jwtSecret = decoded
	return nil
}

type Claims struct {
	jwt.RegisteredClaims
	Scope string `json:"scope"`
}

func GenerateJWT(learnerID string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   learnerID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Scope: "learner",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func VerifyJWT(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid claims")
	}
	return claims.Subject, nil
}
