package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

var DefaultRunner Runner = ExecRunner{}

const (
	MaxMediaDurationSec = "3600"
	DefaultAcquireLimit = 10 * time.Minute
)

func Normalize(ctx context.Context, sourceURL, output string) error {
	return NormalizeWithRunner(ctx, sourceURL, output, DefaultRunner, DefaultResolver)
}

func NormalizeWithRunner(ctx context.Context, sourceURL, output string, runner Runner, resolver Resolver) error {
	normalizedURL, err := NormalizeURL(sourceURL, allowedHostsFromEnv())
	if err != nil {
		return err
	}
	if fakeMediaEnabled() {
		if strings.Contains(strings.ToLower(normalizedURL), "fail-media") {
			return fmt.Errorf("source resolution failed: simulated media failure")
		}
		return WriteSyntheticWAV(output, 16000, 2)
	}
	if err := ValidateResolvedHost(ctx, normalizedURL, resolver); err != nil {
		return err
	}
	if runner == nil {
		runner = DefaultRunner
	}
	runCtx, cancel := context.WithTimeout(ctx, DefaultAcquireLimit)
	defer cancel()

	// yt-dlp supports platform pages and direct URLs; FFmpeg normalizes only the resolved media.
	if out, err := runner.Run(runCtx, "yt-dlp", "--no-playlist", "--max-filesize", "500M", "--get-id", normalizedURL); err != nil {
		return fmt.Errorf("source resolution failed: %s", strings.TrimSpace(string(out)))
	}
	if out, err := runner.Run(runCtx, "ffmpeg", "-nostdin", "-y", "-i", normalizedURL, "-t", MaxMediaDurationSec, "-vn", "-ac", "1", "-ar", "16000", "-acodec", "pcm_s16le", output); err != nil {
		return fmt.Errorf("audio extraction failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func WriteSyntheticWAV(path string, sampleRate int, durationSec int) error {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	if durationSec <= 0 {
		durationSec = 2
	}
	numSamples := sampleRate * durationSec
	dataSize := numSamples * 2
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // Mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataSize))
	for i := 0; i < numSamples; i++ {
		sample := int16(math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)) * 4000)
		_ = binary.Write(&buf, binary.LittleEndian, sample)
	}
	return os.WriteFile(path, buf.Bytes(), 0600)
}

func fakeMediaEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_MEDIA_ENABLED")))
	p := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_PIPELINE")))
	return v == "true" || v == "1" || p == "true" || p == "1"
}
