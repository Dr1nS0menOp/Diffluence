// SPDX-License-Identifier: Apache-2.0
import { readFileSync } from 'node:fs';
import { expect, type Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  EDITOR,
  grant,
  login,
  newSpace,
  test,
  unique,
  VIEWER,
} from './helpers';

async function newDatabase(page: Page, title: string) {
  const space = await newSpace(page, `${title} space`);
  const res = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, parent_id: space.home_page_id, title, kind: 'database' },
  });
  expect(res.status(), await res.text()).toBe(201);
  const { id } = (await res.json()) as { id: string };
  return { space, id };
}

const grid = (page: Page) => page.getByRole('grid');
const cell = (page: Page, row: number, column: number) =>
  grid(page).locator('tbody tr').nth(row).getByRole('gridcell').nth(column);

async function publish(page: Page) {
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Publish', exact: true })).toHaveCount(0);
}

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('a cell that does not fit stays open and says why', async ({ page }) => {
  const { id } = await newDatabase(page, 'Refused');
  const sheet = {
    type: 'doc',
    content: [
      {
        type: 'sheet',
        attrs: {
          columns: [{ id: 'd', name: 'When', type: 'date' }],
          rows: [{ id: 'r', cells: {} }],
        },
      },
    ],
  };
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title: 'Refused', document: sheet, base_version: 1 },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  await page.goto(`/pages/${id}/edit`);
  await cell(page, 0, 0).click();
  await page.keyboard.type('tomorrow');
  await page.keyboard.press('Enter');
  await expect(page.getByText('Write a date as 2026-11-06.')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(cell(page, 0, 0)).toHaveText('');
});

test('a database is made from Create and used with the keys of a spreadsheet', async ({ page }) => {
  const space = await newSpace(page, 'Budget space');
  await page.goto(`/spaces/${space.key}`);
  await page.getByRole('button', { name: 'More to create' }).click();
  await page.getByRole('menuitem', { name: 'New database' }).click();
  await page.getByRole('dialog').getByLabel('Title').fill('Budget');
  await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();
  await expect(grid(page)).toBeVisible();

  await cell(page, 0, 0).click();
  await page.keyboard.type('Tea');
  await page.keyboard.press('Tab');
  await page.keyboard.type('2');
  await page.keyboard.press('Enter');
  await page.keyboard.type('3');
  await page.keyboard.press('Enter');
  await page.keyboard.type('=SUM(B1:B2)');
  await page.keyboard.press('Enter');
  await expect(cell(page, 2, 1)).toHaveText('5');

  await page.keyboard.press('ArrowUp');
  await page.keyboard.press('Delete');
  await expect(cell(page, 2, 1)).toHaveText('');
  await page.keyboard.press('Control+z');
  await expect(cell(page, 2, 1)).toHaveText('5');
  await page.keyboard.press('Escape');

  await publish(page);
  await page.reload();
  await expect(cell(page, 2, 1)).toHaveText('5');
  await expect(page.getByRole('link', { name: 'Budget', exact: true }).first()).toBeVisible();
  await checkAccessibility(page);
});

test('a block copied from Excel fills the grid', async ({ page }) => {
  const { id } = await newDatabase(page, 'Paste');
  await page.goto(`/pages/${id}/edit`);
  await cell(page, 0, 0).click();
  await page.evaluate(() => {
    const data = new DataTransfer();
    data.setData('text/plain', 'a\t1\r\nb\t2\r\nc\t3\r\nd\t4\r\n');
    document
      .querySelector('[data-active="true"]')
      ?.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }));
  });
  await expect(cell(page, 3, 1)).toHaveText('4');
  await expect(grid(page).locator('tbody tr')).toHaveCount(4);
});

test('a reader sorts and filters, and changes nothing', async ({ page, browser }) => {
  const { space, id } = await newDatabase(page, 'Read only');
  const sheet = {
    type: 'doc',
    content: [
      {
        type: 'sheet',
        attrs: {
          columns: [
            { id: 'n', name: 'Name', type: 'text' },
            { id: 'v', name: 'Value', type: 'number' },
          ],
          rows: [
            { id: 'r1', cells: { n: 'Pear', v: '10' } },
            { id: 'r2', cells: { n: 'Apple', v: '9' } },
            { id: 'r3', cells: { n: 'Cherry' } },
          ],
        },
      },
    ],
  };
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title: 'Read only', document: sheet, base_version: 1 },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  const email = `${unique('reader')}@example.com`;
  const reader = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: reader }, ['space.view']);
  const context = await browser.newContext();
  const other = await context.newPage();
  await login(other, email);
  await other.goto(`/pages/${id}`);

  await expect(grid(other)).toHaveAttribute('aria-readonly', 'true');
  await expect(other.getByRole('button', { name: 'Export CSV' })).toHaveCount(0);
  await other.getByRole('columnheader', { name: /Value/ }).getByRole('button').click();
  await expect(cell(other, 0, 0)).toHaveText('Apple');
  await expect(cell(other, 2, 0)).toHaveText('Cherry');
  await other.getByRole('columnheader', { name: /Value/ }).getByRole('button').click();
  await expect(cell(other, 0, 0)).toHaveText('Pear');
  await expect(cell(other, 2, 0)).toHaveText('Cherry');
  await other.getByLabel('Filter rows').fill('app');
  await expect(grid(other).locator('tbody tr')).toHaveCount(1);
  await cell(other, 0, 0).click();
  await other.keyboard.type('x');
  await expect(cell(other, 0, 0)).toHaveText('Apple');
  await context.close();
});

test('CSV goes out harmless, and comes back in', async ({ page }) => {
  const { id } = await newDatabase(page, 'Trip');
  const sheet = {
    type: 'doc',
    content: [
      {
        type: 'sheet',
        attrs: {
          columns: [
            { id: 'n', name: 'Name', type: 'text' },
            { id: 'v', name: 'Value', type: 'number' },
          ],
          rows: [
            { id: 'r1', cells: { n: '@SUM(1)', v: '2' } },
            { id: 'r2', cells: { n: 'Sum', v: '=B1*2' } },
          ],
        },
      },
    ],
  };
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title: 'Trip', document: sheet, base_version: 1 },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  await page.goto(`/pages/${id}`);
  const waiting = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Export CSV' }).click();
  const file = await waiting;
  expect(file.suggestedFilename()).toBe('Trip.csv');
  const text = readFileSync(await file.path(), 'utf8');
  expect(text).toContain(`"'@SUM(1)","2"`);
  expect(text).toContain('"Sum","4"');

  await page.goto(`/pages/${id}/edit`);
  await page.getByLabel('Import CSV').setInputFiles({
    name: 'in.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from('Fruit;Count\nKiwi;3\n'),
  });
  await page.getByRole('button', { name: 'Replace the sheet' }).click();
  await expect(cell(page, 0, 0)).toHaveText('Kiwi');
});

test('a database opens in GridCraft, which cannot reach the session', async ({ page }) => {
  const { id } = await newDatabase(page, 'Heavy');
  const filled = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Heavy',
      base_version: 1,
      document: {
        type: 'doc',
        content: [
          {
            type: 'sheet',
            attrs: {
              columns: [{ id: 'a', name: 'Fruit', type: 'text' }],
              rows: [{ id: 'r', cells: { a: 'Kiwi' } }],
            },
          },
        ],
      },
    },
  });
  expect(filled.ok(), await filled.text()).toBeTruthy();
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Open in GridCraft' }).click();
  const dialog = page.getByRole('dialog', { name: 'Heavy.csv' });
  await expect(dialog.getByRole('status')).toHaveCount(0, { timeout: 60_000 });
  const viewer = page.frames().find((frame) => frame.url().includes('/vendor/gridcraft/'));
  if (!viewer) throw new Error('the viewer has no frame');
  expect(await viewer.evaluate(() => self.origin)).toBe('null');
});

test('an editor can change a database and history shows it', async ({ page, browser }) => {
  const { space, id } = await newDatabase(page, 'Shared');
  const email = `${unique('editor')}@example.com`;
  const editor = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: editor }, [...EDITOR, ...VIEWER]);
  const context = await browser.newContext();
  const other = await context.newPage();
  await login(other, email);
  await other.goto(`/pages/${id}/edit`);
  await cell(other, 0, 0).click();
  await other.keyboard.type('Changed');
  await other.keyboard.press('Enter');
  await publish(other);
  await other.goto(`/pages/${id}/history`);
  await expect(other.getByText('Version 2').first()).toBeVisible();
  await context.close();
});
