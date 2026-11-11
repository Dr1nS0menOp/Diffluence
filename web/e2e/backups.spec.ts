// SPDX-License-Identifier: Apache-2.0
import { ADMIN, checkAccessibility, expect, login, test } from './helpers';

// The console runs backups through a worker (spec 058). The test server has no backup
// service, so the page says so, and "back up now" waits for one.
test('Backups says when no backup service checked in, and back up now waits for one', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/backups');
  await expect(page.getByRole('heading', { name: 'Backups', level: 1 })).toBeVisible();
  await expect(page.getByText('No backup service has checked in yet.')).toBeVisible();
  await expect(page.getByText('No test restore was reported yet.')).toBeVisible();
  await checkAccessibility(page);

  // Pressing it twice cannot queue two: the button gives way to the waiting request.
  await page.getByRole('button', { name: 'Back up now' }).click();
  await expect(page.getByText(/A backup was asked for .* and waits for the backup service\./)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Back up now' })).toHaveCount(0);

  // An empty field means the default the installation was started with.
  await page.getByLabel('Backups to keep').fill('7');
  await page.getByRole('button', { name: 'Save' }).click();
  await page.reload();
  await expect(page.getByLabel('Backups to keep')).toHaveValue('7');
});

// The configuration shows where each setting came from and never a secret value.
test('Configuration lists each setting with its source', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/configuration');
  const settings = page.getByRole('table');
  await expect(settings.getByRole('rowheader', { name: 'DATABASE_URL', exact: true })).toBeVisible();
  await expect(settings.getByRole('row', { name: /^DATABASE_URL/ })).not.toContainText('password=');
  await checkAccessibility(page);
});

// Logging one person out needs their address; everyone is a separate, confirmed step.
test('Sessions logs one person out by address', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/sessions');
  await expect(page.getByRole('button', { name: 'Log this person out' })).toBeDisabled();
  await page.getByLabel('Address of the person').fill('nobody-here@example.com');
  await page.getByRole('button', { name: 'Log this person out' }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await checkAccessibility(page);
});
