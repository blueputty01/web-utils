package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type ctxKey string

const CorrelationIDKey ctxKey = "correlation_id"

func CorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(CorrelationIDKey).(string); ok && v != "" {
		return v
	}
	return ""
}

type Metrics struct {
	RequestsTotal       atomic.Int64
	RequestsRateLimited atomic.Int64
	JobsSubmitted       atomic.Int64
	JobsCompleted       atomic.Int64
	JobsFailed          atomic.Int64
	JobsCancelled       atomic.Int64
	JobsRetried         atomic.Int64
	FormattedPreviews   atomic.Int64
	ArtifactsCleaned    atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]int64{
				"http_requests_total":         m.RequestsTotal.Load(),
				"http_requests_rate_limited":  m.RequestsRateLimited.Load(),
				"jobs_submitted_total":        m.JobsSubmitted.Load(),
				"jobs_completed_total":        m.JobsCompleted.Load(),
				"jobs_failed_total":           m.JobsFailed.Load(),
				"jobs_cancelled_total":        m.JobsCancelled.Load(),
				"jobs_retried_total":          m.JobsRetried.Load(),
				"formatted_previews_total":    m.FormattedPreviews.Load(),
				"artifacts_cleaned_total":     m.ArtifactsCleaned.Load(),
			})
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "webutils_http_requests_total %d\n", m.RequestsTotal.Load())
		fmt.Fprintf(w, "webutils_http_requests_rate_limited_total %d\n", m.RequestsRateLimited.Load())
		fmt.Fprintf(w, "webutils_jobs_submitted_total %d\n", m.JobsSubmitted.Load())
		fmt.Fprintf(w, "webutils_jobs_completed_total %d\n", m.JobsCompleted.Load())
		fmt.Fprintf(w, "webutils_jobs_failed_total %d\n", m.JobsFailed.Load())
		fmt.Fprintf(w, "webutils_jobs_cancelled_total %d\n", m.JobsCancelled.Load())
		fmt.Fprintf(w, "webutils_jobs_retried_total %d\n", m.JobsRetried.Load())
		fmt.Fprintf(w, "webutils_formatted_previews_total %d\n", m.FormattedPreviews.Load())
		fmt.Fprintf(w, "webutils_artifacts_cleaned_total %d\n", m.ArtifactsCleaned.Load())
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func WithTracingAndMetrics(metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if metrics != nil {
			metrics.RequestsTotal.Add(1)
		}
		corrID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
		if corrID == "" {
			corrID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
		}
		if corrID == "" {
			corrID = uuid.NewString()
		}
		w.Header().Set("X-Correlation-ID", corrID)
		ctx := context.WithValue(r.Context(), CorrelationIDKey, corrID)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r.WithContext(ctx))
		slog.Debug("http_request",
			"correlation_id", corrID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]*clientBucket
	metrics *Metrics
}

type clientBucket struct {
	count   int
	resetAt time.Time
}

func NewRateLimiter(limit int, window time.Duration, metrics *Metrics) *RateLimiter {
	if limit <= 0 {
		limit = 120
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		limit:   limit,
		window:  window,
		clients: make(map[string]*clientBucket),
		metrics: metrics,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		now := time.Now()
		rl.mu.Lock()
		bucket, ok := rl.clients[ip]
		if !ok || now.After(bucket.resetAt) {
			bucket = &clientBucket{count: 0, resetAt: now.Add(rl.window)}
			rl.clients[ip] = bucket
		}
		bucket.count++
		exceeded := bucket.count > rl.limit
		rl.mu.Unlock()

		if exceeded {
			if rl.metrics != nil {
				rl.metrics.RequestsRateLimited.Add(1)
			}
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "rate_limit_exceeded",
				"message": "Too many requests. Please wait before submitting again.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
