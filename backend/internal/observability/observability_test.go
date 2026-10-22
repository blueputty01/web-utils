package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTracingMetricsAndRateLimiter(t *testing.T) {
	metrics := NewMetrics()
	limiter := NewRateLimiter(2, time.Minute, metrics)

	handler := WithTracingAndMetrics(metrics, limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if CorrelationID(r.Context()) == "" {
			t.Fatal("missing correlation ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transcription/jobs", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
		if rec.Header().Get("X-Correlation-ID") == "" {
			t.Fatal("missing X-Correlation-ID header")
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcription/jobs", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on third request, got %d", rec.Code)
	}

	mReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	mRec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(mRec, mReq)
	if !strings.Contains(mRec.Body.String(), "webutils_http_requests_rate_limited_total 1") {
		t.Fatalf("unexpected metrics body: %s", mRec.Body.String())
	}
}
