//go:build whisper

package transcription

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/wav"
)

// Transcribe loads a 16 kHz mono WAV and processes it through whisper.cpp.
func Transcribe(ctx context.Context, modelPath, wavPath string) ([]Segment, error) {
	file, err := os.Open(wavPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := wav.NewDecoder(file)
	buffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, fmt.Errorf("decode WAV: %w", err)
	}
	if decoder.NumChans != 1 || int(decoder.SampleRate) != whisper.SampleRate {
		return nil, fmt.Errorf("expected mono %d Hz WAV", whisper.SampleRate)
	}
	if fakeWhisperEnabled() {
		return syntheticSegments(), nil
	}
	if modelMissing(modelPath) {
		return nil, fmt.Errorf("whisper model file missing or empty at %q", modelPath)
	}
	audio := make([]float32, len(buffer.Data))
	for i, sample := range buffer.Data {
		audio[i] = float32(sample) / 32768
	}
	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, err
	}
	defer model.Close()
	engine, err := model.NewContext()
	if err != nil {
		return nil, err
	}
	if engine.IsMultilingual() {
		if err = engine.SetLanguage("auto"); err != nil {
			return nil, err
		}
	}
	var result []Segment
	err = engine.Process(audio, func() bool { return ctx.Err() == nil }, func(segment whisper.Segment) {
		result = append(result, Segment{
			StartMS: int(segment.Start.Milliseconds()),
			EndMS:   int(segment.End.Milliseconds()),
			Text:    strings.TrimSpace(segment.Text),
		})
	}, nil)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func fakeWhisperEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_WHISPER_ENABLED")))
	p := strings.ToLower(strings.TrimSpace(os.Getenv("FAKE_PIPELINE")))
	return v == "true" || v == "1" || p == "true" || p == "1"
}

func modelMissing(path string) bool {
	if strings.TrimSpace(path) == "" {
		return true
	}
	info, err := os.Stat(path)
	return err != nil || info.IsDir() || info.Size() == 0
}

func syntheticSegments() []Segment {
	return []Segment{
		{StartMS: 0, EndMS: 2000, Text: "Welcome to the video transcription utility demo."},
		{StartMS: 2000, EndMS: 4500, Text: "We are testing speaker diarization and Gemini formatting."},
	}
}
