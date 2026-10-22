<script lang="ts">
  import type { FormatPreset, FormattedTranscript } from '$lib/api/types';
  let { available, preview, loading = false, error, onFormat }: { available: boolean; preview?: FormattedTranscript; loading?: boolean; error?: string; onFormat: (preset: FormatPreset) => Promise<void> } = $props();
  let preset = $state<FormatPreset>('meeting-notes');
  let content = $state('');
  let lastPreviewId = $state<string | undefined>();
  $effect(() => { if (preview && preview.id !== lastPreviewId) { lastPreviewId = preview.id; content = preview.content; } });
</script>
<section aria-labelledby="format-heading">
  <h2 id="format-heading">Gemini formatted preview</h2>
  {#if !available}<p>Formatting becomes available after the original transcript completes.</p>
  {:else}<div class="controls"><label for="preset">Format</label><select id="preset" bind:value={preset}><option value="meeting-notes">Meeting notes</option><option value="clean-transcript">Clean transcript</option><option value="summary">Summary</option></select><button onclick={() => onFormat(preset)} disabled={loading}>{loading ? 'Formatting…' : 'Create preview'}</button></div>{#if error}<p class="error" role="alert">{error}</p>{/if}{#if preview}<label for="formatted-content">Editable preview</label><textarea id="formatted-content" bind:value={content} rows="14" aria-label="Editable formatted transcript"></textarea><p class="hint">Changes here do not alter the original transcript.</p>{/if}{/if}
</section>
<style>.controls{display:flex;flex-wrap:wrap;align-items:end;gap:.75rem;margin-bottom:1rem}.controls label{display:grid;gap:.25rem}button{background:#2563eb;color:white;border:0;border-radius:.3rem;padding:.6rem .9rem;font:inherit}button:disabled{opacity:.6}.error{color:#b42318}.hint{color:#475569}textarea{box-sizing:border-box;width:100%;font:inherit;line-height:1.5;padding:.75rem}</style>
