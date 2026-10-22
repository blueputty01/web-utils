import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  cacheDir: '/tmp/vite-cache',
  plugins: [sveltekit()],
  resolve: { conditions: ['browser'] },
  test: { environment: 'jsdom', setupFiles: ['./src/tests/setup.ts'], include: ['src/**/*.test.ts'] }
});
