<svelte:head>
  <title>Web Utilities</title>
</svelte:head>
<main>
  <h1>Web Utilities</h1>
  <p>Select a utility to get started:</p>
  <ul>
    <li><a href="/utilities/video-transcription">Video transcription</a> — Speaker-attributed transcripts and editable Gemini Flash previews.</li>
  </ul>
</main>
<style>
  main {
    max-width: 920px;
    margin: auto;
    padding: 3rem 1.25rem;
    font-family: system-ui, -apple-system, sans-serif;
  }
  a {
    color: var(--link, #2563eb);
  }
  @media (prefers-color-scheme: dark) {
    a {
      color: var(--link, #60a5fa);
    }
  }
</style>
