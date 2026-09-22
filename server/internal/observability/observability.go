package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	bytes    *prometheus.CounterVec
	inflight prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	requests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pixelgo",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests processed by the API.",
		},
		[]string{"method", "route", "status"},
	)
	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pixelgo",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency in seconds.",
			Buckets: []float64{
				0.005, 0.01, 0.025, 0.05, 0.1,
				0.25, 0.5, 1, 2.5, 5,
			},
		},
		[]string{"method", "route"},
	)
	bytes := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pixelgo",
			Subsystem: "http",
			Name:      "response_bytes_total",
			Help:      "Total HTTP response bytes written.",
		},
		[]string{"method", "route", "status"},
	)
	inflight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "pixelgo",
			Subsystem: "http",
			Name:      "in_flight_requests",
			Help:      "Current number of in-flight HTTP requests.",
		},
	)

	registry.MustRegister(
		requests,
		duration,
		bytes,
		inflight,
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	return &Metrics{
		registry: registry,
		requests: requests,
		duration: duration,
		bytes:    bytes,
		inflight: inflight,
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(
		m.registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
}

func Middleware(
	next http.Handler,
	logger *slog.Logger,
	metrics *Metrics,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := newRequestID()
		route := normalizedRoute(r.URL.Path)

		w.Header().Set("X-Request-ID", requestID)
		writer := &captureWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		if metrics != nil {
			metrics.inflight.Inc()
			defer metrics.inflight.Dec()
		}

		next.ServeHTTP(writer, r)

		status := writer.status
		duration := time.Since(started)

		if metrics != nil {
			statusLabel := strconv.Itoa(status)
			metrics.requests.WithLabelValues(
				r.Method,
				route,
				statusLabel,
			).Inc()
			metrics.duration.WithLabelValues(
				r.Method,
				route,
			).Observe(duration.Seconds())
			metrics.bytes.WithLabelValues(
				r.Method,
				route,
				statusLabel,
			).Add(float64(writer.bytes))
		}

		attrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"route", route,
			"status", status,
			"duration_ms", float64(duration.Microseconds()) / 1000,
			"response_bytes", writer.bytes,
		}
		if replayed := writer.Header().Get("Idempotency-Replayed"); replayed != "" {
			attrs = append(attrs, "idempotency_replayed", replayed == "true")
		}

		switch {
		case status >= 500:
			logger.ErrorContext(r.Context(), "http request", attrs...)
		case status >= 400:
			logger.WarnContext(r.Context(), "http request", attrs...)
		case route == "/health" || route == "/metrics":
			logger.DebugContext(r.Context(), "http request", attrs...)
		default:
			logger.InfoContext(r.Context(), "http request", attrs...)
		}
	})
}

func normalizedRoute(path string) string {
	switch path {
	case "/health", "/ready", "/metrics",
		"/v1/auth/register", "/v1/auth/login", "/v1/auth/refresh",
		"/v1/devices", "/v1/transfers", "/v1/events":
		return path
	}

	switch {
	case strings.HasPrefix(path, "/v1/devices/"):
		return "/v1/devices/{deviceId}"
	case strings.HasPrefix(path, "/v1/presence/"):
		return "/v1/presence/{deviceId}"
	case strings.HasPrefix(path, "/v1/transfers/"):
		switch {
		case strings.HasSuffix(path, "/uploaded"):
			return "/v1/transfers/{transferId}/uploaded"
		case strings.HasSuffix(path, "/complete"):
			return "/v1/transfers/{transferId}/complete"
		default:
			return "/v1/transfers/{transferId}"
		}
	case strings.HasPrefix(path, "/dev-upload/"):
		return "/dev-upload/{transferId}"
	case strings.HasPrefix(path, "/dev-download/"):
		return "/dev-download/{transferId}"
	default:
		return "unmatched"
	}
}

func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

type captureWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *captureWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *captureWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *captureWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *captureWriter) Hijack() (
	net.Conn,
	*bufio.ReadWriter,
	error,
) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *captureWriter) ReadFrom(r io.Reader) (int64, error) {
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		if !w.wroteHeader {
			w.WriteHeader(http.StatusOK)
		}
		n, err := readerFrom.ReadFrom(r)
		w.bytes += int(n)
		return n, err
	}
	return io.Copy(struct{ io.Writer }{w}, r)
}

func (w *captureWriter) Push(
	target string,
	opts *http.PushOptions,
) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}
