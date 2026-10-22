import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import TranscriptViewer from './TranscriptViewer.svelte';

describe('TranscriptViewer', () => {
  it('renders timestamped, speaker-attributed segments', () => {
    render(TranscriptViewer, { segments: [{ start_ms: 65000, end_ms: 70000, speaker: 'Speaker 1', text: 'Hello there' }] });
    expect(screen.getByText('[01:05]')).toBeInTheDocument();
    expect(screen.getByText('Speaker 1')).toBeInTheDocument();
    expect(screen.getByText('Hello there')).toBeInTheDocument();
  });
});
