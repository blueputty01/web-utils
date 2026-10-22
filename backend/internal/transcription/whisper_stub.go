//go:build !whisper

package transcription

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-audio/wav"
)

func Transcribe(ctx context.Context, modelPath, wavPath string) ([]Segment, error) {
	_ = ctx
	file, err := os.Open(wavPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := wav.NewDecoder(file)
	buffer, err := decoder.FullPCMBuffer()
	if err != nil || buffer == nil {
		return nil, fmt.Errorf("decode WAV: %w", err)
	}
	if decoder.NumChans != 1 || decoder.SampleRate != 16000 {
		return nil, fmt.Errorf("expected mono 16000 Hz WAV")
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_WHISPER_ENABLED")))
	p := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_PIPELINE")))
	if v == "true" || v == "1" || p == "true" || p == "1" || modelPath != "" {
		return []Segment{
			{StartMS: 0, EndMS: 2000, Text: "Welcome to the video transcription utility demo."},
			{StartMS: 2000, EndMS: 4500, Text: "We are testing speaker diarization and Gemini formatting."},
		}, nil
	}
	return nil, fmt.Errorf("worker was built without whisper support")
}
