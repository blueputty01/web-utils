export type JobPhase = 'queued' | 'resolving' | 'downloading' | 'transcribing' | 'diarizing' | 'merging' | 'completed' | 'failed' | 'cancelled';

export interface TranscriptSegment { id?: string; start_ms: number; end_ms: number; speaker: string; text: string; }
export interface FormattedTranscript { id: string; preset: FormatPreset; content: string; created_at?: string; }
export type FormatPreset = 'meeting-notes' | 'clean-transcript' | 'summary';
export interface TranscriptionJob {
  id: string; status: JobPhase; phase?: string; progress?: number; message?: string; error_code?: string; error_message?: string;
  retryable?: boolean; segments?: TranscriptSegment[]; transcript?: { segments: TranscriptSegment[] }; formatted_transcripts?: FormattedTranscript[];
}
export interface SubmitJobRequest { source_url: string; idempotency_key: string; }
