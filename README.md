# Video Transcription Utilities

A containerized web utilities platform featuring speaker-attributed video transcription and non-destructive Gemini Flash transcript formatting.

## Quick Start

The application runs entirely with Docker Compose:

```bash
cp .env.example .env
# Optional: Set HF_TOKEN, GEMINI_API_KEY, and a long INTERNAL_TOKEN in .env
# By default, FAKE_PIPELINE=true enables deterministic local/CI execution without external model downloads.
docker compose up -d --build --wait
```

Open `http://localhost:3000/utilities/video-transcription`. The API listens on `http://localhost:8080`; only `frontend` and `api` publish host ports. PostgreSQL, `worker`, `diarization`, and `object-store` communicate exclusively over the internal `private` Compose network.

## Architecture & Object Store Usage

1. **Frontend (`frontend/`)**: SvelteKit SPA serving `/utilities/video-transcription` and proxying `/api/*` requests to the Go API over the internal network.
2. **API (`backend/cmd/api`)**: Validates public `http`/`https` URLs (blocking private, loopback, link-local, and metadata destinations), manages job state in PostgreSQL, enforces per-client rate limits, and invokes Gemini Flash (`GEMINI_API_KEY` or `GEMINI_API_KEY_FILE`) to store non-destructive formatted previews in `formatted_transcripts`.
3. **Worker (`backend/cmd/worker`)**: Claims queued jobs with `SELECT ... FOR UPDATE SKIP LOCKED`, resolves media with `yt-dlp`, normalizes audio to 16 kHz mono WAV with FFmpeg, verifies the RIFF/WAVE signature, uploads the normalized WAV to the private S3-compatible **`object-store`** (`jobs/<job-id>/audio.wav`), transcribes speech via `whisper.cpp` Go bindings, calls the internal `diarization` sidecar with the artifact key, merges speaker turns by maximum millisecond overlap, and deletes the temporary artifact (with background TTL and retention cleanup).
4. **Object Store (`object-store`)**: Private S3-compatible service backed by the `object-data` volume storing intermediate WAV artifacts with AWS Signature V4 authentication and TTL-based expiration (`ARTIFACT_TTL`, default `1h`).
5. **Diarization Sidecar (`services/diarization`)**: FastAPI service authenticated with `X-Internal-Token` that fetches the normalized WAV artifact from `object-store` (or shared `/work` volume) and returns millisecond-aligned speaker turns using `pyannote/speaker-diarization-3.1`.

## Models & GPU Profile

To run with live ML models instead of the deterministic development pipeline:

1. Download a Whisper GGML model (e.g., `ggml-base.en.bin`) into `./models/` and set `WHISPER_MODEL_PATH=/models/ggml-base.en.bin`:
   ```bash
   curl -L --fail -o models/ggml-base.en.bin \
     https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin
   ```
2. Supply `HF_TOKEN` (having accepted the conditions for `pyannote/speaker-diarization-community-1` on Hugging Face) and `GEMINI_API_KEY` (via `.env` or `*_FILE` secret mounts) and set `FAKE_PIPELINE=false`.
3. For NVIDIA GPU acceleration on diarization, run with `--profile gpu`.

## Operational Runbook & Observability

- **Liveness & Readiness**:
  - `GET http://localhost:8080/healthz` — API liveness and database ping.
  - `GET http://localhost:8080/readyz` — Verifies PostgreSQL and `object-store` bucket readiness.
  - `GET http://localhost:8080/metrics` — Prometheus text format (or JSON with `Accept: application/json`) counters for HTTP requests, rate-limited requests, job submissions/completions/failures/cancellations/retries, and formatted previews.
- **Verification Commands**:
  ```bash
  # Go unit tests
  docker run --rm -v "$PWD/backend:/src" -w /src golang:1.24-alpine go test -v ./...

  # Python diarization unit tests
  docker run --rm web-utils-diarization pytest -v

  # Frontend component & store unit tests
  docker run --rm web-utils-frontend npm test

  # End-to-end journey tests against running Compose stack
  docker compose exec -T frontend npm run test:e2e
  ```
