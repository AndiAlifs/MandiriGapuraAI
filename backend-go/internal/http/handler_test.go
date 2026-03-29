package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSPreflightAllowedOrigin(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	h := WithCORS(next, "http://localhost:4200")

	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "http://localhost:4200")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if nextCalled {
		t.Fatalf("expected middleware to short-circuit preflight")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:4200" {
		t.Fatalf("expected Access-Control-Allow-Origin to be request origin, got %q", got)
	}
}

func TestWithCORSPreflightDisallowedOrigin(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	h := WithCORS(next, "http://localhost:4200")

	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "http://evil.local")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if nextCalled {
		t.Fatalf("expected middleware to reject preflight without calling next")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

func TestWithCORSWildcardOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := WithCORS(next, "*")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://any-origin.local")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard allow origin, got %q", got)
	}
}

func TestChatCompletionsUsesConfiguredRealm(t *testing.T) {
	h := NewHandler(nil, "myrealm")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()

	h.chatCompletions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if got := rr.Header().Get("WWW-Authenticate"); got != "Basic realm=\"myrealm\"" {
		t.Fatalf("unexpected WWW-Authenticate header: %q", got)
	}
}
