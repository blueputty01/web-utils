package diarization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Turn struct {
	StartMS int    `json:"start_ms"`
	EndMS   int    `json:"end_ms"`
	Speaker string `json:"speaker"`
}

type Client struct {
	URL, Token string
	HTTP       *http.Client
	MaxRetries int
}

func New() Client {
	return Client{
		URL:        strings.TrimRight(os.Getenv("DIARIZATION_URL"), "/"),
		Token:      loadToken(),
		HTTP:       &http.Client{Timeout: 5 * time.Minute},
		MaxRetries: 2,
	}
}

func loadToken() string {
	if path := strings.TrimSpace(os.Getenv("INTERNAL_TOKEN_FILE")); path != "" {
		if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data))
		}
	}
	return os.Getenv("INTERNAL_TOKEN")
}

func (c Client) Diarize(ctx context.Context, audioPath string) ([]Turn, error) {
	return c.DiarizeWithKey(ctx, audioPath, "")
}

func (c Client) DiarizeWithKey(ctx context.Context, audioPath, audioKey string) ([]Turn, error) {
	if c.URL == "" {
		return nil, fmt.Errorf("diarization is not configured")
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	payload := map[string]string{"audio_path": audioPath}
	if audioKey != "" {
		payload["audio_key"] = audioKey
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	attempts := c.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/v1/diarize", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", c.Token)
		res, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if res.StatusCode/100 == 2 {
			var turns []Turn
			decodeErr := json.NewDecoder(res.Body).Decode(&turns)
			res.Body.Close()
			return turns, decodeErr
		}
		status := res.Status
		code := res.StatusCode
		res.Body.Close()
		lastErr = fmt.Errorf("diarization returned %s", status)
		if code < 500 {
			break
		}
	}
	return nil, lastErr
}
