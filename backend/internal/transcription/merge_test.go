package transcription

import (
	"testing"

	"github.com/example/web-utils/backend/internal/diarization"
)

func TestMergeSpeakerTurns(t *testing.T) {
	segments := []Segment{
		{StartMS: 0, EndMS: 2000, Text: "Welcome to the session."},
		{StartMS: 2000, EndMS: 5000, Text: "Thanks for having me."},
		{StartMS: 6000, EndMS: 7500, Text: "Unattributed remark."},
	}
	turns := []diarization.Turn{
		{StartMS: 0, EndMS: 2400, Speaker: "Speaker 1"},
		{StartMS: 2200, EndMS: 5200, Speaker: "Speaker 2"},
	}

	merged := MergeSpeakerTurns(segments, turns)
	if len(merged) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(merged))
	}
	if merged[0].Speaker != "Speaker 1" {
		t.Fatalf("segment 0: expected Speaker 1, got %q", merged[0].Speaker)
	}
	if merged[1].Speaker != "Speaker 2" {
		t.Fatalf("segment 1: expected Speaker 2, got %q", merged[1].Speaker)
	}
	if merged[2].Speaker != "Unknown" {
		t.Fatalf("segment 2: expected Unknown fallback, got %q", merged[2].Speaker)
	}
	if merged[0].Text != segments[0].Text || merged[0].StartMS != segments[0].StartMS {
		t.Fatal("segment text or timestamps were modified during merge")
	}
}
