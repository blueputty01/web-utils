package transcription

import (
	"testing"
)

func TestStateTransitionsAndEventBroker(t *testing.T) {
	validSequence := []string{"queued", "resolving", "downloading", "transcribing", "diarizing", "merging", "completed"}
	for i := 0; i < len(validSequence)-1; i++ {
		if err := ValidateTransition(validSequence[i], validSequence[i+1]); err != nil {
			t.Fatalf("expected transition %s -> %s to succeed: %v", validSequence[i], validSequence[i+1], err)
		}
	}
	if err := ValidateTransition("completed", "queued"); err == nil {
		t.Fatal("expected completed -> queued transition to be rejected")
	}
	if err := ValidateTransition("failed", "queued"); err != nil {
		t.Fatalf("expected failed -> queued retry transition to succeed: %v", err)
	}

	broker := NewEventBroker(10)
	for _, phase := range validSequence {
		broker.Publish("job-1", phase)
	}
	events := broker.History("job-1")
	if len(events) != len(validSequence) {
		t.Fatalf("expected %d events, got %d", len(validSequence), len(events))
	}
	if events[len(events)-1].Progress != 100 {
		t.Fatalf("expected completed progress 100, got %d", events[len(events)-1].Progress)
	}
}

func TestBuildJobResponsePreservesOriginalSegmentsWithFormattedPreviews(t *testing.T) {
	job := Job{ID: "job-42", Status: "completed", SourceURL: "https://example.com/v.mp4"}
	segments := []Segment{
		{ID: "1", StartMS: 0, EndMS: 2000, Speaker: "Speaker 1", Text: "Original first line."},
	}
	previews := []FormattedVersion{
		{ID: "fmt-1", JobID: "job-42", Preset: "meeting-notes", Content: "## Notes\n- First line"},
		{ID: "fmt-2", JobID: "job-42", Preset: "summary", Content: "## Summary\nFirst line"},
	}

	dto := BuildJobResponse(job, segments, previews)
	gotSegs, ok := dto["segments"].([]Segment)
	if !ok || len(gotSegs) != 1 || gotSegs[0].Text != "Original first line." {
		t.Fatalf("original segments altered or missing: %#v", dto["segments"])
	}
	gotPreviews, ok := dto["formatted_transcripts"].([]FormattedVersion)
	if !ok || len(gotPreviews) != 2 {
		t.Fatalf("expected 2 formatted transcript versions, got %#v", dto["formatted_transcripts"])
	}
}
