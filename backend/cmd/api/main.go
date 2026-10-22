package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/web-utils/backend/internal/config"
	"github.com/example/web-utils/backend/internal/format"
	"github.com/example/web-utils/backend/internal/media"
	"github.com/example/web-utils/backend/internal/observability"
	"github.com/example/web-utils/backend/internal/storage"
	"github.com/example/web-utils/backend/internal/transcription"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type createRequest struct {
	URL            string `json:"url"`
	SourceURL      string `json:"source_url"`
	IdempotencyKey string `json:"idempotency_key"`
}

type formatRequest struct {
	Preset string `json:"preset"`
}

func main() {
	cfg := config.Load()
	config.ConfigureLogger(cfg.LogLevel)
	if err := cfg.ValidateAPI(); err != nil {
		slog.Error("invalid API configuration", "error", err)
		return
	}
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("database configuration", "error", err)
		return
	}
	defer pool.Close()

	var artifactStore storage.Store
	if cfg.ObjectStoreEndpoint != "" {
		artifactStore = storage.NewS3Store(cfg.ObjectStoreEndpoint, cfg.ObjectStoreBucket, cfg.ObjectStoreAccessKey, cfg.ObjectStoreSecretKey, cfg.ArtifactTTL)
	} else {
		artifactStore = storage.NewMemoryStore(cfg.ArtifactTTL)
	}

	metrics := observability.NewMetrics()
	limiter := observability.NewRateLimiter(cfg.RateLimitPerMinute, time.Minute, metrics)
	broker := transcription.NewEventBroker(32)
	store := transcription.Store{DB: pool, Events: broker}

	mux := newAPIMux(cfg, pool, store, artifactStore, broker, metrics)
	handler := withCORS(cfg.CORSOrigin, observability.WithTracingAndMetrics(metrics, limiter.Middleware(mux)))

	server := http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("api listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
	}
}

func newAPIMux(
	cfg config.Config,
	pool *pgxpool.Pool,
	store transcription.Store,
	artifactStore storage.Store,
	broker *transcription.EventBroker,
	metrics *observability.Metrics,
) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if pool != nil {
			if err := pool.Ping(r.Context()); err != nil {
				respondError(w, http.StatusServiceUnavailable, "database_unavailable", "Database unavailable.")
				return
			}
		}
		respond(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if pool != nil {
			if err := pool.Ping(r.Context()); err != nil {
				respondError(w, http.StatusServiceUnavailable, "database_unavailable", "Database unavailable.")
				return
			}
		}
		if artifactStore != nil {
			if err := artifactStore.Healthy(r.Context()); err != nil {
				respondError(w, http.StatusServiceUnavailable, "object_store_unavailable", "Object storage unavailable.")
				return
			}
		}
		respond(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /api/v1/utilities", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, http.StatusOK, map[string]any{"utilities": config.Utilities()})
	})

	handleSubmit := func(w http.ResponseWriter, r *http.Request) {
		var input createRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			respondError(w, http.StatusBadRequest, "invalid_request", "Valid JSON request body is required.")
			return
		}
		rawURL := strings.TrimSpace(input.SourceURL)
		if rawURL == "" {
			rawURL = strings.TrimSpace(input.URL)
		}
		if err := media.Validate(rawURL); err != nil {
			respondError(w, http.StatusBadRequest, "invalid_url", "Enter a valid public http or https video URL.")
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			key = strings.TrimSpace(input.IdempotencyKey)
		}
		if key == "" {
			key = uuid.NewString()
		}
		job, err := store.Create(r.Context(), rawURL, key)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "create_failed", "Could not create transcription job.")
			return
		}
		metrics.JobsSubmitted.Add(1)
		respond(w, http.StatusAccepted, transcription.BuildJobResponse(job, nil, nil))
	}
	mux.HandleFunc("POST /api/v1/transcription/jobs", handleSubmit)
	mux.HandleFunc("POST /api/transcriptions", handleSubmit)

	handleGet := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		job, segments, err := store.Get(r.Context(), id)
		if err != nil {
			respondError(w, http.StatusNotFound, "not_found", "Transcription job not found.")
			return
		}
		formatted, _ := store.ListFormatted(r.Context(), id)
		respond(w, http.StatusOK, transcription.BuildJobResponse(job, segments, formatted))
	}
	mux.HandleFunc("GET /api/v1/transcription/jobs/{id}", handleGet)
	mux.HandleFunc("GET /api/transcriptions/{id}", handleGet)

	handleCancel := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := store.Cancel(r.Context(), id); err != nil {
			respondError(w, http.StatusInternalServerError, "cancel_failed", "Could not cancel job.")
			return
		}
		metrics.JobsCancelled.Add(1)
		job, segments, err := store.Get(r.Context(), id)
		if err != nil {
			respond(w, http.StatusOK, map[string]string{"id": id, "status": "cancelled"})
			return
		}
		formatted, _ := store.ListFormatted(r.Context(), id)
		respond(w, http.StatusOK, transcription.BuildJobResponse(job, segments, formatted))
	}
	mux.HandleFunc("POST /api/v1/transcription/jobs/{id}/cancel", handleCancel)
	mux.HandleFunc("POST /api/transcriptions/{id}/cancel", handleCancel)

	handleRetry := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		job, err := store.Retry(r.Context(), id)
		if err != nil {
			respondError(w, http.StatusConflict, "not_retryable", "Only failed or cancelled jobs can be retried.")
			return
		}
		metrics.JobsRetried.Add(1)
		formatted, _ := store.ListFormatted(r.Context(), id)
		respond(w, http.StatusOK, transcription.BuildJobResponse(job, nil, formatted))
	}
	mux.HandleFunc("POST /api/v1/transcription/jobs/{id}/retry", handleRetry)
	mux.HandleFunc("POST /api/transcriptions/{id}/retry", handleRetry)

	handleListFormatted := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		formatted, err := store.ListFormatted(r.Context(), id)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "list_failed", "Could not list formatted previews.")
			return
		}
		respond(w, http.StatusOK, formatted)
	}
	mux.HandleFunc("GET /api/v1/transcription/jobs/{id}/formatted-transcripts", handleListFormatted)

	handleFormat := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req formatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		preset, _, err := format.NormalizePreset(req.Preset)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid_preset", err.Error())
			return
		}
		job, segments, err := store.Get(r.Context(), id)
		if err != nil || job.Status != "completed" {
			respondError(w, http.StatusConflict, "transcript_not_completed", "Formatting is available only after the original transcript completes.")
			return
		}
		var lines []string
		for _, s := range segments {
			lines = append(lines, s.Speaker+": "+s.Text)
		}
		original := strings.Join(lines, "\n")
		if original == "" {
			original = "No speech segments detected."
		}
		content, err := format.NewClient().Reformat(r.Context(), preset, original)
		if err != nil {
			respondError(w, http.StatusBadGateway, "formatting_unavailable", "Gemini formatting is currently unavailable.")
			return
		}
		version, err := store.SaveFormatted(r.Context(), job.ID, preset, content)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "save_failed", "Could not persist formatted transcript preview.")
			return
		}
		metrics.FormattedPreviews.Add(1)
		respond(w, http.StatusAccepted, version)
	}
	mux.HandleFunc("POST /api/v1/transcription/jobs/{id}/formatted-transcripts", handleFormat)
	mux.HandleFunc("POST /api/transcriptions/{id}/format", handleFormat)

	mux.HandleFunc("GET /api/v1/transcription/jobs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		respond(w, http.StatusOK, map[string]any{"job_id": id, "events": broker.History(id)})
	})

	_ = cfg
	return mux
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]string{
		"error":   code,
		"message": message,
	})
}

func withCORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-Correlation-ID, X-Request-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
