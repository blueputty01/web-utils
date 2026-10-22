<script lang="ts">
  import { transcriptionApi } from '$lib/api/client';
  import type { FormatPreset, TranscriptionJob } from '$lib/api/types';
  import FormattedPreview from '$lib/transcription/FormattedPreview.svelte';
  import TranscriptViewer from '$lib/transcription/TranscriptViewer.svelte';
  import { createJobStore } from '$lib/transcription/job-store';
  import { onDestroy } from 'svelte';

  let sourceUrl = $state('');
  let job = $state<TranscriptionJob | undefined>();
  let error = $state('');
  let submitError = $state('');
  let submitting = $state(false);
  let actionLoading = $state(false);
  let previewError = $state('');
  const store = createJobStore((next) => { job = next; }, (message) => { error = message; });
  onDestroy(store.stop);

  const isHttpUrl = (value: string) => { try { const url = new URL(value); return url.protocol === 'http:' || url.protocol === 'https:'; } catch { return false; } };
  const idempotencyKey = () => crypto.randomUUID?.() ?? `${Date.now()}-${Math.random()}`;
  const segments = () => job?.segments ?? job?.transcript?.segments ?? [];

  async function submit() {
    submitError = ''; error = '';
    if (!isHttpUrl(sourceUrl)) { submitError = 'Enter a valid http or https media URL.'; return; }
    submitting = true;
    try { job = await transcriptionApi.submit({ source_url: sourceUrl, idempotency_key: idempotencyKey() }); store.start(job.id); }
    catch (cause) { submitError = cause instanceof Error ? cause.message : 'Unable to start transcription.'; }
    finally { submitting = false; }
  }
  async function cancel() { if (!job) return; actionLoading = true; error = ''; try { job = await transcriptionApi.cancel(job.id); store.stop(); } catch (cause) { error = cause instanceof Error ? cause.message : 'Unable to cancel this job.'; } finally { actionLoading = false; } }
  async function retry() { if (!job) return; actionLoading = true; error = ''; try { job = await transcriptionApi.retry(job.id); store.start(job.id); } catch (cause) { error = cause instanceof Error ? cause.message : 'Unable to retry this job.'; } finally { actionLoading = false; } }
  async function format(preset: FormatPreset) { if (!job) return; previewError = ''; actionLoading = true; try { const preview = await transcriptionApi.format(job.id, preset); job = { ...job, formatted_transcripts: [...(job.formatted_transcripts ?? []), preview] }; } catch (cause) { previewError = cause instanceof Error ? cause.message : 'Unable to format the transcript.'; } finally { actionLoading = false; } }
</script>

<svelte:head><title>Video transcription | Web Utilities</title><meta name="description" content="Create speaker-labelled video transcripts." /></svelte:head>
<main>
  <header><p class="eyebrow">Web utilities</p><h1>Video transcription</h1><p>Submit media you have the right to process. Progress and transcripts are retained by the service.</p></header>
  <section aria-labelledby="submit-heading" class="card"><h2 id="submit-heading">Transcribe a video</h2><form onsubmit={(event) => { event.preventDefault(); void submit(); }}><label for="source-url">Video or supported platform URL</label><div class="form-row"><input id="source-url" name="source-url" type="url" bind:value={sourceUrl} required placeholder="https://example.com/video.mp4" aria-describedby="source-help submit-error" /><button type="submit" disabled={submitting}>{submitting ? 'Submitting…' : 'Start transcription'}</button></div><p id="source-help" class="hint">Only http and https URLs are accepted.</p>{#if submitError}<p id="submit-error" class="error" role="alert">{submitError}</p>{/if}</form></section>
  {#if job}<section class="card" aria-labelledby="progress-heading"><h2 id="progress-heading">Job progress</h2><p aria-live="polite"><strong>{job.status}</strong>{#if job.phase} — {job.phase}{/if}{#if job.progress !== undefined} ({job.progress}%){/if}</p>{#if job.message}<p>{job.message}</p>{/if}{#if job.status === 'failed'}<p class="error" role="alert">{job.error_message ?? 'Processing failed safely. You can retry this job.'}</p>{/if}{#if error}<p class="error" role="alert">{error}</p>{/if}<div class="actions">{#if !['completed', 'failed', 'cancelled'].includes(job.status)}<button class="secondary" onclick={cancel} disabled={actionLoading}>Cancel</button>{/if}{#if job.status === 'failed' && job.retryable !== false}<button onclick={retry} disabled={actionLoading}>Retry</button>{/if}</div></section>
  <section class="card"><TranscriptViewer segments={segments()} loading={!['completed', 'failed', 'cancelled'].includes(job.status)} /></section>
  <section class="card"><FormattedPreview available={job.status === 'completed'} preview={job.formatted_transcripts?.at(-1)} loading={actionLoading} error={previewError} onFormat={format} /></section>{/if}
</main>
<style>:global(body){margin:0;background:#f5f7fb;color:#172033;font-family:system-ui,-apple-system,sans-serif}main{max-width:920px;margin:auto;padding:3rem 1.25rem}.eyebrow{text-transform:uppercase;font-size:.8rem;letter-spacing:.1em;color:#2563eb;font-weight:700}.card{background:white;border:1px solid #dce3ef;border-radius:.6rem;padding:1.25rem;margin-top:1.25rem;box-shadow:0 1px 2px #1720330d}.form-row{display:flex;gap:.75rem;margin-top:.5rem}input,button{font:inherit}input{min-width:0;flex:1;padding:.65rem;border:1px solid #94a3b8;border-radius:.3rem}button{background:#2563eb;color:#fff;border:0;border-radius:.3rem;padding:.65rem .9rem;cursor:pointer}button:disabled{opacity:.6;cursor:not-allowed}.secondary{background:#475569}.actions{display:flex;gap:.75rem}.hint{color:#475569;font-size:.9rem}.error{color:#b42318}@media(max-width:600px){main{padding-top:1.5rem}.form-row{flex-direction:column}}</style>
