CREATE TABLE utility_runs (
  id UUID PRIMARY KEY,
  idempotency_key TEXT NOT NULL UNIQUE,
  source_url TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE transcription_jobs (
  id UUID PRIMARY KEY,
  run_id UUID NOT NULL REFERENCES utility_runs(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('queued','resolving','downloading','transcribing','diarizing','merging','completed','failed','cancelled')),
  lease_expires_at TIMESTAMPTZ,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  attempts INTEGER NOT NULL DEFAULT 0,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX transcription_jobs_claim_idx ON transcription_jobs (status, next_attempt_at, lease_expires_at);

CREATE TABLE transcript_segments (
  id BIGSERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES transcription_jobs(id) ON DELETE CASCADE,
  start_ms INTEGER NOT NULL,
  end_ms INTEGER NOT NULL,
  text TEXT NOT NULL,
  speaker TEXT,
  CHECK (end_ms >= start_ms)
);
CREATE INDEX transcript_segments_job_idx ON transcript_segments (job_id, start_ms);

CREATE TABLE speaker_turns (
  id BIGSERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES transcription_jobs(id) ON DELETE CASCADE,
  start_ms INTEGER NOT NULL,
  end_ms INTEGER NOT NULL,
  speaker TEXT NOT NULL,
  CHECK (end_ms >= start_ms)
);
CREATE INDEX speaker_turns_job_idx ON speaker_turns (job_id, start_ms);

CREATE TABLE formatted_transcripts (
  id UUID PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES transcription_jobs(id) ON DELETE CASCADE,
  preset TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX formatted_transcripts_job_idx ON formatted_transcripts (job_id, created_at DESC);
