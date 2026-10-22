<script lang="ts">
  import { onMount } from 'svelte';
  import { applySystemTheme } from '$lib/theme';

  let { children } = $props();

  onMount(() => applySystemTheme());
</script>

{@render children()}

<style>
  :global(:root) {
    color-scheme: light dark;
    --bg: #f5f7fb;
    --text: #172033;
    --text-strong: #2e3440;
    --text-muted: #475569;
    --text-time: #4c566a;
    --surface: #ffffff;
    --surface-input: #ffffff;
    --border: #dce3ef;
    --border-subtle: #d8dee9;
    --border-input: #94a3b8;
    --shadow: 0 1px 2px #1720330d;
    --primary: #2563eb;
    --primary-text: #ffffff;
    --secondary: #475569;
    --link: #2563eb;
    --error: #b42318;
  }

  @media (prefers-color-scheme: dark) {
    :global(:root:not([data-theme='light'])) {
      --bg: #0f172a;
      --text: #e2e8f0;
      --text-strong: #f1f5f9;
      --text-muted: #94a3b8;
      --text-time: #94a3b8;
      --surface: #1e293b;
      --surface-input: #0f172a;
      --border: #334155;
      --border-subtle: #334155;
      --border-input: #475569;
      --shadow: 0 1px 2px #00000040;
      --primary: #3b82f6;
      --primary-text: #ffffff;
      --secondary: #334155;
      --link: #60a5fa;
      --error: #f87171;
    }
  }

  :global(:root[data-theme='dark']) {
    --bg: #0f172a;
    --text: #e2e8f0;
    --text-strong: #f1f5f9;
    --text-muted: #94a3b8;
    --text-time: #94a3b8;
    --surface: #1e293b;
    --surface-input: #0f172a;
    --border: #334155;
    --border-subtle: #334155;
    --border-input: #475569;
    --shadow: 0 1px 2px #00000040;
    --primary: #3b82f6;
    --primary-text: #ffffff;
    --secondary: #334155;
    --link: #60a5fa;
    --error: #f87171;
  }

  :global(body) {
    margin: 0;
    background: var(--bg);
    color: var(--text);
    font-family: system-ui, -apple-system, sans-serif;
  }

  :global(a) {
    color: var(--link);
  }
</style>
