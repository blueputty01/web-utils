package format

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiClientAndPresets(t *testing.T) {
	if _, _, err := NormalizePreset("invalid-preset"); err == nil {
		t.Fatal("expected invalid preset error")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "key=real-key") {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{
				map[string]any{
					"content": map[string]any{
						"parts": []any{
							map[string]string{"text": "## Formatted Notes\n- Speaker 1 welcomed everyone."},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := Client{
		Key:     "real-key",
		BaseURL: srv.URL,
		HTTP:    srv.Client(),
	}
	original := "Speaker 1: Welcome everyone."
	result, err := client.Reformat(context.Background(), "meeting-notes", original)
	if err != nil {
		t.Fatalf("Reformat failed: %v", err)
	}
	if !strings.Contains(result, "Formatted Notes") {
		t.Fatalf("unexpected formatted result: %q", result)
	}
	if original != "Speaker 1: Welcome everyone." {
		t.Fatal("original transcript was mutated")
	}
}
