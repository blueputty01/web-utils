# Resolved Planning Decisions

## Video source handling

- **Decision**: Support direct media URLs and configured platform page URLs. Resolve platform pages with `yt-dlp`, then normalize downloaded media using FFmpeg.
- **Rationale**: Matches the requested platform-plus-direct scope while keeping FFmpeg responsible for deterministic audio extraction.
- **Alternatives considered**: Direct URLs only; rejected because it excludes requested platform pages.

## Speaker diarization

- **Decision**: Run `pyannote.audio` in a private Python sidecar and call it from the Go worker over an authenticated internal API.
- **Rationale**: Pyannote is Python-native and its model/runtime lifecycle should not be embedded in the Go API process. The sidecar isolates GPU/PyTorch dependencies and allows independent scaling.
- **Alternatives considered**: Hosted diarization API; rejected because no provider was selected. Deferring diarization; rejected because diarization is required.

## Job durability

- **Decision**: Use PostgreSQL as the durable job queue/state store, with workers claiming jobs using row locking.
- **Rationale**: Matches the requested deployment-ready, multi-instance path and persists state across restarts without a second queue system in the MVP.
- **Alternatives considered**: In-memory jobs and SQLite; rejected because they do not meet the selected durability/scalability target.

## Gemini output

- **Decision**: Render Gemini Flash output as an editable preview stored as a separate version; preserve original Whisper/diarization segments.
- **Rationale**: Matches the requested behavior and keeps traceable source evidence.
- **Alternatives considered**: Replacing the displayed transcript; rejected because it loses the original transcription view.

## Containerized runtime

- **Decision**: Docker Compose is the required runtime for the complete system: `frontend`, `api`, `worker`, `diarization`, PostgreSQL, and private object storage. Individual host-language installs are not required outside container image builds.
- **Rationale**: The project combines Go, Node, Python/PyTorch, FFmpeg, yt-dlp, and model caches. Compose provides a reproducible integration topology, explicit service isolation, health-gated startup, and a single workflow for development and CI.
- **Alternatives considered**: Running services directly on developer hosts; rejected because it makes native media/model dependencies inconsistent. Kubernetes; deferred because it is unnecessary for the initial deployment topology.

## Operational assumptions

- Platform support is controlled by an explicit allowlist and each enabled source must be reviewed for terms, access controls, and rights to process submitted media.
- Docker Compose secrets or the deployment secret store supplies server-only `GEMINI_API_KEY` and Hugging Face credentials; they are never copied into images or committed `.env` files.
- Only `frontend` and `api` expose host ports. Workers, diarization, PostgreSQL, and object storage use private Compose networks. Model caches, PostgreSQL data, and transient artifacts use named volumes with lifecycle controls.
- The initial transcript is English-oriented unless model/language selection is added as a later requirement.
- PostgreSQL stores durable job state and transcript metadata only; downloaded media and model caches stay in private Compose volumes/object storage. Workers claim queued jobs with short `FOR UPDATE SKIP LOCKED` transactions and leases, then process media outside the transaction so multiple worker containers can operate concurrently.
