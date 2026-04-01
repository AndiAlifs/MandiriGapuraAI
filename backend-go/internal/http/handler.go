package http

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"gapura/backend-go/internal/auth"
	"gapura/backend-go/internal/db"
	"gapura/backend-go/internal/logging"
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
	service   *pipeline.Service
	authRealm string
}

func NewHandler(service *pipeline.Service, authRealm string) *Handler {
	if strings.TrimSpace(authRealm) == "" {
		authRealm = "gapura"
	}

	return &Handler{service: service, authRealm: authRealm}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/v1/chat/completions", h.chatCompletions)
	mux.HandleFunc("/v1/studio/api-keys", h.studioCreateAPIKey)
	mux.HandleFunc("/v1/studio/scorecards", h.studioScorecards)
	mux.HandleFunc("/v1/studio/audit-logs", h.studioAuditLogs)
	mux.HandleFunc("/v1/studio/models", h.studioModels)
	return loggingMiddleware(recoveryMiddleware(mux))
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

	token, err := auth.ExtractBearerToken(r)
	if err != nil {
		log.Printf("[%s] auth: credential parse failed: %v", reqID, err)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer realm=%q", h.authRealm))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing credentials"})
		return
	}

	log.Printf("[%s] auth: attempting login with token", reqID)
	apiKey, err := h.service.AuthenticateAPIKey(r.Context(), token)
	if err != nil {
		log.Printf("[%s] auth: database error: %v", reqID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if apiKey == nil {
		log.Printf("[%s] auth: invalid credentials", reqID)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	log.Printf("[%s] auth: success api_key_id=%s project_id=%s", reqID, apiKey.ID, apiKey.ProjectID)

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

	result, err := h.service.Process(r.Context(), pipeline.ProcessInput{
		APIKey:    *apiKey,
		RawBody:   rawBody,
		Request:   req,
		RequestID: reqID,
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

func (h *Handler) studioScorecards(w http.ResponseWriter, r *http.Request) {
	reqID := RequestIDFromContext(r.Context())
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	res, err := h.service.StudioScorecards(r.Context())
	if err != nil {
		log.Printf("[%s] studio scorecards failed: %v", reqID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load scorecards"})
		return
	}

	writeJSON(w, http.StatusOK, res)
}

type createAPIKeyRequest struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

func (h *Handler) studioCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	reqID := RequestIDFromContext(r.Context())
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[%s] studio create api key invalid payload: %v", reqID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	if req.ProjectID == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "projectId and name are required",
		})
		return
	}

	plainKey := auth.GeneratePlainAPIKey()
	hash := auth.HashAPIKey(plainKey)

	// User requested to pass the hash and request details to h.service.CreateAPIKey
	created, err := h.service.CreateAPIKey(r.Context(), db.APIKey{
		ProjectID: req.ProjectID,
		Name:      req.Name,
		KeyHash:   hash,
		IsActive:  true,
	})

	if err != nil {
		log.Printf("[%s] studio create api key failed: %v", reqID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create api key"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        created.ID,
		"projectId": created.ProjectID,
		"name":      created.Name,
		"plainKey":  plainKey, // CRITICAL: Only shown once
	})
}

// StudioCreateAppAuthAction is kept as a compatibility shim for older tests/callers.
// The current implementation provisions API keys via the studioCreateAPIKey handler.
func (h *Handler) StudioCreateAppAuthAction(w http.ResponseWriter, r *http.Request) {
	h.studioCreateAPIKey(w, r)
}

// studioCreateAppAuth is kept as a compatibility shim for older tests/callers.
func (h *Handler) studioCreateAppAuth(w http.ResponseWriter, r *http.Request) {
	h.studioCreateAPIKey(w, r)
}

func (h *Handler) studioAuditLogs(w http.ResponseWriter, r *http.Request) {
	reqID := RequestIDFromContext(r.Context())
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modelID, hasModelID := intQueryOptional(r, "modelId")
	if !hasModelID {
		modelID, hasModelID = intQueryOptional(r, "model_id")
	}

	filter := db.AuditLogFilter{
		ProjectName: strings.TrimSpace(r.URL.Query().Get("project")),
		ModelUsed:   strings.TrimSpace(r.URL.Query().Get("model")),
		Limit:       intQuery(r, "limit", 50),
		Offset:      intQuery(r, "offset", 0),
	}
	if hasModelID {
		filter.ModelID = &modelID
	}

	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	res, err := h.service.StudioAuditLogs(r.Context(), filter)
	if err != nil {
		log.Printf("[%s] studio audit logs failed: %v", reqID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load audit logs"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  res,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *Handler) studioModels(w http.ResponseWriter, r *http.Request) {
	reqID := RequestIDFromContext(r.Context())
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	res, err := h.service.StudioModels(r.Context())
	if err != nil {
		log.Printf("[%s] studio models failed: %v", reqID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load models"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": res})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(data)
	rw.bytesWritten += n
	return n, err
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := RequestIDFromContext(r.Context())
				logging.Errorw("http.request.panic", map[string]any{
					"request_id": reqID,
					"method":     r.Method,
					"path":       r.URL.Path,
					"panic":      fmt.Sprintf("%v", rec),
					"stack":      string(debug.Stack()),
				})
				w.Header().Set("X-Request-ID", reqID)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error":      "internal server error",
					"request_id": reqID,
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		reqID := requestIDFromHeader(r)

		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", reqID)

		logging.Infow("http.request.started", map[string]any{
			"request_id": reqID,
			"method":     r.Method,
			"path":       r.URL.Path,
			"client_ip":  extractClientIP(r),
			"user_agent": r.UserAgent(),
		})

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK, bytesWritten: 0}
		next.ServeHTTP(rw, r)

		logging.Infow("http.request.completed", map[string]any{
			"request_id":    reqID,
			"method":        r.Method,
			"path":          r.URL.Path,
			"status":        rw.statusCode,
			"status_text":   http.StatusText(rw.statusCode),
			"duration_ms":   time.Since(start).Milliseconds(),
			"bytes_written": rw.bytesWritten,
		})
	})
}

func requestIDFromHeader(r *http.Request) string {
	provided := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if provided != "" && isSafeRequestID(provided) {
		if len(provided) > 64 {
			return provided[:64]
		}
		return provided
	}

	buf := make([]byte, 8)
	if _, err := cryptorand.Read(buf); err == nil {
		return fmt.Sprintf("%x", buf)
	}

	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func isSafeRequestID(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func extractClientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}

type corsPolicy struct {
	allowAll bool
	origins  map[string]struct{}
}

// WithCORS adds origin-based CORS handling, including OPTIONS preflight responses.
func WithCORS(next http.Handler, allowedOrigins string) http.Handler {
	policy := parseCORSOrigins(allowedOrigins)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		isPreflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

		if isPreflight {
			if origin != "" && !policy.isAllowed(origin) {
				http.Error(w, "cors origin not allowed", http.StatusForbidden)
				return
			}
			setCORSHeaders(w, origin, policy)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if origin != "" && policy.isAllowed(origin) {
			setCORSHeaders(w, origin, policy)
		}

		next.ServeHTTP(w, r)
	})
}

func parseCORSOrigins(raw string) corsPolicy {
	policy := corsPolicy{origins: make(map[string]struct{})}
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		if origin == "" {
			continue
		}
		if origin == "*" {
			policy.allowAll = true
			continue
		}
		policy.origins[origin] = struct{}{}
	}

	return policy
}

func (p corsPolicy) isAllowed(origin string) bool {
	if p.allowAll {
		return true
	}
	_, ok := p.origins[origin]
	return ok
}

func setCORSHeaders(w http.ResponseWriter, origin string, policy corsPolicy) {
	if policy.allowAll {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Username, X-Password")
	w.Header().Set("Access-Control-Max-Age", "600")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorsIsContextCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func intQuery(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func intQueryOptional(r *http.Request, key string) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return v, true
}
