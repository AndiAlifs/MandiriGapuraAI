package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gapura/backend-go/internal/cache"
	"gapura/backend-go/internal/config"
	"gapura/backend-go/internal/db"
	gatewayhttp "gapura/backend-go/internal/http"
	"gapura/backend-go/internal/pipeline"
)

func main() {
	// Configure log output with microsecond precision timestamps.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Log startup configuration (secrets are masked).
	log.Printf("=== GAPURA Go Backend Starting ===")
	log.Printf("config: listen=%s", cfg.ListenAddr)
	log.Printf("config: openai_base_url=%s  openai_key=%s", cfg.OpenAIBaseURL, maskSecret(cfg.OpenAIAPIKey))
	log.Printf("config: gemini_base_url=%s  gemini_key=%s", cfg.GeminiBaseURL, maskSecret(cfg.GeminiAPIKey))
	log.Printf("config: ollama_chat_url=%s", cfg.OllamaChatURL)
	log.Printf("config: request_timeout=%s  cache_ttl=%s", cfg.RequestTimeout, cfg.CacheTTL)
	log.Printf("config: cors_origin=%s", cfg.CORSOrigin)

	repo, err := db.NewRepository(cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer repo.Close()
	log.Printf("database: connected successfully")

	cacheStore := cache.NewMemoryCache(cfg.CacheTTL)
	service := pipeline.NewService(cfg, repo, cacheStore)
	handler := gatewayhttp.NewHandler(service, cfg.AuthRealm)
	routes := gatewayhttp.WithCORS(handler.Routes(), cfg.CORSOrigin)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           routes,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			log.Printf("GAPURA Go backend running on %s (TLS enabled)", cfg.ListenAddr)
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("tls server error: %v", err)
			}
		} else {
			log.Printf("GAPURA Go backend running on %s (TLS disabled)", cfg.ListenAddr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server error: %v", err)
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Printf("shutting down gracefully…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown warning: %v", err)
	}
	log.Printf("server stopped")
}

// maskSecret returns a masked version of a secret for safe logging.
func maskSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
