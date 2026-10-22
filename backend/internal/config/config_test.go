package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndSecretFile(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "gemini_key")
	if err := os.WriteFile(secretFile, []byte("secret-from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_API_KEY_FILE", secretFile)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
	t.Setenv("ALLOWED_SOURCE_HOSTS", "Example.com, cdn.example.org ")

	cfg := Load()
	if cfg.GeminiAPIKey != "secret-from-file" {
		t.Fatalf("expected secret-from-file, got %q", cfg.GeminiAPIKey)
	}
	if err := cfg.ValidateAPI(); err != nil {
		t.Fatalf("unexpected ValidateAPI error: %v", err)
	}
	if err := cfg.ValidateWorker(); err != nil {
		t.Fatalf("unexpected ValidateWorker error: %v", err)
	}
	if len(cfg.AllowedSourceHosts) != 2 || cfg.AllowedSourceHosts[0] != "example.com" {
		t.Fatalf("unexpected AllowedSourceHosts: %#v", cfg.AllowedSourceHosts)
	}
	utils := Utilities()
	if len(utils) == 0 || utils[0].ID != "video-transcription" {
		t.Fatalf("unexpected utility registry: %#v", utils)
	}
}
