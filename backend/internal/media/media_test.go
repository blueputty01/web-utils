package media

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeResolver struct {
	ips []net.IPAddr
	err error
}

func (f fakeResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	return f.ips, f.err
}

type fakeRunner struct {
	calls [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return []byte("vid-123"), nil
}

func TestValidate(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/video.mp4",
		"http://169.254.169.254/latest/meta-data",
		"http://localhost:8080/video.mp4",
		"http://service.internal/video.mp4",
		"https://user:pass@example.com/video.mp4",
		"ftp://example.com/a.mp4",
		"not a url",
	} {
		if err := Validate(raw); err == nil {
			t.Fatalf("Validate(%q) unexpectedly succeeded", raw)
		}
	}
	if err := Validate("https://example.com/video.mp4"); err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
}

func TestAllowlistAndRedirectSafety(t *testing.T) {
	if _, err := NormalizeURL("https://untrusted.example.org/video.mp4", []string{"trusted.example.com"}); err == nil {
		t.Fatal("expected allowlist rejection")
	}
	if _, err := NormalizeURL("https://sub.trusted.example.com/video.mp4", []string{"trusted.example.com"}); err != nil {
		t.Fatalf("expected subdomain to match allowlist: %v", err)
	}

	privateResolver := fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("10.0.0.8")}}}
	if err := ValidateResolvedHost(context.Background(), "https://example.com/video.mp4", privateResolver); err == nil {
		t.Fatal("expected private resolved IP to be rejected")
	}

	publicResolver := fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
	policy := SafeRedirectPolicy(context.Background(), publicResolver, []string{"example.com"})
	u, _ := url.Parse("http://127.0.0.1/secret")
	if err := policy(&http.Request{URL: u}, []*http.Request{{}}); err == nil {
		t.Fatal("expected redirect to loopback to be rejected")
	}
}

func TestNormalizeWithRunnerArguments(t *testing.T) {
	runner := &fakeRunner{}
	resolver := fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
	out := filepath.Join(t.TempDir(), "audio.wav")

	if err := NormalizeWithRunner(context.Background(), "https://example.com/watch?v=abc#frag", out, runner, resolver); err != nil {
		t.Fatalf("NormalizeWithRunner failed: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 subprocess invocations, got %d", len(runner.calls))
	}
	if runner.calls[0][0] != "yt-dlp" || !strings.Contains(strings.Join(runner.calls[0], " "), "--no-playlist") || !strings.Contains(strings.Join(runner.calls[0], " "), "-o "+out+".source") {
		t.Fatalf("unexpected yt-dlp invocation: %v", runner.calls[0])
	}
	if runner.calls[1][0] != "ffmpeg" || !strings.Contains(strings.Join(runner.calls[1], " "), "-i "+out+".source") || !strings.Contains(strings.Join(runner.calls[1], " "), "-ar 16000") {
		t.Fatalf("unexpected ffmpeg invocation: %v", runner.calls[1])
	}
}

func TestFakeMediaWAVGeneration(t *testing.T) {
	t.Setenv("FAKE_MEDIA_ENABLED", "true")
	out := filepath.Join(t.TempDir(), "synthetic.wav")
	if err := Normalize(context.Background(), "https://example.com/demo.mp4", out); err != nil {
		t.Fatalf("fake media normalization failed: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil || len(data) < 44 || string(data[:4]) != "RIFF" {
		t.Fatalf("expected valid WAV file, got len=%d err=%v", len(data), err)
	}
}
