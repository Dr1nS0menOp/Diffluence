// SPDX-License-Identifier: Apache-2.0
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: {
    timeout: 10_000,
    // The pictures of the design standard. The default colour tolerance of 0.2 lets a
    // blue button turn green unnoticed, which is exactly what these must catch.
    toHaveScreenshot: { maxDiffPixelRatio: 0.002, threshold: 0.02, animations: 'disabled' },
  },
  snapshotPathTemplate: '{testDir}/design-baselines/{arg}{ext}',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  // On GitHub a failure also becomes an annotation, which can be read without logging in.
  reporter: process.env.GITHUB_ACTIONS ? [['list'], ['github']] : [['list']],
  use: {
    baseURL: process.env.BASE_URL ?? 'http://localhost:8080',
    trace: 'retain-on-failure',
    ...devices['Desktop Chrome'],
    locale: 'en-GB',
    timezoneId: 'Europe/Amsterdam',
  },
});
