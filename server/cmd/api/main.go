package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/devices"
	"github.com/armaabetancourtt/pixelgo/server/internal/httpapi"
	"github.com/armaabetancourtt/pixelgo/server/internal/realtime"
	"github.com/armaabetancourtt/pixelgo/server/internal/transfers"
)

func main() {
	addr := env("PIXELGO_ADDR", ":8080")
	baseURL := env("PIXELGO_PUBLIC_BASE_URL", "http://localhost:8080")

	hub := realtime.NewHub()
	deviceService := devices.NewService()
	transferService := transfers.NewService(transfers.NewMemoryRepository(), hub, baseURL)
	handler := httpapi.New(deviceService, transferService, hub)

	server := &http.Server{
		Addr: addr,
		Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	log.Printf("pixelgo api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" { return value }
	return fallback
}
