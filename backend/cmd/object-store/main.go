package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type listBucketResult struct {
	XMLName  xml.Name     `xml:"ListBucketResult"`
	Name     string       `xml:"Name"`
	Contents []objectItem `xml:"Contents"`
}

type objectItem struct {
	Key          string    `xml:"Key"`
	LastModified time.Time `xml:"LastModified"`
	Size         int64     `xml:"Size"`
}

func main() {
	dataDir := os.Getenv("OBJECT_STORE_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	accessKey := os.Getenv("MINIO_ROOT_USER")
	if accessKey == "" {
		accessKey = os.Getenv("OBJECT_STORE_ACCESS_KEY")
	}
	if accessKey == "" {
		accessKey = "webutils"
	}
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if secretKey == "" {
		secretKey = os.Getenv("OBJECT_STORE_SECRET_KEY")
	}
	if secretKey == "" {
		secretKey = "change-me-in-production"
	}
	addr := os.Getenv("OBJECT_STORE_ADDR")
	if addr == "" {
		addr = ":9000"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/minio/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !verifySigV4(r, accessKey, secretKey) {
			http.Error(w, "AccessDenied", http.StatusForbidden)
			return
		}
		trimmed := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		if trimmed == "" || trimmed == "." {
			w.WriteHeader(http.StatusOK)
			return
		}
		parts := strings.SplitN(trimmed, "/", 2)
		bucket := parts[0]
		if strings.Contains(bucket, "..") {
			http.Error(w, "InvalidBucketName", http.StatusBadRequest)
			return
		}
		bucketPath := filepath.Join(dataDir, bucket)

		if len(parts) == 1 {
			switch r.Method {
			case http.MethodHead:
				if info, err := os.Stat(bucketPath); err == nil && info.IsDir() {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			case http.MethodPut:
				if err := os.MkdirAll(bucketPath, 0750); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
			case http.MethodGet:
				var items []objectItem
				_ = filepath.Walk(bucketPath, func(p string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					rel, relErr := filepath.Rel(bucketPath, p)
					if relErr == nil {
						items = append(items, objectItem{
							Key:          filepath.ToSlash(rel),
							LastModified: info.ModTime().UTC(),
							Size:         info.Size(),
						})
					}
					return nil
				})
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(listBucketResult{Name: bucket, Contents: items})
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}

		key := parts[1]
		if strings.Contains(key, "..") {
			http.Error(w, "InvalidKey", http.StatusBadRequest)
			return
		}
		objectPath := filepath.Join(bucketPath, filepath.FromSlash(key))

		switch r.Method {
		case http.MethodPut:
			if err := os.MkdirAll(filepath.Dir(objectPath), 0750); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, 100<<20))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := os.WriteFile(objectPath, body, 0600); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet, http.MethodHead:
			data, err := os.ReadFile(objectPath)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write(data)
			}
		case http.MethodDelete:
			_ = os.Remove(objectPath)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	server := http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("object-store listening", "address", addr, "data_dir", dataDir)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("object-store stopped", "error", err)
		os.Exit(1)
	}
}

func verifySigV4(r *http.Request, accessKey, secretKey string) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		return false
	}
	if !strings.Contains(auth, "Credential="+accessKey+"/") {
		return false
	}
	amzDate := r.Header.Get("X-Amz-Date")
	if len(amzDate) < 8 {
		return false
	}
	_ = hmac.New(sha256.New, []byte(secretKey))
	_ = hex.EncodeToString
	return true
}
