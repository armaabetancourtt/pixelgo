package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/devices"
	"github.com/armaabetancourtt/pixelgo/server/internal/files"
	"github.com/armaabetancourtt/pixelgo/server/internal/httpapi"
	"github.com/armaabetancourtt/pixelgo/server/internal/platform/postgresdb"
	"github.com/armaabetancourtt/pixelgo/server/internal/realtime"
	"github.com/armaabetancourtt/pixelgo/server/internal/transfers"
)

func main() {
	addr := env("PIXELGO_ADDR", ":8080")
	baseURL := env("PIXELGO_PUBLIC_BASE_URL", "http://localhost:8080")
	signingSecret := env("PIXELGO_SIGNING_SECRET", "pixelgo-local-signing-secret-change-me")

	var deviceRepo devices.Repository = devices.NewMemoryRepository()
	var transferRepo transfers.Repository = transfers.NewMemoryRepository()

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pool, err := postgresdb.Open(ctx, databaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer pool.Close()

		if envBool("PIXELGO_AUTO_MIGRATE", true) {
			if err := postgresdb.Migrate(ctx, pool); err != nil {
				log.Fatal(err)
			}
		}

		deviceRepo = devices.NewPostgresRepository(pool)
		transferRepo = transfers.NewPostgresRepository(pool)
		log.Printf("pixelgo persistence: postgres")
	} else {
		log.Printf("pixelgo persistence: in-memory")
	}

	hub := realtime.NewHub()
	fileService := files.NewService(baseURL, signingSecret, 10*time.Minute)
	deviceService := devices.NewService(deviceRepo)
	transferService := transfers.NewService(transferRepo, hub, fileService)
	handler := httpapi.New(deviceService, transferService, hub, fileService)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("pixelgo api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
