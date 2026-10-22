package diarization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDiarizeClientRetriesAndDecodes(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["audio_key"] != "jobs/1/audio.wav" {
			t.Errorf("expected audio_key jobs/1/audio.wav, got %q", req["audio_key"])
		}
		if calls.Add(1) == 1 {
			http.Error(w, "temporary error", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Turn{
			{StartMS: 0, EndMS: 2500, Speaker: "Speaker 1"},
			{StartMS: 2500, EndMS: 5000, Speaker: "Speaker 2"},
		})
	}))
	defer srv.Close()

	client := Client{
		URL:        srv.URL,
		Token:      "secret-token",
		HTTP:       srv.Client(),
		MaxRetries: 2,
	}
	turns, err := client.DiarizeWithKey(context.Background(), "/work/audio.wav", "jobs/1/audio.wav")
	if err != nil {
		t.Fatalf("DiarizeWithKey failed: %v", err)
	}
	if len(turns) != 2 || turns[0].Speaker != "Speaker 1" || turns[1].Speaker != "Speaker 2" {
		t.Fatalf("unexpected turns: %#v", turns)
	}
}
