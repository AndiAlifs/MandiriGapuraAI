package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"gapura/backend-go/internal/auth"
	"gapura/backend-go/internal/openai"
	"gapura/backend-go/internal/pipeline"
)

// contextKey is an unexported type used for context keys in this package.
type contextKey string

const requestIDKey contextKey = "requestID"

// RequestIDFromContext extracts the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return "unknown"
}

type Handler struct {
	service *pipeline.Service
}

func NewHandler(service *pipeline.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/v1/chat/completions", h.chatCompletions)
	return loggingMiddleware(mux)
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	reqID := RequestIDFromContext(r.Context())

	if r.Method != http.MethodPost {
		log.Printf("[%s] rejected: method %s not allowed", reqID, r.Method)
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, password, err := auth.ParseCredentials(r)
	if err != nil {
		log.Printf("[%s] auth: credential parse failed: %v", reqID, err)
		w.Header().Set("WWW-Authenticate", "Basic realm=\"gapura\"")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing credentials"})
		return
	}

	log.Printf("[%s] auth: attempting login for user=%q", reqID, username)
	app, err := h.service.AuthenticateAndCheckQuota(r.Context(), username, password)
	if err != nil {
		log.Printf("[%s] auth: database error for user=%q: %v", reqID, username, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if app == nil {
		log.Printf("[%s] auth: invalid credentials for user=%q", reqID, username)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if app.AppID == -1 {
		log.Printf("[%s] auth: quota exceeded for user=%q project=%q", reqID, username, app.ProjectName)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "daily token quota exceeded"})
		return
	}
	log.Printf("[%s] auth: success user=%q app_id=%d project=%q", reqID, username, app.AppID, app.ProjectName)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[%s] request: failed to read body: %v", reqID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	log.Printf("[%s] request: body size=%d bytes", reqID, len(rawBody))

	req, err := openai.DecodeRequest(rawBody)
	if err != nil {
		log.Printf("[%s] request: invalid JSON payload: %v", reqID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
		return
	}
	if req.Model == "" || len(req.Messages) == 0 {
		log.Printf("[%s] request: validation failed model=%q messages_count=%d", reqID, req.Model, len(req.Messages))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model and messages are required"})
		return
	}
	log.Printf("[%s] request: model=%q messages=%d stream=%v", reqID, req.Model, len(req.Messages), req.Stream)

	hash := h.service.HashRequest(req)
	result, err := h.service.Process(r.Context(), pipeline.ProcessInput{
		App:         *app,
		RawBody:     rawBody,
		Request:     req,
		RequestHash: hash,
		RequestID:   reqID,
	})
	if err != nil {
		status := http.StatusBadGateway
		if errorsIsContextCanceled(err) {
			status = http.StatusGatewayTimeout
			log.Printf("[%s] process: request timed out / cancelled: %v", reqID, err)
		} else {
			log.Printf("[%s] process: upstream failure: %v", reqID, err)
		}
		writeJSON(w, status, map[string]string{"error": "upstream provider failure"})
		return
	}

	if result.FromCache {
		log.Printf("[%s] response: status=%d from_cache=true", reqID, result.StatusCode)
	} else {
		log.Printf("[%s] response: status=%d body_size=%d", reqID, result.StatusCode, len(result.Body))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate a short unique request ID.
		reqID := fmt.Sprintf("%04x%04x", time.Now().UnixMilli()&0xFFFF, rand.Intn(0xFFFF))

		// Inject request ID into context.
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		r = r.WithContext(ctx)

		log.Printf("[%s] --> %s %s from %s", reqID, r.Method, r.URL.Path, r.RemoteAddr)

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		log.Printf("[%s] <-- %d %s (%dms)", reqID, rw.statusCode, http.StatusText(rw.statusCode), time.Since(start).Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorsIsContextCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
