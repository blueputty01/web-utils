import { transcriptionApi } from '$lib/api/client';
import type { TranscriptionJob } from '$lib/api/types';

const terminal = new Set(['completed', 'failed', 'cancelled']);

export function createJobStore(onUpdate: (job: TranscriptionJob) => void, onError: (message: string) => void) {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let stopped = false;
  const stop = () => { stopped = true; if (timer) clearTimeout(timer); };
  const poll = async (jobId: string, attempt = 0): Promise<void> => {
    try {
      const job = await transcriptionApi.get(jobId);
      onUpdate(job);
      if (!stopped && !terminal.has(job.status)) timer = setTimeout(() => void poll(jobId, 0), Math.min(5000, 800 + attempt * 400));
    } catch (error) {
      if (!stopped && attempt < 4) timer = setTimeout(() => void poll(jobId, attempt + 1), Math.min(5000, 800 * 2 ** attempt));
      else if (!stopped) onError(error instanceof Error ? error.message : 'Unable to update job progress.');
    }
  };
  return { start: (jobId: string) => { stop(); stopped = false; void poll(jobId); }, stop };
}
