package transcription

import (
	"fmt"
	"sync"
	"time"
)

type JobEvent struct {
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Phase     string    `json:"phase"`
	Progress  int       `json:"progress"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type EventBroker struct {
	mu      sync.Mutex
	maxPer  int
	history map[string][]JobEvent
}

func NewEventBroker(maxPerJob int) *EventBroker {
	if maxPerJob <= 0 {
		maxPerJob = 32
	}
	return &EventBroker{
		maxPer:  maxPerJob,
		history: make(map[string][]JobEvent),
	}
}

func (b *EventBroker) Publish(jobID, status string) JobEvent {
	evt := JobEvent{
		JobID:     jobID,
		Status:    status,
		Phase:     PhaseLabel(status),
		Progress:  PhaseProgress(status),
		Message:   PhaseMessage(status),
		Timestamp: time.Now().UTC(),
	}
	b.mu.Lock()
	list := append(b.history[jobID], evt)
	if len(list) > b.maxPer {
		list = list[len(list)-b.maxPer:]
	}
	b.history[jobID] = list
	b.mu.Unlock()
	return evt
}

func (b *EventBroker) History(jobID string) []JobEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	src := b.history[jobID]
	out := make([]JobEvent, len(src))
	copy(out, src)
	return out
}

func (b *EventBroker) Prune(maxJobs int) int {
	if maxJobs <= 0 {
		maxJobs = 500
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	removed := 0
	for k := range b.history {
		if len(b.history) <= maxJobs {
			break
		}
		delete(b.history, k)
		removed++
	}
	return removed
}

func PhaseProgress(status string) int {
	switch status {
	case "queued":
		return 0
	case "resolving":
		return 15
	case "downloading":
		return 35
	case "transcribing":
		return 65
	case "diarizing":
		return 85
	case "merging":
		return 95
	case "completed":
		return 100
	default:
		return 0
	}
}

func PhaseLabel(status string) string {
	switch status {
	case "queued":
		return "Queued for worker"
	case "resolving":
		return "Resolving video source"
	case "downloading":
		return "Downloading and normalizing audio"
	case "transcribing":
		return "Transcribing speech with Whisper"
	case "diarizing":
		return "Identifying speakers"
	case "merging":
		return "Aligning speaker turns"
	case "completed":
		return "Completed"
	case "failed":
		return "Failed"
	case "cancelled":
		return "Cancelled"
	default:
		return status
	}
}

func PhaseMessage(status string) string {
	switch status {
	case "queued":
		return "Your transcription job is queued and waiting for an available worker."
	case "resolving":
		return "Validating URL and resolving media stream metadata."
	case "downloading":
		return "Extracting and normalizing 16 kHz mono WAV audio."
	case "transcribing":
		return "Generating timestamped transcript segments."
	case "diarizing":
		return "Running speaker diarization on normalized audio."
	case "merging":
		return "Attributing transcript segments to detected speakers."
	case "completed":
		return "Original speaker-attributed transcript is ready."
	case "failed":
		return "Processing failed safely. You can retry this job."
	case "cancelled":
		return "This job was cancelled."
	default:
		return ""
	}
}

func SafeErrorMessage(code string) string {
	switch code {
	case "workspace_failed":
		return "Temporary workspace allocation failed. Please retry the job."
	case "media_failed":
		return "Could not download or normalize media from the provided URL. Check that the URL is publicly accessible and supported."
	case "storage_failed":
		return "Failed to persist intermediate audio in private object storage. Please retry."
	case "transcription_failed":
		return "Audio transcription failed safely. You can retry this job."
	case "diarization_failed":
		return "Speaker diarization service was unable to process the audio. You can retry this job."
	default:
		if code == "" {
			return ""
		}
		return "Processing failed safely. You can retry this job."
	}
}

var allowedTransitions = map[string]map[string]bool{
	"queued":       {"resolving": true, "cancelled": true, "failed": true},
	"resolving":    {"downloading": true, "cancelled": true, "failed": true},
	"downloading":  {"transcribing": true, "cancelled": true, "failed": true},
	"transcribing": {"diarizing": true, "cancelled": true, "failed": true},
	"diarizing":    {"merging": true, "completed": true, "cancelled": true, "failed": true},
	"merging":      {"completed": true, "cancelled": true, "failed": true},
	"failed":       {"queued": true},
	"cancelled":    {"queued": true},
	"completed":    {},
}

func ValidateTransition(from, to string) error {
	if from == to {
		return nil
	}
	next, ok := allowedTransitions[from]
	if !ok || !next[to] {
		return fmt.Errorf("invalid job state transition from %q to %q", from, to)
	}
	return nil
}

func BuildJobResponse(job Job, segments []Segment, formatted []FormattedVersion) map[string]any {
	if segments == nil {
		segments = []Segment{}
	}
	if formatted == nil {
		formatted = []FormattedVersion{}
	}
	errMsg := job.ErrorMessage
	if errMsg == "" && job.ErrorCode != "" {
		errMsg = SafeErrorMessage(job.ErrorCode)
	}
	retryable := job.Status == "failed" && job.Attempts < 5
	resp := map[string]any{
		"id":                    job.ID,
		"jobId":                 job.ID,
		"status":                job.Status,
		"phase":                 PhaseLabel(job.Status),
		"progress":              PhaseProgress(job.Status),
		"message":               PhaseMessage(job.Status),
		"error_code":            job.ErrorCode,
		"errorCode":             job.ErrorCode,
		"retryable":             retryable,
		"segments":              segments,
		"transcript":            map[string]any{"segments": segments},
		"formatted_transcripts": formatted,
	}
	if errMsg != "" {
		resp["error_message"] = errMsg
	}
	return resp
}
