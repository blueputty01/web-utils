package format

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

var allowedPresets = map[string]string{
	"meeting-notes":    "structured meeting notes with key takeaways, decisions, and speaker attribution",
	"clean-transcript": "a clean, readable transcript with polished punctuation and speaker labels",
	"summary":          "a concise executive summary highlighting main topics and speaker points",
	"readable":         "a clean, readable transcript with polished punctuation and speaker labels",
}

func NormalizePreset(preset string) (string, string, error) {
	cleaned := strings.ToLower(strings.TrimSpace(preset))
	if cleaned == "" {
		cleaned = "meeting-notes"
	}
	instruction, ok := allowedPresets[cleaned]
	if !ok {
		return "", "", fmt.Errorf("unsupported format preset %q", preset)
	}
	if cleaned == "readable" {
		cleaned = "clean-transcript"
	}
	return cleaned, instruction, nil
}

type Client struct {
	Key     string
	BaseURL string
	HTTP    *http.Client
}

func NewClient() Client {
	return Client{
		Key:     loadGeminiKey(),
		BaseURL: strings.TrimRight(envOr("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"), "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

func loadGeminiKey() string {
	if path := strings.TrimSpace(os.Getenv("GEMINI_API_KEY_FILE")); path != "" {
		if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data))
		}
	}
	return strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
}

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func (c Client) Reformat(ctx context.Context, preset, transcript string) (string, error) {
	normalizedPreset, instruction, err := NormalizePreset(preset)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(transcript) == "" {
		return "", fmt.Errorf("transcript content is required")
	}
	if len(transcript) > 200_000 {
		return "", fmt.Errorf("transcript exceeds maximum formatting size")
	}

	if fakeGeminiEnabled() || c.Key == "fake" || strings.HasPrefix(c.Key, "test-") {
		return buildDeterministicPreview(normalizedPreset, transcript), nil
	}
	if c.Key == "" {
		return "", fmt.Errorf("Gemini is not configured")
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	prompt := "Reformat this transcript as " + instruction + ". Preserve meaning; do not invent content.\n\n" + transcript
	body, _ := json.Marshal(map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]string{"text": prompt},
				},
			},
		},
	})

	endpoint := baseURL + "/v1beta/models/gemini-2.0-flash:generateContent?key=" + c.Key
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("Gemini returned %s", resp.Status)
	}
	var decoded struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}
	if len(decoded.Candidates) == 0 || len(decoded.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned no content")
	}
	return decoded.Candidates[0].Content.Parts[0].Text, nil
}

func fakeGeminiEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_GEMINI_ENABLED")))
	p := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_PIPELINE")))
	return v == "true" || v == "1" || p == "true" || p == "1"
}

func buildDeterministicPreview(preset, transcript string) string {
	switch preset {
	case "summary":
		return "## Executive Summary\n\n" + transcript
	case "clean-transcript":
		return "## Clean Transcript\n\n" + transcript
	default:
		return "## Meeting Notes\n\n- Key discussion points:\n" + transcript
	}
}
