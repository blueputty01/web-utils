import http.server
import os
import socketserver
import struct
import threading
import wave
from fastapi.testclient import TestClient
import pytest

from app import main


def _write_wav(path: str):
    with wave.open(path, "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(16000)
        wav_file.writeframes(struct.pack("<" + "h" * 320, *([0] * 320)))


def test_healthz():
    client = TestClient(main.app)
    res = client.get("/healthz")
    assert res.status_code == 200
    assert res.json() == {"status": "ok"}


def test_diarize_rejects_invalid_token(monkeypatch):
    monkeypatch.setenv("INTERNAL_TOKEN", "expected-secret")
    client = TestClient(main.app)
    res = client.post("/v1/diarize", json={"audio_path": "/tmp/a.wav"}, headers={"X-Internal-Token": "wrong"})
    assert res.status_code == 401


def test_diarize_normalizes_speaker_turns(monkeypatch, tmp_path):
    monkeypatch.setenv("INTERNAL_TOKEN", "expected-secret")
    monkeypatch.setenv("FAKE_DIARIZATION_ENABLED", "true")
    main._pipeline = None

    wav_path = tmp_path / "sample.wav"
    _write_wav(str(wav_path))

    client = TestClient(main.app)
    res = client.post(
        "/v1/diarize",
        json={"audio_path": str(wav_path)},
        headers={"X-Internal-Token": "expected-secret"},
    )
    assert res.status_code == 200
    turns = res.json()
    assert len(turns) == 2
    assert turns[0] == {"start_ms": 0, "end_ms": 2000, "speaker": "Speaker 1"}
    assert turns[1] == {"start_ms": 2000, "end_ms": 4500, "speaker": "Speaker 2"}


def test_diarize_fetches_artifact_from_object_store(monkeypatch, tmp_path):
    wav_path = tmp_path / "object.wav"
    _write_wav(str(wav_path))
    wav_bytes = wav_path.read_bytes()

    received_headers = {}

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            received_headers["Authorization"] = self.headers.get("Authorization", "")
            received_headers["Path"] = self.path
            self.send_response(200)
            self.send_header("Content-Type", "audio/wav")
            self.end_headers()
            self.wfile.write(wav_bytes)

        def log_message(self, format, *args):
            return

    with socketserver.TCPServer(("127.0.0.1", 0), Handler) as httpd:
        port = httpd.server_address[1]
        thread = threading.Thread(target=httpd.serve_forever, daemon=True)
        thread.start()
        try:
            monkeypatch.setenv("INTERNAL_TOKEN", "expected-secret")
            monkeypatch.setenv("FAKE_DIARIZATION_ENABLED", "true")
            monkeypatch.setenv("OBJECT_STORE_ENDPOINT", f"http://127.0.0.1:{port}")
            monkeypatch.setenv("OBJECT_STORE_BUCKET", "artifacts")
            main._pipeline = None

            client = TestClient(main.app)
            res = client.post(
                "/v1/diarize",
                json={"audio_key": "jobs/job-99/audio.wav"},
                headers={"X-Internal-Token": "expected-secret"},
            )
            assert res.status_code == 200
            assert received_headers["Path"] == "/artifacts/jobs/job-99/audio.wav"
            assert received_headers["Authorization"].startswith("AWS4-HMAC-SHA256 ")
            assert len(res.json()) == 2
        finally:
            httpd.shutdown()
