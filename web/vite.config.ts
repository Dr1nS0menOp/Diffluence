// SPDX-License-Identifier: Apache-2.0
import { rmSync } from 'node:fs';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

const outDir = '../internal/webui/dist';

export default defineConfig({
  plugins: [
    react(),
    {
      // Removes only what a build produces. Emptying the whole folder would delete
      // the placeholder that lets the Go code compile from a clean checkout.
      name: 'clean-build-output',
      apply: 'build',
      buildStart() {
        rmSync(`${outDir}/assets`, { recursive: true, force: true });
      },
    },
  ],
  build: {
    outDir,
    emptyOutDir: false,
    // The polyfill is an inline script, which our Content Security Policy forbids.
    modulePreload: { polyfill: false },
    chunkSizeWarningLimit: 1500,
  },
  server: {
    proxy: { '/api': 'http://localhost:8080' },
  },
  test: {
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx', 'scripts/**/*.test.mjs'],
  },
});
