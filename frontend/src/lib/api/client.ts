import type { FormatPreset, FormattedTranscript, SubmitJobRequest, TranscriptionJob } from './types';

const baseUrl = (import.meta.env.PUBLIC_API_BASE_URL ?? '').replace(/\/$/, '');
const jobsPath = `${baseUrl}/api/v1/transcription/jobs`;

export class ApiError extends Error {
  constructor(message: string, readonly status?: number) { super(message); this.name = 'ApiError'; }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } });
  if (!response.ok) {
    const payload = await response.json().catch(() => null) as { message?: string; error?: string } | null;
    throw new ApiError(payload?.message ?? payload?.error ?? 'The request could not be completed.', response.status);
  }
  return response.json() as Promise<T>;
}

export const transcriptionApi = {
  submit: (body: SubmitJobRequest) => request<TranscriptionJob>(jobsPath, { method: 'POST', body: JSON.stringify(body), headers: { 'Idempotency-Key': body.idempotency_key } }),
  get: (id: string) => request<TranscriptionJob>(`${jobsPath}/${encodeURIComponent(id)}`),
  cancel: (id: string) => request<TranscriptionJob>(`${jobsPath}/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  retry: (id: string) => request<TranscriptionJob>(`${jobsPath}/${encodeURIComponent(id)}/retry`, { method: 'POST' }),
  format: (id: string, preset: FormatPreset) => request<FormattedTranscript>(`${jobsPath}/${encodeURIComponent(id)}/formatted-transcripts`, { method: 'POST', body: JSON.stringify({ preset }) })
};
