import { expect, test } from '@playwright/test';

test.describe('Video transcription utility critical journeys', () => {
  test('rejects unsafe loopback URL without starting download', async ({ page }) => {
    await page.goto('/utilities/video-transcription');
    await page.getByLabel('Video or supported platform URL').fill('http://127.0.0.1/private.mp4');
    await page.getByRole('button', { name: 'Start transcription' }).click();
    await expect(page.getByRole('alert')).toContainText(/valid public http or https video URL/i);
  });

  test('submits valid URL, displays speaker-attributed transcript, and creates editable Gemini preview without altering original', async ({ page }) => {
    await page.goto('/utilities/video-transcription');
    await page.getByLabel('Video or supported platform URL').fill('https://example.com/session.mp4');
    await page.getByRole('button', { name: 'Start transcription' }).click();

    await expect(page.getByText('Welcome to the video transcription utility demo.')).toBeVisible({ timeout: 20000 });
    await expect(page.getByText('We are testing speaker diarization and Gemini formatting.')).toBeVisible();
    await expect(page.getByText('Speaker 1')).toBeVisible();
    await expect(page.getByText('Speaker 2')).toBeVisible();

    await page.getByRole('button', { name: 'Create preview' }).click();
    const previewBox = page.getByLabel('Editable formatted transcript');
    await expect(previewBox).toBeVisible({ timeout: 10000 });
    await expect(previewBox).toHaveValue(/Meeting Notes/);

    await previewBox.fill('## Custom Edited Meeting Notes\n- Verified non-destructive preview');
    await expect(previewBox).toHaveValue(/Custom Edited Meeting Notes/);

    // Original transcript segments remain visible and unmodified.
    await expect(page.getByText('Welcome to the video transcription utility demo.')).toBeVisible();
    await expect(page.getByText('We are testing speaker diarization and Gemini formatting.')).toBeVisible();
  });

  test('surfaces recoverable processing failure and allows retry', async ({ page }) => {
    await page.goto('/utilities/video-transcription');
    await page.getByLabel('Video or supported platform URL').fill('https://example.com/fail-media.mp4');
    await page.getByRole('button', { name: 'Start transcription' }).click();

    await expect(page.getByRole('button', { name: 'Retry' })).toBeVisible({ timeout: 20000 });
    await expect(page.getByRole('alert')).toContainText(/Could not download or normalize media|failed safely/i);
  });
});
