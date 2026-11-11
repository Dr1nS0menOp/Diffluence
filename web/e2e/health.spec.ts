// SPDX-License-Identifier: Apache-2.0
import { ADMIN, checkAccessibility, expect, login, test } from './helpers';

// System health lists every check with what it says and what to do (spec 051, OPS-103),
// and the summary repeats the ones that need a look.
test('System health shows every check, and the summary repeats what needs a look', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/system');
  await expect(page.getByRole('heading', { name: 'Checks' })).toBeVisible();
  const checks = page
    .getByRole('table')
    .filter({ has: page.getByRole('columnheader', { name: 'What it says' }) });
  for (const name of [
    'Disk',
    'Database',
    'Backup',
    'Migrations',
    'Background work',
    'Audit log',
    'Mail',
    'Login',
    'Release',
  ]) {
    await expect(checks.getByRole('rowheader', { name, exact: true })).toBeVisible();
  }
  // The test server has no mail server, so its check says what to set.
  await expect(checks.getByRole('row', { name: /^Mail/ })).toContainText(
    'Set DIFFLUENCE_SMTP_HOST and DIFFLUENCE_SMTP_FROM',
  );
  // The worker of the test server seals audit events within a second (ADR-041).
  await expect(checks.getByRole('row', { name: /^Audit log/ })).toContainText(
    'sealed in the chain within seconds',
  );
  await checkAccessibility(page);

  // The test server never ran a backup, so that needs a look, here and on the summary.
  await expect(checks.getByRole('row', { name: /^Backup/ })).toContainText('Needs a look');
  await page.goto('/admin/summary');
  await expect(page.getByText('Needs a look: Backup')).toBeVisible();
  await checkAccessibility(page);
});
