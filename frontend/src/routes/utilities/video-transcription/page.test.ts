import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';
import { transcriptionApi } from '$lib/api/client';

describe('Video transcription page', () => {
  it('rejects non-http URLs on client validation', async () => {
    render(Page);
    const input = screen.getByLabelText(/Video or supported platform URL/i);
    await fireEvent.input(input, { target: { value: 'ftp://example.com/video.mp4' } });
    const form = input.closest('form')!;
    await fireEvent.submit(form);
    expect(await screen.findByRole('alert')).toHaveTextContent(/Enter a valid http or https media URL/i);
  });

  it('submits a valid URL, displays speaker segments, and creates formatted preview without altering original', async () => {
    const completedJob = {
      id: 'job-100',
      status: 'completed' as const,
      phase: 'Completed',
      progress: 100,
      segments: [
        { id: '1', start_ms: 0, end_ms: 2000, speaker: 'Speaker 1', text: 'Welcome everyone.' }
      ],
      formatted_transcripts: []
    };
    const submitSpy = vi.spyOn(transcriptionApi, 'submit').mockResolvedValue(completedJob);
    const getSpy = vi.spyOn(transcriptionApi, 'get').mockResolvedValue(completedJob);
    const formatSpy = vi.spyOn(transcriptionApi, 'format').mockResolvedValue({
      id: 'fmt-100',
      preset: 'meeting-notes',
      content: '## Meeting Notes\n- Speaker 1 welcomed everyone.'
    });

    render(Page);
    const input = screen.getByLabelText(/Video or supported platform URL/i);
    await fireEvent.input(input, { target: { value: 'https://example.com/video.mp4' } });
    await fireEvent.submit(input.closest('form')!);

    await waitFor(() => expect(screen.getByText('Welcome everyone.')).toBeInTheDocument());
    expect(screen.getByText('Speaker 1')).toBeInTheDocument();

    const createPreviewBtn = screen.getByRole('button', { name: /Create preview/i });
    await fireEvent.click(createPreviewBtn);

    const textarea = await screen.findByLabelText(/Editable formatted transcript/i) as HTMLTextAreaElement;
    expect(textarea.value).toContain('## Meeting Notes');
    expect(screen.getByText('Welcome everyone.')).toBeInTheDocument();

    submitSpy.mockRestore();
    getSpy.mockRestore();
    formatSpy.mockRestore();
  });
});
