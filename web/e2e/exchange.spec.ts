// SPDX-License-Identifier: Apache-2.0
// Everything can leave and come in (spec 014), by a person in a real browser.
import { inflateRawSync } from 'node:zlib';
import type { Page } from '@playwright/test';
import { ADMIN, checkAccessibility, expect, login, newSpace, publishedPage, test } from './helpers';

const SITE_ADDRESS = 'https://site.test';
const ZIP_LOCAL_HEADER = 0x04034b50;
const LOCAL_HEADER_BYTES = 30;
const DEFLATED = 8;

/** Reads a zip without a module: Go wrote it with sizes in a central directory. */
function readZip(data: Buffer): Map<string, Buffer> {
  const files = new Map<string, Buffer>();
  const CENTRAL = 0x02014b50;
  let at = data.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  const count = data.readUInt16LE(at + 10);
  at = data.readUInt32LE(at + 16);
  for (let i = 0; i < count; i++) {
    expect(data.readUInt32LE(at)).toBe(CENTRAL);
    const method = data.readUInt16LE(at + 10);
    const size = data.readUInt32LE(at + 20);
    const nameLength = data.readUInt16LE(at + 28);
    const extra = data.readUInt16LE(at + 30);
    const comment = data.readUInt16LE(at + 32);
    const local = data.readUInt32LE(at + 42);
    const name = data.toString('utf8', at + 46, at + 46 + nameLength);
    expect(data.readUInt32LE(local)).toBe(ZIP_LOCAL_HEADER);
    const start = local + LOCAL_HEADER_BYTES + data.readUInt16LE(local + 26) + data.readUInt16LE(local + 28);
    const body = data.subarray(start, start + size);
    files.set(name, method === DEFLATED ? inflateRawSync(body) : body);
    at += 46 + nameLength + extra + comment;
  }
  return files;
}

const CONTENT_TYPES: Record<string, string> = {
  html: 'text/html',
  css: 'text/css',
  js: 'text/javascript',
  json: 'application/json',
};

/** Shows a built site at its own address, as a web server would. */
async function serveSite(page: Page, files: Map<string, Buffer>) {
  await page.route(`${SITE_ADDRESS}/**`, async (route) => {
    const name = new URL(route.request().url()).pathname.slice(1) || 'index.html';
    const body = files.get(name);
    if (!body) {
      await route.fulfill({ status: 404, body: 'not found' });
      return;
    }
    await route.fulfill({ body, contentType: CONTENT_TYPES[name.split('.').pop() ?? ''] ?? 'text/plain' });
  });
}

test('a page leaves as a file through the dialog, by keyboard', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Exports');
  const id = await publishedPage(page, space.id, 'Leaving', 'words to take along');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Export' }).click();
  const dialog = page.getByRole('dialog', { name: 'Export' });
  await dialog.getByLabel('Format').selectOption('text');
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Export' }).press('Enter');
  const link = dialog.getByRole('link', { name: 'Download' });
  await expect(link).toBeVisible();
  const href = await link.getAttribute('href');
  const res = await page.request.get(href ?? '');
  expect(await res.text()).toContain('words to take along');
});

test('choosing pages says how many, and nothing chosen says why it cannot start', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Choosing');
  const parent = await publishedPage(page, space.id, 'Parent page', 'x');
  await publishedPage(page, space.id, 'Child page', 'y', parent);
  await page.goto(`/pages/${parent}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Export' }).click();
  const dialog = page.getByRole('dialog', { name: 'Export' });
  await dialog.getByLabel('Choose pages').first().check();
  await expect(dialog.getByRole('button', { name: 'Export' })).toBeDisabled();
  await expect(dialog.getByText('Choose at least one page.')).toBeVisible();
  await dialog.getByRole('checkbox', { name: 'Parent page' }).check();
  await expect(dialog.getByText('2 pages chosen')).toBeVisible();
  await dialog.getByRole('checkbox', { name: 'Child page' }).uncheck();
  await expect(dialog.getByText('1 page chosen')).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Export' })).toBeEnabled();
});

test('an import shows its report first, runs when asked, and can be undone', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Imports');
  await page.goto(`/spaces/${space.key}/settings?section=import`);
  await page
    .locator('label.file-picker', { hasText: 'Import' })
    .locator('input')
    .setInputFiles({
      name: 'Report first.md',
      mimeType: 'text/markdown',
      buffer: Buffer.from('---\nodd: thing\n---\n# Inside\n'),
    });
  await expect(page.getByText('The front matter key "odd" was left out.', { exact: false })).toBeVisible();
  const tree = await page.request.get(`/api/v1/spaces/${space.id}/tree`);
  expect(await tree.text()).not.toContain('Report first');

  await page.getByRole('button', { name: 'Import', exact: true }).click();
  await expect(page.getByText('The import is done.')).toBeVisible();
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Undo this import' }).click();
  await expect(page.getByText('The import was undone.', { exact: false })).toBeVisible();
});

test('a public site goes through a preview, and works by keyboard without the wiki', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const on = await page.request.put('/api/v1/admin/export-policy', {
    data: {
      rules: [],
      limits: {
        max_export_mib: 100,
        max_import_mib: 100,
        public_sites_enabled: true,
        public_links_enabled: true,
      },
    },
  });
  expect(on.status()).toBe(204);
  const space = await newSpace(page, 'Open space');
  const shown = await publishedPage(page, space.id, 'Shown to the world', 'a findable word');
  await publishedPage(page, space.id, 'Left inside', 'plantedsecret');

  await page.goto(`/spaces/${space.key}/sites`);
  await page.getByLabel('Name').last().fill('Our docs');
  await page.getByRole('button', { name: 'Create site' }).click();
  await page.getByRole('checkbox', { name: 'Shown to the world' }).check();
  await page.getByRole('button', { name: 'Save' }).nth(1).click();
  await page.getByRole('button', { name: 'Show what would change' }).click();
  await expect(page.getByRole('heading', { name: 'Added' })).toBeVisible();
  await checkAccessibility(page);
  const publish = page.getByRole('button', { name: 'Publish', exact: true });
  await expect(publish).toBeDisabled();
  await page.getByLabel('Type Our docs to publish for the first time').fill('Our docs');
  await publish.click();
  await expect(page.getByRole('link', { name: 'Download as zip' })).toBeVisible();

  const sites = await page.request.get(`/api/v1/spaces/${space.id}/sites`);
  const { sites: list } = (await sites.json()) as { sites: { id: string }[] };
  const zip = await page.request.get(`/api/v1/sites/${list[0]?.id}/download`);
  const files = readZip(await zip.body());
  expect(files.get('search.json')?.toString()).not.toContain('plantedsecret');

  await serveSite(page, files);
  await page.goto(`${SITE_ADDRESS}/index.html`);
  await expect(page.getByRole('link', { name: 'Shown to the world' }).first()).toBeVisible();
  await checkAccessibility(page);
  await page.getByLabel('Search this site').focus();
  await page.keyboard.type('findable');
  const hit = page.getByRole('status').getByRole('link', { name: 'Shown to the world' });
  await expect(hit).toBeVisible();
  await hit.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: 'Shown to the world', level: 1 })).toBeVisible();
  await checkAccessibility(page);

  await page.goto(`/pages/${shown}`);
  await expect(page.getByText('Public', { exact: true })).toBeVisible();
});
