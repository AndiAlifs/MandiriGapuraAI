package http

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"gapura/backend-go/internal/auth"
	"gapura/backend-go/internal/openai"
	"gapura/backend-go/internal/pipeline"
)

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
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, password, err := auth.ParseCredentials(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"gapura\"")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing credentials"})
		return
	}

	app, err := h.service.AuthenticateAndCheckQuota(r.Context(), username, password)
	if err != nil {
		log.Printf("auth error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if app == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if app.AppID == -1 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "daily token quota exceeded"})
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	req, err := openai.DecodeRequest(rawBody)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
		return
	}
	if req.Model == "" || len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model and messages are required"})
		return
	}

	hash := h.service.HashRequest(req)
	result, err := h.service.Process(r.Context(), pipeline.ProcessInput{
		App:         *app,
		RawBody:     rawBody,
		Request:     req,
		RequestHash: hash,
	})
	if err != nil {
		status := http.StatusBadGateway
		if errorsIsContextCanceled(err) {
			status = http.StatusGatewayTimeout
		}
		writeJSON(w, status, map[string]string{"error": "upstream provider failure"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
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
