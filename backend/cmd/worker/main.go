package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/example/web-utils/backend/internal/config"
	"github.com/example/web-utils/backend/internal/diarization"
	"github.com/example/web-utils/backend/internal/media"
	"github.com/example/web-utils/backend/internal/storage"
	"github.com/example/web-utils/backend/internal/transcription"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()
	config.ConfigureLogger(cfg.LogLevel)
	if err := cfg.ValidateWorker(); err != nil {
		slog.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("database configuration", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	var artifactStore storage.Store
	if cfg.ObjectStoreEndpoint != "" {
		artifactStore = storage.NewS3Store(cfg.ObjectStoreEndpoint, cfg.ObjectStoreBucket, cfg.ObjectStoreAccessKey, cfg.ObjectStoreSecretKey, cfg.ArtifactTTL)
	} else {
		artifactStore = storage.NewMemoryStore(cfg.ArtifactTTL)
	}

	store := transcription.Store{DB: pool, Events: transcription.NewEventBroker(32)}
	pollTicker := time.NewTicker(500 * time.Millisecond)
	defer pollTicker.Stop()
	cleanupTicker := time.NewTicker(time.Minute)
	defer cleanupTicker.Stop()

	slog.Info("worker started", "object_store", cfg.ObjectStoreEndpoint, "bucket", cfg.ObjectStoreBucket)
	for {
		select {
		case <-pollTicker.C:
			process(context.Background(), cfg, store, artifactStore)
		case <-cleanupTicker.C:
			runCleanup(context.Background(), cfg, store, artifactStore)
		}
	}
}

func process(ctx context.Context, cfg config.Config, store transcription.Store, artifactStore storage.Store) {
	job, err := store.Claim(ctx)
	if err != nil {
		slog.Error("claim job", "error", err)
		return
	}
	if job.ID == "" {
		return
	}
	slog.Info("claimed job", "job_id", job.ID, "source_url", job.SourceURL)

	root := cfg.WorkDir
	if root == "" {
		root = os.TempDir()
	}
	work := filepath.Join(root, "web-utils", job.ID)
	defer os.RemoveAll(work)
	if err := os.MkdirAll(work, 0700); err != nil {
		_ = store.Fail(ctx, job.ID, "workspace_failed")
		return
	}

	wav := filepath.Join(work, "audio.wav")
	if err := store.Touch(ctx, job.ID, "downloading"); err != nil {
		return
	}
	if err := media.Normalize(ctx, job.SourceURL, wav); err != nil {
		slog.Warn("media normalization failed", "job_id", job.ID, "error", err)
		_ = store.Fail(ctx, job.ID, "media_failed")
		return
	}

	wavBytes, err := os.ReadFile(wav)
	if err != nil {
		_ = store.Fail(ctx, job.ID, "media_failed")
		return
	}
	artifactKey := fmt.Sprintf("jobs/%s/audio.wav", job.ID)
	if _, err := artifactStore.Put(ctx, artifactKey, wavBytes, "audio/wav"); err != nil {
		slog.Warn("artifact storage failed", "job_id", job.ID, "error", err)
		_ = store.Fail(ctx, job.ID, "storage_failed")
		return
	}
	defer func() {
		_ = artifactStore.Delete(ctx, artifactKey)
	}()

	if err := store.Touch(ctx, job.ID, "transcribing"); err != nil {
		return
	}
	segments, err := transcription.Transcribe(ctx, cfg.WhisperModelPath, wav)
	if err != nil {
		slog.Warn("whisper transcription failed", "job_id", job.ID, "error", err)
		_ = store.Fail(ctx, job.ID, "transcription_failed")
		return
	}

	if err := store.Touch(ctx, job.ID, "diarizing"); err != nil {
		return
	}
	turns, err := diarization.New().DiarizeWithKey(ctx, wav, artifactKey)
	if err != nil {
		slog.Warn("diarization failed", "job_id", job.ID, "error", err)
		_ = store.Fail(ctx, job.ID, "diarization_failed")
		return
	}

	if err := store.Touch(ctx, job.ID, "merging"); err != nil {
		return
	}
	merged := transcription.MergeSpeakerTurns(segments, turns)
	if err := store.CompleteWithTurns(ctx, job.ID, merged, turns); err != nil {
		slog.Error("complete job", "job_id", job.ID, "error", err)
		return
	}
	slog.Info("completed job", "job_id", job.ID, "segments", len(merged))
}

func runCleanup(ctx context.Context, cfg config.Config, store transcription.Store, artifactStore storage.Store) {
	if artifactStore != nil {
		if removed, err := artifactStore.CleanupExpired(ctx, time.Now().UTC()); err == nil && removed > 0 {
			slog.Info("cleaned expired artifacts", "count", removed)
		}
	}
	if deleted, err := store.CleanupExpired(ctx, cfg.RetentionWindow); err == nil && deleted > 0 {
		slog.Info("cleaned expired job records", "count", deleted)
	}
}
