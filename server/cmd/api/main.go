package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
	"github.com/armaabetancourtt/pixelgo/server/internal/devices"
	"github.com/armaabetancourtt/pixelgo/server/internal/files"
	"github.com/armaabetancourtt/pixelgo/server/internal/httpapi"
	"github.com/armaabetancourtt/pixelgo/server/internal/platform/postgresdb"
	"github.com/armaabetancourtt/pixelgo/server/internal/platform/redisdb"
	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/armaabetancourtt/pixelgo/server/internal/ratelimit"
	"github.com/armaabetancourtt/pixelgo/server/internal/realtime"
	"github.com/armaabetancourtt/pixelgo/server/internal/transfers"
)

func main() {
	addr := env("PIXELGO_ADDR", ":8080")
	baseURL := env("PIXELGO_PUBLIC_BASE_URL", "http://localhost:8080")
	signingSecret := env("PIXELGO_SIGNING_SECRET", "pixelgo-local-signing-secret-change-me")
	jwtSecret := env("PIXELGO_JWT_SECRET", "pixelgo-local-jwt-secret-change-me-32")
	requireAuth := envBool("PIXELGO_REQUIRE_AUTH", false)
	if len(jwtSecret) < 32 {
		log.Fatal("PIXELGO_JWT_SECRET must be at least 32 bytes")
	}

	var authRepo auth.Repository = auth.NewMemoryRepository()
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

		authRepo = auth.NewPostgresRepository(pool)
		deviceRepo = devices.NewPostgresRepository(pool)
		transferRepo = transfers.NewPostgresRepository(pool)
		log.Printf("pixelgo persistence: postgres")
	} else {
		log.Printf("pixelgo persistence: in-memory")
	}

	var presenceStore presence.Store = presence.NewMemoryStore()
	var requestLimiter ratelimit.Limiter = ratelimit.NewMemoryLimiter()
	var broker realtime.Broker
	httpOptions := make([]httpapi.Option, 0, 4)

	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		redisClient, err := redisdb.Open(ctx, redisURL)
		if err != nil {
			log.Fatal(err)
		}
		defer redisClient.Close()

		presenceStore = presence.NewRedisStore(redisClient)
		requestLimiter = ratelimit.NewRedisLimiter(redisClient)
		broker = realtime.NewRedisBroker(redisClient)
		httpOptions = append(httpOptions, httpapi.WithRedisIdempotency(redisClient))
		log.Printf("pixelgo ephemeral state: redis")
	} else {
		log.Printf("pixelgo ephemeral state: in-memory")
	}

	hubOptions := []realtime.Option{
		realtime.WithPresence(presenceStore),
	}
	if broker != nil {
		hubOptions = append(hubOptions, realtime.WithBroker(broker))
	}

	hub := realtime.NewHub(hubOptions...)
	hub.Start(context.Background())

	var fileService *files.Service
	if storageEndpoint := os.Getenv("OBJECT_STORAGE_ENDPOINT"); storageEndpoint != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		storage, err := files.NewS3Service(ctx, files.S3Config{
			Endpoint:   storageEndpoint,
			Bucket:     env("OBJECT_STORAGE_BUCKET", "pixelgo"),
			AccessKey:  os.Getenv("OBJECT_STORAGE_ACCESS_KEY"),
			SecretKey:  os.Getenv("OBJECT_STORAGE_SECRET_KEY"),
			Region:     env("OBJECT_STORAGE_REGION", "us-east-1"),
			Prefix:     env("OBJECT_STORAGE_PREFIX", "transfers"),
			TTL:        10 * time.Minute,
			AutoCreate: envBool("OBJECT_STORAGE_AUTO_CREATE", false),
		})
		if err != nil {
			log.Fatal(err)
		}
		fileService = storage
		log.Printf("pixelgo payload storage: s3-compatible")
	} else {
		fileService = files.NewService(baseURL, signingSecret, 10*time.Minute)
		log.Printf("pixelgo payload storage: in-memory development adapter")
	}

	authService := auth.NewService(authRepo, []byte(jwtSecret))
	deviceService := devices.NewService(deviceRepo)
	transferService := transfers.NewService(transferRepo, hub, fileService)
	httpOptions = append(
		httpOptions,
		httpapi.WithPresence(presenceStore),
		httpapi.WithRateLimiter(requestLimiter),
		httpapi.WithAuth(authService, requireAuth),
	)
	handler := httpapi.New(
		deviceService,
		transferService,
		hub,
		fileService,
		httpOptions...,
	)

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
