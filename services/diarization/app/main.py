from datetime import datetime, timezone
import hashlib
import hmac
import logging
import os
import tempfile
import urllib.parse
import urllib.request
import wave
from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel

logger = logging.getLogger("uvicorn.error")
app = FastAPI(title="Diarization Service")
_pipeline = None


class DiarizeRequest(BaseModel):
    audio_path: str = ""
    audio_key: str = ""


class Turn(BaseModel):
    start_ms: int
    end_ms: int
    speaker: str


def _secret_or_env(name: str, default: str = "") -> str:
    file_path = os.environ.get(f"{name}_FILE", "").strip()
    if file_path and os.path.isfile(file_path):
        with open(file_path, "r", encoding="utf-8") as handle:
            val = handle.read().strip()
            if val:
                return val
    return os.environ.get(name, default).strip()


def _fake_mode() -> bool:
    for key in ("FAKE_DIARIZATION_ENABLED", "FAKE_PIPELINE"):
        if os.environ.get(key, "").strip().lower() in ("true", "1", "yes"):
            return True
    token = _secret_or_env("HF_TOKEN")
    return token in ("fake", "test")


class _FakeSegment:
    def __init__(self, start: float, end: float):
        self.start = start
        self.end = end


class _FakeAnnotation:
    def __init__(self, tracks):
        self._tracks = tracks

    def itertracks(self, yield_label: bool = False):
        for start, end, speaker in self._tracks:
            if yield_label:
                yield _FakeSegment(start, end), None, speaker
            else:
                yield _FakeSegment(start, end), None


def _fake_pipeline(audio_path: str):
    if not os.path.isfile(audio_path):
        raise RuntimeError(f"audio file not found: {audio_path}")
    with wave.open(audio_path, "rb") as wav_file:
        if wav_file.getnchannels() != 1 or wav_file.getframerate() != 16000:
            raise RuntimeError("expected 16 kHz mono WAV")
    return _FakeAnnotation([
        (0.0, 2.0, "Speaker 1"),
        (2.0, 4.5, "Speaker 2"),
    ])


def pipeline():
    global _pipeline
    if _pipeline is not None:
        return _pipeline
    if _fake_mode():
        return _fake_pipeline
    token = _secret_or_env("HF_TOKEN")
    if not token:
        raise RuntimeError("HF_TOKEN is required")
    import torch
    if hasattr(torch.serialization, "add_safe_globals"):
        safe_globals = [torch.torch_version.TorchVersion]
        try:
            from pyannote.audio.core.task import Specifications, Problem, Resolution
            safe_globals.extend([Specifications, Problem, Resolution])
        except ImportError:
            pass
        torch.serialization.add_safe_globals(safe_globals)
    from pyannote.audio import Pipeline
    try:
        _pipeline = Pipeline.from_pretrained("pyannote/speaker-diarization-community-1", token=token)
    except TypeError:
        _pipeline = Pipeline.from_pretrained("pyannote/speaker-diarization-community-1", use_auth_token=token)
    return _pipeline


def fetch_from_object_store(audio_key: str) -> bytes:
    endpoint = os.environ.get("OBJECT_STORE_ENDPOINT", "").strip().rstrip("/")
    if not endpoint or not audio_key:
        raise RuntimeError("object store endpoint or key not configured")
    bucket = os.environ.get("OBJECT_STORE_BUCKET", "artifacts").strip() or "artifacts"
    access_key = _secret_or_env("OBJECT_STORE_ACCESS_KEY", "webutils")
    secret_key = _secret_or_env("OBJECT_STORE_SECRET_KEY", "change-me-in-production")
    region = "us-east-1"

    clean_key = audio_key.lstrip("/")
    uri_path = f"/{bucket}/{clean_key}"
    parsed = urllib.parse.urlparse(endpoint)
    host = parsed.netloc

    now = datetime.now(timezone.utc)
    amz_date = now.strftime("%Y%m%dT%H%M%SZ")
    date_stamp = now.strftime("%Y%m%d")
    payload_hash = hashlib.sha256(b"").hexdigest()

    canonical_headers = f"host:{host}\nx-amz-content-sha256:{payload_hash}\nx-amz-date:{amz_date}\n"
    signed_headers = "host;x-amz-content-sha256;x-amz-date"
    canonical_request = f"GET\n{uri_path}\n\n{canonical_headers}\n{signed_headers}\n{payload_hash}"

    credential_scope = f"{date_stamp}/{region}/s3/aws4_request"
    string_to_sign = (
        f"AWS4-HMAC-SHA256\n{amz_date}\n{credential_scope}\n"
        f"{hashlib.sha256(canonical_request.encode('utf-8')).hexdigest()}"
    )

    def _sign(key: bytes, msg: str) -> bytes:
        return hmac.new(key, msg.encode("utf-8"), hashlib.sha256).digest()

    k_date = _sign(("AWS4" + secret_key).encode("utf-8"), date_stamp)
    k_region = _sign(k_date, region)
    k_service = _sign(k_region, "s3")
    k_signing = _sign(k_service, "aws4_request")
    signature = hmac.new(k_signing, string_to_sign.encode("utf-8"), hashlib.sha256).hexdigest()

    auth_header = (
        f"AWS4-HMAC-SHA256 Credential={access_key}/{credential_scope}, "
        f"SignedHeaders={signed_headers}, Signature={signature}"
    )

    req = urllib.request.Request(
        f"{endpoint}{uri_path}",
        method="GET",
        headers={
            "Host": host,
            "X-Amz-Date": amz_date,
            "X-Amz-Content-Sha256": payload_hash,
            "Authorization": auth_header,
        },
    )
    with urllib.request.urlopen(req, timeout=15) as resp:
        data = resp.read()
    if len(data) < 44 or data[:4] != b"RIFF" or data[8:12] != b"WAVE":
        raise RuntimeError("downloaded object is not a valid WAV artifact")
    return data


@app.get("/healthz")
def health():
    return {"status": "ok"}


@app.get("/readyz")
def ready():
    return {"status": "ready"}


@app.post("/v1/diarize", response_model=list[Turn])
def diarize(request: DiarizeRequest, x_internal_token: str = Header(default="")):
    expected_token = _secret_or_env("INTERNAL_TOKEN")
    if not expected_token or x_internal_token != expected_token:
        raise HTTPException(401, "invalid internal token")

    temp_path = None
    target_path = request.audio_path
    try:
        if request.audio_key and os.environ.get("OBJECT_STORE_ENDPOINT", "").strip():
            try:
                wav_bytes = fetch_from_object_store(request.audio_key)
                fd, temp_path = tempfile.mkstemp(suffix=".wav")
                with os.fdopen(fd, "wb") as handle:
                    handle.write(wav_bytes)
                target_path = temp_path
            except Exception:
                if not target_path or not os.path.exists(target_path):
                    raise
        result = pipeline()(target_path)
    except Exception as error:
        logger.exception("diarization failed")
        raise HTTPException(503, "diarization unavailable") from error
    finally:
        if temp_path and os.path.exists(temp_path):
            try:
                os.remove(temp_path)
            except OSError:
                pass

    annotation = getattr(result, "speaker_diarization", result)
    return [
        Turn(start_ms=round(turn.start * 1000), end_ms=round(turn.end * 1000), speaker=speaker)
        for turn, _, speaker in annotation.itertracks(yield_label=True)
    ]
