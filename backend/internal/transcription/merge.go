package transcription

import "github.com/example/web-utils/backend/internal/diarization"

// MergeSpeakerTurns deterministically assigns each transcript segment to the speaker turn
// with the greatest positive time overlap in milliseconds, falling back to "Unknown".
func MergeSpeakerTurns(segments []Segment, turns []diarization.Turn) []Segment {
	out := make([]Segment, len(segments))
	copy(out, segments)
	for i := range out {
		best := 0
		speaker := ""
		for _, turn := range turns {
			overlap := minInt(out[i].EndMS, turn.EndMS) - maxInt(out[i].StartMS, turn.StartMS)
			if overlap > best {
				best = overlap
				speaker = turn.Speaker
			}
		}
		if speaker == "" {
			speaker = "Unknown"
		}
		out[i].Speaker = speaker
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
