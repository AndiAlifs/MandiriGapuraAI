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
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	repo, err := db.NewRepository(cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer repo.Close()

	cacheStore := cache.NewMemoryCache(cfg.CacheTTL)
	service := pipeline.NewService(cfg, repo, cacheStore)
	handler := gatewayhttp.NewHandler(service)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("GAPURA Go backend running on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown warning: %v", err)
	}
}
