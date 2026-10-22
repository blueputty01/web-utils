import { fireEvent, render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import FormattedPreview from './FormattedPreview.svelte';

describe('FormattedPreview', () => {
  it('shows unavailable message before transcript completion', () => {
    render(FormattedPreview, { available: false, onFormat: vi.fn() });
    expect(screen.getByText(/Formatting becomes available after the original transcript completes/i)).toBeInTheDocument();
  });

  it('triggers onFormat with selected preset and renders editable preview', async () => {
    const onFormat = vi.fn().mockResolvedValue(undefined);
    render(FormattedPreview, {
      available: true,
      preview: { id: 'fmt-1', preset: 'meeting-notes', content: '## Notes\n- Item 1' },
      onFormat
    });

    const button = screen.getByRole('button', { name: /Create preview/i });
    await fireEvent.click(button);
    expect(onFormat).toHaveBeenCalledWith('meeting-notes');

    const textarea = screen.getByLabelText(/Editable formatted transcript/i) as HTMLTextAreaElement;
    expect(textarea.value).toContain('## Notes');
    await fireEvent.input(textarea, { target: { value: '## Edited Notes' } });
    expect(textarea.value).toBe('## Edited Notes');
    expect(screen.getByText(/Changes here do not alter the original transcript/i)).toBeInTheDocument();
  });
});
