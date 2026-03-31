package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gapura/backend-go/internal/cache"
	"gapura/backend-go/internal/config"
	"gapura/backend-go/internal/db"
	gatewayhttp "gapura/backend-go/internal/http"
	"gapura/backend-go/internal/logging"
	"gapura/backend-go/internal/pipeline"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logging.Configure(cfg.LogLevel, cfg.LogFormat)

	// Log startup configuration (secrets are masked).
	logging.Infow("gapura backend starting", map[string]any{
		"listen":              cfg.ListenAddr,
		"openai_base_url":     cfg.OpenAIBaseURL,
		"openai_key":          maskSecret(cfg.OpenAIAPIKey),
		"gemini_base_url":     cfg.GeminiBaseURL,
		"gemini_key":          maskSecret(cfg.GeminiAPIKey),
		"ollama_chat_url":     cfg.OllamaChatURL,
		"request_timeout":     cfg.RequestTimeout.String(),
		"cache_ttl":           cfg.CacheTTL.String(),
		"cors_origin":         cfg.CORSOrigin,
		"log_level":           cfg.LogLevel,
		"log_format":          cfg.LogFormat,
		"tls_cert_configured": cfg.TLSCertFile != "",
		"tls_key_configured":  cfg.TLSKeyFile != "",
	})

	repo, err := db.NewRepository(cfg.DatabaseDSN)
	if err != nil {
		logging.Errorf("connect database: %v", err)
		os.Exit(1)
	}
	defer repo.Close()
	logging.Infof("database connected successfully")

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
			logging.Infow("http server listening", map[string]any{"listen": cfg.ListenAddr, "tls_enabled": true})
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				logging.Errorf("tls server error: %v", err)
				os.Exit(1)
			}
		} else {
			logging.Infow("http server listening", map[string]any{"listen": cfg.ListenAddr, "tls_enabled": false})
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Errorf("server error: %v", err)
				os.Exit(1)
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logging.Infof("shutting down gracefully")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logging.Warnf("shutdown warning: %v", err)
	}
	logging.Infof("server stopped")
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
