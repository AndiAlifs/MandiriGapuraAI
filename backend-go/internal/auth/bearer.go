package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

var ErrMissingCredentials = errors.New("missing credentials")
var ErrInvalidTokenFormat = errors.New("invalid token format")

func ExtractBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", ErrMissingCredentials
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(authHeader), strings.ToLower(prefix)) {
		return "", ErrInvalidTokenFormat
	}

	token := strings.TrimSpace(authHeader[len(prefix):])
	if token == "" {
		return "", ErrMissingCredentials
	}

	return token, nil
}

func HashAPIKey(plainToken string) string {
	hash := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(hash[:])
}

func GeneratePlainAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	encoded := base64.RawURLEncoding.EncodeToString(b)
	return "gapura-sk-" + encoded
}
