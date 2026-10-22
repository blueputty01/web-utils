# Implementation Plan: Video Transcription Utilities

**Date**: 2026-10-18 | **Spec**: User-provided MVP requirements

## Summary

Build a Go API and Svelte SPA for a multi-utility platform. The first utility accepts a direct media URL or a supported platform page URL, resolves and downloads audio with `yt-dlp` and FFmpeg, transcribes it locally through `github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper`, and aligns the transcript with speaker turns from a Python `pyannote.audio` diarization sidecar. A PostgreSQL-backed worker provides durable asynchronous jobs. Once a completed transcript is displayed, the user can request an editable Gemini Flash formatted preview while retaining the original transcript.

## Technical Context

**Language/Version**: Go 1.24+ API/worker; Python 3.11+ diarization service; TypeScript 5+ / Svelte 5 frontend.

**Primary Dependencies**: Go `chi`, `pgx`, `whisper`, structured logging, `yt-dlp`, FFmpeg; Python `FastAPI`, `pyannote.audio`, PyTorch; SvelteKit, `@sveltejs/adapter-node`, fetch; Gemini API Go SDK.

**Storage**: PostgreSQL 16+ for jobs, utility runs, original transcript segments, diarization turns, formatted transcript versions, and idempotency records. Audio and intermediate artifacts reside in a private object store with TTL-based deletion.

**Testing**: Go unit/integration tests, Python unit/API contract tests, Svelte component tests, and Playwright journey tests. Media, Whisper, and Gemini calls are replaced with deterministic fakes outside an opt-in integration environment.

**Target Platform**: A Docker Compose deployment is the sole runtime topology for local, CI, and initial production deployment. Compose services are `frontend`, `api`, `worker`, `diarization`, `postgres`, and an S3-compatible private object-store emulator; each application service has its own production Dockerfile. `yt-dlp` and FFmpeg are installed only in the worker image; Whisper and diarization model caches use named read-only-at-runtime volumes. GPU is an optional Compose profile with an NVIDIA device reservation; the default CPU profile has explicit resource limits.

**Performance Goals**: Return a job ID within 2 seconds; expose live job state; retain a partial failure diagnostic; target processing completion within 1.5× media duration on a baseline CPU deployment, subject to source and model size.

**Constraints**: Never expose Gemini or Hugging Face tokens to the browser. All services run as non-root containers with read-only filesystems except declared writable mounts; only `frontend` and `api` publish ports, and `worker`, `diarization`, PostgreSQL, and object storage communicate on internal Compose networks. Inject secrets through Docker Compose secrets or the deployment secret store, never image layers or committed `.env` files. Allow only `http`/`https` URLs; block private, loopback, link-local, and metadata-network destinations after DNS resolution; enforce source allowlists, redirect validation, duration/size limits, concurrency quotas, and artifact TTLs. Users must have rights to process submitted material.

### Repository Layout Rationale

`backend/` is one Go codebase, not only the HTTP API: it contains two closely related executables, `cmd/api` and `cmd/worker`, plus their shared domain, repository, media, and transcription packages in `internal/`. Keeping them together prevents duplicating Go types and database logic. `services/diarization/` is separate because it is an independently built Python/PyTorch container with a different package manager, model cache, and runtime lifecycle. This separation follows language/runtime ownership rather than an API-versus-service boundary. The Svelte application remains in `frontend/` because it is a separately built Node container.

### PostgreSQL Job Model

PostgreSQL is the durable source of truth; it does not process media itself. The API inserts one `transcription_jobs` row after validating a submission. Its `status` begins as `queued`; the worker atomically claims jobs with `SELECT ... FOR UPDATE SKIP LOCKED`, sets a lease and phase, and commits before invoking external tools. `SKIP LOCKED` lets multiple worker containers safely claim different rows without waiting on one another. On success or failure, the worker writes the resulting transcript/speaker records and transitions the same job row in a transaction. A lease expiry lets another worker reclaim work after a worker crash.

`utility_runs` owns a submission and its idempotency key, so a repeated browser request returns the existing job rather than starting duplicate downloads. `transcript_segments` stores immutable Whisper text/time ranges; `speaker_turns` stores pyannote ranges; the merge records speaker attribution without overwriting source timings. `formatted_transcripts` stores a separate Gemini result linked to the source transcript, so edits never replace the original. Foreign keys enforce ownership and cleanup order; unique indexes protect idempotency; indexes on `(status, next_attempt_at, lease_expires_at)` support worker claiming; and indexes on each job foreign key support status/transcript reads. A scheduled cleanup query removes expired object references and then dependent SQL rows in foreign-key order.

## Constitution Check

No project constitution was supplied. The plan applies these release gates:

- **Least privilege — PASS**: server-only secrets; private object storage; restricted worker network egress.
- **Durability — PASS**: PostgreSQL job state and idempotency survive restarts.
- **Privacy — PASS**: artifacts have a defined TTL and are never published by URL.
- **Observable failure — PASS**: job events include phase, safe error code, and retry status.
- **Progressive delivery — PASS**: the utility registry permits later tools without coupling them to transcription.

## Applied Guidelines

No repository-specific or migration guidelines were available. The implementation uses a bounded asynchronous worker pipeline, typed API contracts shared through generated TypeScript types/OpenAPI, cancellation-aware subprocess execution, and contract tests between Go and the Python sidecar.

## Implementation Steps

### Phase 1: Setup

#### Step 1.1: [Cross-cutting] Create the multi-utility application foundation
- **Requirements**: REQ-001, REQ-008
- **Design inputs**: Greenfield architecture; utility registry pattern.
- **Description**: Scaffold Go API and worker commands, SvelteKit frontend, and Python sidecar as independently built containers. Define Docker Compose as the required runtime for development, test, and deployment, including service dependencies, profiles, persistent volumes, internal network boundaries, health checks, and startup sequencing. Add PostgreSQL migrations, configuration validation, and a utility registry that routes `/utilities/video-transcription` without assuming it is the only utility.

- [x] T001 [Plan:1.1] Create Go module and `cmd/api`, `cmd/worker`, and `internal/` package layout in `backend/`.
- [x] T002 [P] [Plan:1.1] Scaffold the SvelteKit application and transcription utility route in `frontend/`.
- [x] T003 [P] [Plan:1.1] Create FastAPI service scaffold and pinned Python environment in `services/diarization/`.
- [x] T004 [Plan:1.1] Define the complete Compose topology—`frontend`, `api`, `worker`, `diarization`, `postgres`, and private object storage—with named volumes, internal/public networks, health checks, dependency conditions, CPU/GPU profiles, and secret mounts in `compose.yaml`, `compose.override.yaml`, and `.env.example`.
- [x] T005 [Plan:1.1] Add configuration parsing, startup validation, structured logging, and readiness endpoints in `backend/internal/config/` and `backend/internal/http/`.

### Phase 2: Foundational pipeline and persistence

#### Step 2.1: [Cross-cutting] Persist transcription jobs and pipeline state
- **Requirements**: REQ-002, REQ-003, REQ-007, REQ-008
- **Design inputs**: PostgreSQL durable queue and object-store TTL policy.
- **Description**: Define job ownership/idempotency, submission metadata, phase transitions (`queued`, `resolving`, `downloading`, `transcribing`, `diarizing`, `merging`, `completed`, `failed`, `cancelled`), retry controls, versioned transcript records, and cleanup scheduling. Implement the PostgreSQL job queue with short claim transactions: workers use `FOR UPDATE SKIP LOCKED`, write a lease expiry, commit before media/model work, and atomically persist completion/failure afterward. This avoids holding database locks during long-running transcription while enabling crash recovery.

- [x] T006 [Plan:2.1] Add PostgreSQL migrations for `utility_runs`, `transcription_jobs`, `transcript_segments`, `speaker_turns`, `formatted_transcripts`, and idempotency keys in `backend/migrations/`.
- [x] T007 [Plan:2.1] Implement typed repositories, transactional state transitions, job claiming with `FOR UPDATE SKIP LOCKED`, and retry eligibility in `backend/internal/transcription/repository/`.
- [x] T008 [Plan:2.1] Implement authenticated job submit/status/cancel HTTP handlers and OpenAPI contract in `backend/internal/transcription/http/` and `api/openapi.yaml`.
- [x] T009 [Plan:2.1] Implement event publication/polling response DTOs and bounded retention cleanup in `backend/internal/transcription/service/`.

#### Step 2.2: [Cross-cutting] Build a safe media acquisition service
- **Requirements**: REQ-001, REQ-002, REQ-008
- **Design inputs**: Platform-plus-direct source decision; FFmpeg/yt-dlp process model.
- **Description**: Validate URLs and each redirect/DNS result, accept direct media links or configured platform page URLs, resolve platform media through `yt-dlp`, download with resource-limited subprocesses, convert to mono 16 kHz WAV with FFmpeg, and store ephemeral private inputs.

- [x] T010 [Plan:2.2] Implement URL normalization, source allowlists, DNS/IP address denial rules, redirect revalidation, and request limits in `backend/internal/media/validate.go`.
- [x] T011 [Plan:2.2] Implement cancellation-aware `yt-dlp` source resolution and FFmpeg audio extraction wrappers with time, byte, and duration limits in `backend/internal/media/`.
- [x] T012 [Plan:2.2] Implement private artifact storage, content-type verification, and TTL cleanup interfaces in `backend/internal/storage/`.
- [x] T013 [Plan:2.2] Add deterministic fake process runners and unit tests for source validation and command argument construction in `backend/internal/media/`.

### Phase 3: User story 1 — submit and receive speaker-attributed transcript

#### Step 3.1: [Cross-cutting] Transcribe and diarize completed audio
- **Requirements**: REQ-002, REQ-003, REQ-004, REQ-007
- **Design inputs**: `whisper.cpp` Go bindings and `pyannote.audio` sidecar contract.
- **Description**: Run Whisper on normalized WAV input, call the internal diarization service with the same audio, merge time ranges into speaker-attributed segments using a documented overlap algorithm, preserve original text and timecodes, and surface phase-specific safe failures.

- [x] T014 [US1] [Plan:3.1] Implement lazy Whisper model loading, configurable model selection, segment extraction, and cancellation handling using `github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper` in `backend/internal/transcription/whisper.go`.
- [x] T015 [US1] [Plan:3.1] Define the authenticated internal diarization request/response contract in `api/diarization.openapi.yaml`.
- [x] T016 [US1] [Plan:3.1] Implement pyannote pipeline initialization from a server-side Hugging Face token, audio diarization endpoint, and normalized speaker-turn output in `services/diarization/app/`.
- [x] T017 [US1] [Plan:3.1] Implement Go diarization client timeouts, retries, correlation IDs, and contract decoding in `backend/internal/diarization/`.
- [x] T018 [US1] [Plan:3.1] Implement deterministic overlap-based segment-to-speaker merging, unknown-speaker fallback, and persistence in `backend/internal/transcription/merge.go`.
- [x] T019 [US1] [Plan:3.1] Orchestrate claimed jobs through acquisition, Whisper, diarization, merge, persistence, cleanup, and terminal state handling in `backend/internal/transcription/worker.go`.

#### Step 3.2: [Cross-cutting] Present submission progress and original transcript
- **Requirements**: REQ-001, REQ-004, REQ-007
- **Design inputs**: Job status API and versioned transcript model.
- **Description**: Provide an accessible URL form, progress/error UI, cancellation/retry action, and timestamped, speaker-labelled transcript view. The reformat action is unavailable until the original transcript is complete.

- [x] T020 [US1] [Plan:3.2] Generate TypeScript API types from `api/openapi.yaml` into `frontend/src/lib/api/`.
- [x] T021 [US1] [Plan:3.2] Implement URL submission, validation feedback, and idempotency-key handling in `frontend/src/routes/utilities/video-transcription/+page.svelte`.
- [x] T022 [US1] [Plan:3.2] Implement bounded status polling, cancellation/retry controls, and safe failure rendering in `frontend/src/lib/transcription/job-store.ts`.
- [x] T023 [US1] [Plan:3.2] Implement accessible timestamped speaker segment rendering and transcript loading states in `frontend/src/lib/transcription/TranscriptViewer.svelte`.

### Phase 4: User story 2 — create an editable Gemini-formatted preview

#### Step 4.1: [Cross-cutting] Generate and preserve formatted transcript versions
- **Requirements**: REQ-005, REQ-006, REQ-007, REQ-008
- **Design inputs**: Editable-preview decision; Gemini Flash server-side integration.
- **Description**: Submit completed original transcript text and an explicit formatting instruction to Gemini Flash from the backend. Store prompt metadata and generated result as a separate version; never overwrite the original transcription or speaker/time data.

- [x] T024 [US2] [Plan:4.1] Add formatted-transcript request and response endpoints, explicit format presets, and schema validation to `api/openapi.yaml` and `backend/internal/format/http/`.
- [x] T025 [US2] [Plan:4.1] Implement server-side Gemini Flash client with secret loading, request timeout, retry classification, quota-safe error mapping, and request-size guardrails in `backend/internal/gemini/client.go`.
- [x] T026 [US2] [Plan:4.1] Implement formatting service that builds a non-destructive prompt from transcript segments, persists output versions, and links them to the source transcript in `backend/internal/format/service.go`.
- [x] T027 [US2] [Plan:4.1] Implement reformat button visibility, preset selection, progress/error states, and editable formatted-preview panel in `frontend/src/lib/transcription/FormattedPreview.svelte`.
- [x] T028 [US2] [Plan:4.1] Integrate formatted previews alongside the immutable original transcript in `frontend/src/routes/utilities/video-transcription/+page.svelte`.

### Phase 5: Cross-cutting quality and delivery

#### Step 5.1: [Cross-cutting] Validate critical flows, operations, and deployment
- **Requirements**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008
- **Design inputs**: Containerized deployment; fake external dependencies; privacy/security constraints.
- **Description**: Add automated coverage for the pipeline and browser journey, instrument operational metrics, document model/secrets setup, and deploy runtime checks that prevent a ready state without required models and credentials.

- [x] T029 [Plan:5.1] Add Go unit tests for state transitions, speaker merging, URL safety, and Gemini version preservation under `backend/internal/**/*_test.go`.
- [x] T030 [P] [Plan:5.1] Add Python tests for diarization response normalization and service authentication in `services/diarization/tests/`.
- [x] T031 [P] [Plan:5.1] Add Svelte component tests for form, transcript, and formatted preview states in `frontend/src/**/*.test.ts`.
- [x] T032 [Plan:5.1] Add Playwright journey tests using fake worker/Gemini results in `frontend/e2e/video-transcription.spec.ts`.
- [x] T033 [Plan:5.1] Add metrics, traces, health/readiness checks, rate limiting, and a runbook in `backend/internal/observability/` and `README.md`.
- [x] T034 [Plan:5.1] Build hardened multi-stage images for every application service; enforce non-root/read-only execution, Compose secret mounts, model-cache and artifact-volume permissions, internal network isolation, and CI `docker compose config`, build, health, and smoke validation in `backend/Dockerfile`, `services/diarization/Dockerfile`, `frontend/Dockerfile`, `compose.yaml`, and `.github/workflows/ci.yml`.

## Project Structure

```text
api/
  openapi.yaml
  diarization.openapi.yaml
backend/
  cmd/api/
  cmd/worker/
  internal/{config,diarization,format,http,media,observability,storage,transcription}/
  migrations/
frontend/
  src/lib/{api,transcription}/
  src/routes/utilities/video-transcription/
  e2e/
services/diarization/
  app/
  tests/
compose.yaml
compose.override.yaml
infra/compose/{frontend,api,worker,diarization}/
.env.example
README.md
```

## Testing Strategy

- **appType**: SPA with API and asynchronous media worker.
- **Critical user journeys**: submit valid URL; reject unsafe/unsupported URL; observe job completion and speaker-labelled transcript; see recoverable processing failure; generate and edit a Gemini-formatted preview while original remains visible.
- **primaryValidationStack**: Go tests + pytest contract tests + Svelte tests + Playwright against Compose with fake model providers.
- **fallbackMatrix**:
  - `infra-tier`: Docker Compose PostgreSQL/object-store emulator → testcontainers or repository fakes. Prerequisite: Docker.
  - `browser-tier`: Playwright → Svelte component tests. Prerequisite: Node.js and browser binaries.
- **Environment requirements**: Docker Compose v2 is required; Go, Node.js, Python, FFmpeg, yt-dlp, and PostgreSQL run inside their respective containers. Optional NVIDIA Container Toolkit enables the GPU Compose profile. Server-held Hugging Face and Gemini keys are injected as secrets for opt-in live tests.
- **knownGaps**: Fake model tests cannot measure transcription/diarization quality or Gemini output quality; an opt-in staging suite must exercise licensed models on representative consented media.
- **Test data strategy**: Use short, repository-owned or synthetic audio fixtures with known speaker boundaries. Create per-test job IDs and delete objects/database records after each integration test.
- **Acceptance criteria**: All critical journeys pass; no secrets appear in browser requests or logs; original transcript remains unchanged after formatted-preview generation; invalid destinations never reach download subprocesses.
- **Validation review expectations**: Confirm model licensing/access, source platform policy, rate limits, artifact deletion, worker restart recovery, and staging quality evaluation.

## Requirement Mapping

| REQ ID | Description | Plan Items | Implementation Evidence |
|---|---|---|---|
| REQ-001 | Submit direct or supported platform video URL. | 1.1, 2.2, 3.2, 5.1 | `media/validate.go`, `media/`, video-transcription page |
| REQ-002 | Download and normalize media asynchronously with FFmpeg. | 2.1, 2.2, 3.1, 5.1 | migrations, worker, `media/` |
| REQ-003 | Transcribe normalized audio with Whisper Go bindings. | 2.1, 3.1, 5.1 | `transcription/whisper.go`, worker |
| REQ-004 | Display timestamped speaker-attributed original transcript. | 3.1, 3.2, 5.1 | merge service, `TranscriptViewer.svelte` |
| REQ-005 | Make reformat available after original transcript completion. | 3.2, 4.1, 5.1 | page route, `FormattedPreview.svelte` |
| REQ-006 | Use Gemini Flash to produce editable preview without replacing original. | 4.1, 5.1 | `gemini/client.go`, `format/service.go` |
| REQ-007 | Report durable progress, cancellation, retries, and safe failures. | 2.1, 3.1, 3.2, 4.1, 5.1 | repositories, worker, job store |
| REQ-008 | Support future utilities with secure, observable operation. | 1.1, 2.1, 2.2, 4.1, 5.1 | utility registry, config, observability, runbook |
