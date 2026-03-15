package auth

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

var ErrMissingCredentials = errors.New("missing credentials")

func ParseCredentials(r *http.Request) (string, string, error) {
	if username := r.Header.Get("X-Username"); username != "" {
		return username, r.Header.Get("X-Password"), nil
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", "", ErrMissingCredentials
	}

	const prefix = "Basic "
	if !strings.HasPrefix(strings.ToLower(authHeader), strings.ToLower(prefix)) {
		return "", "", ErrMissingCredentials
	}

	encoded := strings.TrimSpace(authHeader[len(prefix):])
	decodedBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}
	decoded := string(decodedBytes)
	separator := strings.Index(decoded, ":")
	if separator < 0 {
		return "", "", ErrMissingCredentials
	}

	return decoded[:separator], decoded[separator+1:], nil
}
