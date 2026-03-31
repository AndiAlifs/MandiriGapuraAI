package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
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

func TestStudioCreateAppAuthMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil, "gapura")

	req := httptest.NewRequest(http.MethodGet, "/v1/studio/apps-auth", nil)
	rr := httptest.NewRecorder()

	h.studioCreateAppAuth(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
	if got := rr.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("expected Allow header %q, got %q", http.MethodPost, got)
	}
}

func TestStudioCreateAppAuthInvalidPayload(t *testing.T) {
	h := NewHandler(nil, "gapura")

	req := httptest.NewRequest(http.MethodPost, "/v1/studio/apps-auth", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	h.studioCreateAppAuth(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestStudioCreateAppAuthValidation(t *testing.T) {
	h := NewHandler(nil, "gapura")

	body := `{"projectName":"","username":"user-a","password":"pass-a","dailyTokenLimit":0}`
	req := httptest.NewRequest(http.MethodPost, "/v1/studio/apps-auth", strings.NewReader(body))
	rr := httptest.NewRecorder()

	h.studioCreateAppAuth(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestLoggingMiddlewarePropagatesRequestID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	h := loggingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
	if got := rr.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Fatalf("expected response request ID abc-123, got %q", got)
	}
}

func TestLoggingMiddlewareGeneratesRequestID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := loggingMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	rid := rr.Header().Get("X-Request-ID")
	if rid == "" {
		t.Fatalf("expected generated request ID to be present")
	}
	isSafe := regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString
	if !isSafe(rid) {
		t.Fatalf("expected generated request ID to contain only safe chars, got %q", rid)
	}
}

func TestRecoveryMiddlewareRecoversPanic(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	h := loggingMiddleware(recoveryMiddleware(next))
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set("X-Request-ID", "panic-req-1")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
	if got := rr.Header().Get("X-Request-ID"); got != "panic-req-1" {
		t.Fatalf("expected panic response request ID panic-req-1, got %q", got)
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON response body, got unmarshal error: %v", err)
	}
	if payload["error"] != "internal server error" {
		t.Fatalf("expected internal server error message, got %q", payload["error"])
	}
	if payload["request_id"] != "panic-req-1" {
		t.Fatalf("expected request_id panic-req-1, got %q", payload["request_id"])
	}
}
