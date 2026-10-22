import { describe, expect, it, vi } from 'vitest';
import { createJobStore } from './job-store';
import { transcriptionApi } from '$lib/api/client';

describe('createJobStore', () => {
  it('polls until job reaches a terminal state', async () => {
    vi.useFakeTimers();
    const getSpy = vi.spyOn(transcriptionApi, 'get')
      .mockResolvedValueOnce({ id: 'job-1', status: 'downloading', phase: 'Downloading', progress: 35 })
      .mockResolvedValueOnce({
        id: 'job-1',
        status: 'completed',
        phase: 'Completed',
        progress: 100,
        segments: [{ start_ms: 0, end_ms: 1000, speaker: 'Speaker 1', text: 'Done' }]
      });

    const updates: string[] = [];
    const errors: string[] = [];
    const store = createJobStore((job) => updates.push(job.status), (msg) => errors.push(msg));

    store.start('job-1');
    await vi.runAllTimersAsync();

    expect(updates).toEqual(['downloading', 'completed']);
    expect(errors).toEqual([]);
    getSpy.mockRestore();
    vi.useRealTimers();
  });
});
