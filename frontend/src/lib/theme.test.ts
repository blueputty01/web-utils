import { render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { applySystemTheme, getSystemTheme } from './theme';
import Layout from '../routes/+layout.svelte';
import { createRawSnippet } from 'svelte';

function createMockMatchMedia(initialMatches: boolean) {
  let matches = initialMatches;
  const listeners = new Set<(event: MediaQueryListEvent) => void>();

  const mediaQueryList = {
    get matches() {
      return matches;
    },
    media: '(prefers-color-scheme: dark)',
    onchange: null,
    addEventListener: vi.fn((type: string, listener: (event: MediaQueryListEvent) => void) => {
      if (type === 'change') listeners.add(listener);
    }),
    removeEventListener: vi.fn((type: string, listener: (event: MediaQueryListEvent) => void) => {
      if (type === 'change') listeners.delete(listener);
    }),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn()
  } as unknown as MediaQueryList;

  return {
    matchMedia: vi.fn().mockReturnValue(mediaQueryList),
    setMatches(next: boolean) {
      matches = next;
      for (const listener of listeners) {
        listener({ matches: next, media: '(prefers-color-scheme: dark)' } as MediaQueryListEvent);
      }
    }
  };
}

describe('system theme auto dark mode', () => {
  it('detects dark and light mode from system settings', () => {
    const darkMock = createMockMatchMedia(true);
    expect(getSystemTheme({ matchMedia: darkMock.matchMedia })).toBe('dark');

    const lightMock = createMockMatchMedia(false);
    expect(getSystemTheme({ matchMedia: lightMock.matchMedia })).toBe('light');
  });

  it('applies system theme to documentElement and updates when system preference changes', () => {
    const mock = createMockMatchMedia(true);
    const cleanup = applySystemTheme(document, { matchMedia: mock.matchMedia });

    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.style.colorScheme).toBe('dark');

    mock.setMatches(false);
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(document.documentElement.style.colorScheme).toBe('light');

    cleanup();
    mock.setMatches(true);
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('syncs theme on Layout mount and cleans up on unmount', () => {
    const mock = createMockMatchMedia(true);
    vi.stubGlobal('matchMedia', mock.matchMedia);

    const children = createRawSnippet(() => ({
      render: () => '<div>Content</div>'
    }));

    const { unmount } = render(Layout, { children });
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.style.colorScheme).toBe('dark');

    mock.setMatches(false);
    expect(document.documentElement.dataset.theme).toBe('light');

    unmount();
    vi.unstubAllGlobals();
  });
});
