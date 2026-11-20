// SPDX-License-Identifier: Apache-2.0
import { readFileSync } from 'node:fs';
import { ADMIN, checkAccessibility, createUser, expect, login, test, unique } from './helpers';

// The console shows what runs the installation (spec 075, wave CS1): upgrades, events, the
// signatures of the audit log, the monitoring files, the log level for a while, the disk
// thresholds and the count before logging everyone out.

test('a member reaches none of the areas that run the installation', async ({ page, browser }) => {
  const email = `${unique('member')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  for (const path of ['/admin/upgrades', '/admin/events', '/admin/system', '/admin/configuration']) {
    await page.goto(path);
    await expect(page.getByRole('alert')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Nodes' })).toHaveCount(0);
  }
  const answer = await page.request.get('/api/v1/admin/monitoring/rules');
  expect(answer.status()).toBe(403);
});

test('the upgrade page shows the nodes, the cleanup migrations and how to upgrade', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/upgrades');
  await expect(page.getByRole('heading', { name: 'Upgrades', level: 1 })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Nodes' })).toBeVisible();
  await expect(page.getByText('No cleanup migration waits.')).toBeVisible();
  await expect(page.getByText('git pull && docker compose up -d --build')).toBeVisible();
  await checkAccessibility(page);
});

test('the events area says where the events go', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/events');
  await expect(page.getByText(/Events go through (NATS|PostgreSQL), since/)).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Nodes of an older release' })).toBeVisible();
  await checkAccessibility(page);
});

test('the audit area shows the keys that sign and how to report the witness', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/audit');
  await expect(page.getByRole('heading', { name: 'Signatures' })).toBeVisible();
  await expect(
    page.getByRole('cell', { name: "The installation's own, from the encryption key" }),
  ).toBeVisible();
  await expect(
    page.getByText('kenwerk audit verify --witness FILE --report', { exact: false }),
  ).toBeVisible();
});

test('the monitoring files are the files of the release', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/system');
  await page.getByRole('button', { name: 'Monitoring files' }).click();
  await expect(page.getByRole('menuitem', { name: 'Monitoring rules (Prometheus)' })).toBeVisible();
  await page.keyboard.press('Escape');
  for (const [kind, name] of [
    ['rules', 'kenwerk-rules.yml'],
    ['dashboard', 'kenwerk-dashboard.json'],
  ]) {
    const answer = await page.request.get(`/api/v1/admin/monitoring/${kind}`);
    expect(answer.status()).toBe(200);
    expect(answer.headers()['content-disposition']).toContain(name);
    expect(await answer.text()).toBe(readFileSync(`../deploy/monitoring/${name}`, 'utf8'));
  }
});

test('the log level changes for a while and goes back on request', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/system');
  const line = page.getByRole('status').filter({ hasText: 'logs at' });
  await expect(line).toContainText('as it was started');
  await page.getByRole('button', { name: 'Change', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Log level for a while' });
  await dialog.getByLabel('Level').selectOption('debug');
  await dialog.getByLabel('For').selectOption('1');
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Set the log level' }).click();
  await expect(line).toContainText('Every node logs at Debug until');
  await page.getByRole('button', { name: 'Change', exact: true }).click();
  await page.getByRole('button', { name: 'Back to the started level now' }).click();
  await expect(line).toContainText('as it was started');
});

test('lower disk thresholds ask for a second factor, and nothing is stored before it', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/settings');
  await expect(page.getByRole('heading', { name: 'Health' })).toBeVisible();
  await page.getByLabel('Warn when less than this share of the disk is free (percent)').fill('5');
  await page
    .locator('form')
    .filter({ has: page.getByRole('heading', { name: 'Health' }) })
    .getByRole('button', { name: 'Save', exact: true })
    .click();
  await expect(
    page.getByText('Warning later or refusing files later lets the disk fill further'),
  ).toBeVisible();
  const stored = await page.request.get('/api/v1/admin/settings/health');
  expect(await stored.json()).toEqual({ disk_warn_percent: 0, disk_refuse_percent: 0 });
});

test('logging everyone out says first how many sessions end', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/sessions');
  await expect(page.getByText('docs/operations/incidents.md', { exact: false })).toBeVisible();
  await page.getByRole('button', { name: 'Log everyone out' }).click();
  const dialog = page.getByRole('dialog', { name: 'Log everyone out' });
  await expect(dialog.getByText(/This ends \d+ sessions of \d+ people/)).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('heading', { name: 'Everyone' })).toBeVisible();
});

test('the configuration shows the traces and the outside systems, and changes nothing', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/configuration');
  await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Outside systems at the start' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Why it is not changed here' })).toBeVisible();
  await expect(page.locator('.settings-area form')).toHaveCount(0);
  await checkAccessibility(page);
});
