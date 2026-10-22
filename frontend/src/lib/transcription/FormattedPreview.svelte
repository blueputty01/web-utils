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
<style>.controls{display:flex;flex-wrap:wrap;align-items:end;gap:.75rem;margin-bottom:1rem}.controls label{display:grid;gap:.25rem}select,textarea{background:var(--surface-input,#fff);color:var(--text,#172033);border:1px solid var(--border-input,#94a3b8);border-radius:.3rem;font:inherit}select{padding:.55rem .65rem}button{background:var(--primary,#2563eb);color:var(--primary-text,white);border:0;border-radius:.3rem;padding:.6rem .9rem;font:inherit;cursor:pointer}button:disabled{opacity:.6;cursor:not-allowed}.error{color:var(--error,#b42318)}.hint{color:var(--text-muted,#475569)}textarea{box-sizing:border-box;width:100%;line-height:1.5;padding:.75rem}@media(prefers-color-scheme:dark){select,textarea{background:var(--surface-input,#0f172a);color:var(--text,#e2e8f0);border-color:var(--border-input,#475569)}button{background:var(--primary,#3b82f6)}.error{color:var(--error,#f87171)}.hint{color:var(--text-muted,#94a3b8)}}</style>
