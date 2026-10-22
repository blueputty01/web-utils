export type SystemTheme = 'light' | 'dark';

export function getSystemTheme(win: Pick<Window, 'matchMedia'> | undefined = typeof window !== 'undefined' ? window : undefined): SystemTheme {
  if (!win || typeof win.matchMedia !== 'function') {
    return 'light';
  }
  return win.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applySystemTheme(
  doc: Document | undefined = typeof document !== 'undefined' ? document : undefined,
  win: Pick<Window, 'matchMedia'> | undefined = typeof window !== 'undefined' ? window : undefined
): () => void {
  if (!doc || !win || typeof win.matchMedia !== 'function') {
    return () => {};
  }

  const mediaQuery = win.matchMedia('(prefers-color-scheme: dark)');
  const update = (matches: boolean) => {
    const theme: SystemTheme = matches ? 'dark' : 'light';
    doc.documentElement.dataset.theme = theme;
    doc.documentElement.style.colorScheme = theme;
  };

  update(mediaQuery.matches);

  const listener = (event: MediaQueryListEvent) => {
    update(event.matches);
  };

  if (typeof mediaQuery.addEventListener === 'function') {
    mediaQuery.addEventListener('change', listener);
    return () => mediaQuery.removeEventListener('change', listener);
  }

  mediaQuery.addListener?.(listener);
  return () => mediaQuery.removeListener?.(listener);
}
