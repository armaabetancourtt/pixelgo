package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
	"github.com/armaabetancourtt/pixelgo/server/internal/devices"
	"github.com/armaabetancourtt/pixelgo/server/internal/files"
	"github.com/armaabetancourtt/pixelgo/server/internal/httpapi"
	"github.com/armaabetancourtt/pixelgo/server/internal/observability"
	"github.com/armaabetancourtt/pixelgo/server/internal/platform/postgresdb"
	"github.com/armaabetancourtt/pixelgo/server/internal/platform/redisdb"
	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/armaabetancourtt/pixelgo/server/internal/ratelimit"
	"github.com/armaabetancourtt/pixelgo/server/internal/realtime"
	"github.com/armaabetancourtt/pixelgo/server/internal/transfers"
)

func main() {
	logger := newLogger()
	slog.SetDefault(logger)
	metrics := observability.NewMetrics()

	addr := env("PIXELGO_ADDR", ":8080")
	baseURL := env("PIXELGO_PUBLIC_BASE_URL", "http://localhost:8080")
	signingSecret := env("PIXELGO_SIGNING_SECRET", "pixelgo-local-signing-secret-change-me")
	jwtSecret := env("PIXELGO_JWT_SECRET", "pixelgo-local-jwt-secret-change-me-32")
	requireAuth := envBool("PIXELGO_REQUIRE_AUTH", false)
	if len(jwtSecret) < 32 {
		fatal(logger, "invalid configuration", "error", "PIXELGO_JWT_SECRET must be at least 32 bytes")
	}

	var authRepo auth.Repository = auth.NewMemoryRepository()
	var deviceRepo devices.Repository = devices.NewMemoryRepository()
	var transferRepo transfers.Repository = transfers.NewMemoryRepository()
	readinessChecks := make([]observability.Check, 0, 3)

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pool, err := postgresdb.Open(ctx, databaseURL)
		if err != nil {
			fatal(logger, "startup failed", "error", err)
		}
		defer pool.Close()

		if envBool("PIXELGO_AUTO_MIGRATE", true) {
			if err := postgresdb.Migrate(ctx, pool); err != nil {
				fatal(logger, "startup failed", "error", err)
			}
		}

		authRepo = auth.NewPostgresRepository(pool)
		deviceRepo = devices.NewPostgresRepository(pool)
		transferRepo = transfers.NewPostgresRepository(pool)
		readinessChecks = append(
			readinessChecks,
			observability.Check{Name: "postgres", Run: pool.Ping},
		)
		logger.Info("persistence configured", "adapter", "postgres")
	} else {
		logger.Info("persistence configured", "adapter", "in-memory")
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
			fatal(logger, "startup failed", "error", err)
		}
		defer redisClient.Close()

		presenceStore = presence.NewRedisStore(redisClient)
		requestLimiter = ratelimit.NewRedisLimiter(redisClient)
		broker = realtime.NewRedisBroker(redisClient)
		readinessChecks = append(
			readinessChecks,
			observability.Check{
				Name: "redis",
				Run: func(ctx context.Context) error {
					return redisClient.Ping(ctx).Err()
				},
			},
		)
		httpOptions = append(httpOptions, httpapi.WithRedisIdempotency(redisClient))
		logger.Info("ephemeral state configured", "adapter", "redis")
	} else {
		logger.Info("ephemeral state configured", "adapter", "in-memory")
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
			fatal(logger, "startup failed", "error", err)
		}
		fileService = storage
		readinessChecks = append(
			readinessChecks,
			observability.Check{
				Name: "object_storage",
				Run:  fileService.Health,
			},
		)
		logger.Info("payload storage configured", "adapter", "s3-compatible")
	} else {
		fileService = files.NewService(baseURL, signingSecret, 10*time.Minute)
		logger.Info("payload storage configured", "adapter", "in-memory-development")
	}

	readinessHandler := observability.ReadinessHandler(
		readinessChecks,
		2*time.Second,
	)

	authService := auth.NewService(authRepo, []byte(jwtSecret))
	deviceService := devices.NewService(deviceRepo)
	transferService := transfers.NewService(transferRepo, hub, fileService)
	httpOptions = append(
		httpOptions,
		httpapi.WithPresence(presenceStore),
		httpapi.WithRateLimiter(requestLimiter),
		httpapi.WithAuth(authService, requireAuth),
		httpapi.WithMetrics(metrics.Handler()),
		httpapi.WithReadiness(readinessHandler),
	)
	handler := observability.Middleware(
		httpapi.New(
			deviceService,
			transferService,
			hub,
			fileService,
			httpOptions...,
		),
		logger,
		metrics,
	)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info(
		"pixelgo api listening",
		"addr", addr,
		"auth_required", requireAuth,
		"readiness_checks", len(readinessChecks),
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(logger, "api server stopped unexpectedly", "error", err)
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


func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(env("PIXELGO_LOG_LEVEL", "info")) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	return slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: level},
		),
	)
}

func fatal(logger *slog.Logger, message string, args ...any) {
	logger.Error(message, args...)
	os.Exit(1)
}
