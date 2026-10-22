<script lang="ts">
  import type { TranscriptSegment } from '$lib/api/types';
  let { segments = [], loading = false } = $props<{ segments?: TranscriptSegment[]; loading?: boolean }>();
  const stamp = (ms: number) => new Date(ms).toISOString().slice(14, 19);
</script>
<section aria-labelledby="original-heading">
  <h2 id="original-heading">Original transcript</h2>
  {#if loading}<p aria-live="polite">Loading transcript…</p>
  {:else if segments.length === 0}<p>No transcript segments are available yet.</p>
  {:else}<ol class="segments">{#each segments as segment, index (segment.id ?? index)}<li><time datetime={`PT${segment.start_ms / 1000}S`}>[{stamp(segment.start_ms)}]</time><strong>{segment.speaker || 'Unknown speaker'}</strong><span>{segment.text}</span></li>{/each}</ol>{/if}
</section>
<style>.segments{list-style:none;padding:0}.segments li{display:grid;grid-template-columns:5rem 9rem 1fr;gap:.75rem;padding:.75rem 0;border-bottom:1px solid #d8dee9}time{font-variant-numeric:tabular-nums;color:#4c566a}strong{color:#2e3440}@media(max-width:600px){.segments li{grid-template-columns:1fr}.segments li>*{display:block}}</style>
