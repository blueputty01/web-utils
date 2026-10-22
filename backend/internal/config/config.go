package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type UtilityDescriptor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Route       string `json:"route"`
	Status      string `json:"status"`
}

type Config struct {
	DatabaseURL           string
	CORSOrigin            string
	ListenAddr            string
	LogLevel              string
	InternalToken         string
	DiarizationURL        string
	WhisperModelPath      string
	WorkDir               string
	GeminiAPIKey          string
	GeminiBaseURL         string
	ObjectStoreEndpoint   string
	ObjectStoreBucket     string
	ObjectStoreAccessKey  string
	ObjectStoreSecretKey  string
	ArtifactTTL           time.Duration
	RetentionWindow       time.Duration
	RateLimitPerMinute    int
	AllowedSourceHosts    []string
	FakePipelineEnabled   bool
}

func Load() Config {
	ttl, err := time.ParseDuration(envOrDefault("ARTIFACT_TTL", "1h"))
	if err != nil || ttl <= 0 {
		ttl = time.Hour
	}
	retention, err := time.ParseDuration(envOrDefault("RETENTION_WINDOW", "168h"))
	if err != nil || retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	rateLimit, err := strconv.Atoi(envOrDefault("RATE_LIMIT_PER_MINUTE", "120"))
	if err != nil || rateLimit <= 0 {
		rateLimit = 120
	}
	var allowedHosts []string
	if raw := strings.TrimSpace(os.Getenv("ALLOWED_SOURCE_HOSTS")); raw != "" {
		for _, host := range strings.Split(raw, ",") {
			if h := strings.ToLower(strings.TrimSpace(host)); h != "" {
				allowedHosts = append(allowedHosts, h)
			}
		}
	}
	return Config{
		DatabaseURL:          secretOrEnv("DATABASE_URL", ""),
		CORSOrigin:           envOrDefault("CORS_ORIGIN", "http://localhost:3000"),
		ListenAddr:           envOrDefault("LISTEN_ADDR", ":8080"),
		LogLevel:             envOrDefault("LOG_LEVEL", "info"),
		InternalToken:        secretOrEnv("INTERNAL_TOKEN", "development-only"),
		DiarizationURL:       envOrDefault("DIARIZATION_URL", "http://diarization:8000"),
		WhisperModelPath:     envOrDefault("WHISPER_MODEL_PATH", "/models/ggml-base.en.bin"),
		WorkDir:              envOrDefault("WORK_DIR", os.TempDir()),
		GeminiAPIKey:         secretOrEnv("GEMINI_API_KEY", ""),
		GeminiBaseURL:        envOrDefault("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		ObjectStoreEndpoint:  envOrDefault("OBJECT_STORE_ENDPOINT", ""),
		ObjectStoreBucket:    envOrDefault("OBJECT_STORE_BUCKET", "artifacts"),
		ObjectStoreAccessKey: secretOrEnv("OBJECT_STORE_ACCESS_KEY", "webutils"),
		ObjectStoreSecretKey: secretOrEnv("OBJECT_STORE_SECRET_KEY", "change-me-in-production"),
		ArtifactTTL:          ttl,
		RetentionWindow:      retention,
		RateLimitPerMinute:   rateLimit,
		AllowedSourceHosts:   allowedHosts,
		FakePipelineEnabled:  strings.EqualFold(os.Getenv("FAKE_PIPELINE"), "true") || strings.EqualFold(os.Getenv("FAKE_PIPELINE"), "1"),
	}
}

func (c Config) ValidateAPI() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}

func (c Config) ValidateWorker() error {
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if strings.TrimSpace(c.DiarizationURL) == "" {
		return fmt.Errorf("DIARIZATION_URL is required")
	}
	if strings.TrimSpace(c.InternalToken) == "" {
		return fmt.Errorf("INTERNAL_TOKEN is required")
	}
	return nil
}

func ConfigureLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(logger)
	return logger
}

func Utilities() []UtilityDescriptor {
	return []UtilityDescriptor{
		{
			ID:          "video-transcription",
			Name:        "Video transcription",
			Description: "Create speaker-attributed transcripts from video URLs and generate editable Gemini Flash previews.",
			Route:       "/utilities/video-transcription",
			Status:      "available",
		},
	}
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func secretOrEnv(key, fallback string) string {
	if filePath := strings.TrimSpace(os.Getenv(key + "_FILE")); filePath != "" {
		if data, err := os.ReadFile(filePath); err == nil {
			if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
				return trimmed
			}
		}
	}
	return envOrDefault(key, fallback)
}
