import assert from 'node:assert/strict';

const baseUrl = (process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:3000').replace(/\/$/, '');

async function run() {
  // 1. Verify frontend SSR page renders at /utilities/video-transcription
  const pageRes = await fetch(`${baseUrl}/utilities/video-transcription`);
  assert.equal(pageRes.status, 200, 'Expected /utilities/video-transcription to return 200');
  const html = await pageRes.text();
  assert.match(html, /Video transcription/);
  assert.match(html, /Video or supported platform URL/);

  // 2. Journey: Reject unsafe private/loopback URL
  const unsafeRes = await fetch(`${baseUrl}/api/v1/transcription/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': `e2e-unsafe-${Date.now()}` },
    body: JSON.stringify({ source_url: 'http://127.0.0.1/private.mp4', idempotency_key: `e2e-unsafe-${Date.now()}` })
  });
  assert.equal(unsafeRes.status, 400, 'Unsafe loopback URL must be rejected with 400');

  // 3. Journey: Submit valid URL, poll until completion, verify speaker-attributed segments
  const validKey = `e2e-valid-${Date.now()}`;
  const submitRes = await fetch(`${baseUrl}/api/v1/transcription/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': validKey },
    body: JSON.stringify({ source_url: 'https://example.com/interview.mp4', idempotency_key: validKey })
  });
  assert.equal(submitRes.status, 202, 'Valid URL submission must return 202');
  let job = await submitRes.json();
  assert.ok(job.id, 'Submitted job must return id');

  for (let i = 0; i < 30 && !['completed', 'failed', 'cancelled'].includes(job.status); i++) {
    await new Promise((r) => setTimeout(r, 300));
    const pollRes = await fetch(`${baseUrl}/api/v1/transcription/jobs/${job.id}`);
    assert.equal(pollRes.status, 200);
    job = await pollRes.json();
  }
  assert.equal(job.status, 'completed', `Expected completed job, got ${job.status}`);
  assert.ok(Array.isArray(job.segments) && job.segments.length >= 2, 'Expected speaker-attributed transcript segments');
  assert.equal(job.segments[0].speaker, 'Speaker 1');
  assert.equal(job.segments[1].speaker, 'Speaker 2');
  const originalSegmentsSnapshot = JSON.stringify(job.segments);

  // 4. Journey: Generate Gemini-formatted preview without altering original transcript
  const formatRes = await fetch(`${baseUrl}/api/v1/transcription/jobs/${job.id}/formatted-transcripts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ preset: 'meeting-notes' })
  });
  assert.equal(formatRes.status, 202, 'Formatted preview creation must return 202');
  const preview = await formatRes.json();
  assert.match(preview.content, /Meeting Notes/);

  const afterFormatRes = await fetch(`${baseUrl}/api/v1/transcription/jobs/${job.id}`);
  const jobAfterFormat = await afterFormatRes.json();
  assert.equal(JSON.stringify(jobAfterFormat.segments), originalSegmentsSnapshot, 'Original transcript segments must remain unchanged');
  assert.equal(jobAfterFormat.formatted_transcripts.length, 1);

  // 5. Journey: Recoverable failure and retry
  const failKey = `e2e-fail-${Date.now()}`;
  const failSubmitRes = await fetch(`${baseUrl}/api/v1/transcription/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': failKey },
    body: JSON.stringify({ source_url: 'https://example.com/fail-media.mp4', idempotency_key: failKey })
  });
  let failJob = await failSubmitRes.json();
  for (let i = 0; i < 30 && failJob.status !== 'failed'; i++) {
    await new Promise((r) => setTimeout(r, 300));
    const pollRes = await fetch(`${baseUrl}/api/v1/transcription/jobs/${failJob.id}`);
    failJob = await pollRes.json();
  }
  assert.equal(failJob.status, 'failed');
  assert.equal(failJob.retryable, true);

  const retryRes = await fetch(`${baseUrl}/api/v1/transcription/jobs/${failJob.id}/retry`, { method: 'POST' });
  assert.equal(retryRes.status, 200);

  console.log('All E2E user journeys passed against live Compose stack!');
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
