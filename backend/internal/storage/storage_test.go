package storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleWAV() []byte {
	var buf bytes.Buffer
	numSamples := 160
	dataSize := numSamples * 2
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16000))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(32000))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(make([]byte, dataSize))
	return buf.Bytes()
}

func TestVerifyWAVAndMemoryStore(t *testing.T) {
	if err := VerifyWAV([]byte("not a wav")); err == nil {
		t.Fatal("expected non-WAV to fail verification")
	}
	wav := sampleWAV()
	if err := VerifyWAV(wav); err != nil {
		t.Fatalf("valid WAV rejected: %v", err)
	}

	store := NewMemoryStore(10 * time.Minute)
	art, err := store.Put(context.Background(), "jobs/123/audio.wav", wav, "audio/wav")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	got, err := store.Get(context.Background(), art.Key)
	if err != nil || !bytes.Equal(got, wav) {
		t.Fatalf("Get mismatch: %v", err)
	}

	removed, err := store.CleanupExpired(context.Background(), art.ExpiresAt.Add(time.Second))
	if err != nil || removed != 1 {
		t.Fatalf("expected 1 expired artifact removed, got %d (%v)", removed, err)
	}
	if _, err := store.Get(context.Background(), art.Key); err == nil {
		t.Fatal("expected expired artifact to be absent")
	}
}

func TestS3StoreLifecycleAndTTL(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	buckets := map[string]bool{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			http.Error(w, "missing SigV4 header", http.StatusForbidden)
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		bucket := parts[0]
		key := ""
		if len(parts) > 1 {
			key = parts[1]
		}
		mu.Lock()
		defer mu.Unlock()

		if key == "" {
			switch r.Method {
			case http.MethodHead:
				if !buckets[bucket] {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusOK)
			case http.MethodPut:
				buckets[bucket] = true
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<ListBucketResult></ListBucketResult>`))
			}
			return
		}

		fullKey := bucket + "/" + key
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			objects[fullKey] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			data, ok := objects[fullKey]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		case http.MethodDelete:
			delete(objects, fullKey)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	s3 := NewS3Store(srv.URL, "artifacts", "webutils", "change-me-in-production", 5*time.Minute)
	ctx := context.Background()
	wav := sampleWAV()

	art, err := s3.Put(ctx, "jobs/job-1/audio.wav", wav, "audio/wav")
	if err != nil {
		t.Fatalf("S3 Put failed: %v", err)
	}
	loaded, err := s3.Get(ctx, art.Key)
	if err != nil || !bytes.Equal(loaded, wav) {
		t.Fatalf("S3 Get failed: %v", err)
	}
	cleaned, err := s3.CleanupExpired(ctx, art.ExpiresAt.Add(time.Second))
	if err != nil || cleaned != 1 {
		t.Fatalf("expected 1 cleaned object, got %d (%v)", cleaned, err)
	}
	if _, err := s3.Get(ctx, art.Key); err == nil {
		t.Fatal("expected deleted object to return error")
	}
}
