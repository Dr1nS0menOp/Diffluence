// SPDX-License-Identifier: Apache-2.0
// Spec 024: what happens around a file once it is on a page.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  editorSettled,
  expect,
  login,
  newSpace,
  publishedPage,
  test,
} from './helpers';

const ADD_FILE = 'input[type=file][aria-label="Add image or file"]';
const TWO_MEBIBYTES = 2 << 20;

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

async function addFile(page: Page, pageId: string, name: string, text: string) {
  const sent = await page.request.post(`/api/v1/pages/${pageId}/attachments?filename=${name}`, {
    headers: { 'Content-Type': 'application/octet-stream' },
    data: text,
  });
  expect(sent.ok(), await sent.text()).toBeTruthy();
  return (await sent.json()) as { id: string };
}

async function showFiles(page: Page, pageId: string, tiles: { id: string; name: string }[]) {
  const done = await page.request.post(`/api/v1/pages/${pageId}/publish`, {
    data: {
      title: 'Page with tiles',
      base_version: 1,
      document: {
        type: 'doc',
        content: tiles.map((tile) => ({
          type: 'file',
          attrs: { attachment_id: tile.id, filename: tile.name },
        })),
      },
    },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
}

async function openAttachments(page: Page, pageId: string) {
  await page.goto(`/pages/${pageId}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Attachments' }).click();
  return page.getByRole('dialog', { name: 'Attachments' });
}

test('a large upload shows how far it is, and stopping it leaves nothing', async ({ page }) => {
  const space = await newSpace(page, 'Uploads');
  const id = await publishedPage(page, space.id, 'Upload page', 'x');
  // Held back, so the upload is still running when the person changes their mind.
  await page.route(/\/attachments\?filename=big/, () => undefined);
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.locator(ADD_FILE).setInputFiles({
    name: 'big.bin',
    mimeType: 'application/octet-stream',
    buffer: Buffer.alloc(TWO_MEBIBYTES),
  });
  await expect(page.getByRole('status').filter({ hasText: /Uploading big\.bin: \d+%/ })).toBeVisible();
  await page.getByRole('status').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByText(/Uploading big\.bin/)).toBeHidden();
  await expect(page.locator('.file-tile')).toHaveCount(0);
  await expect(page.locator('.error-note, [role=alert]')).toHaveCount(0);
});

test('a file dropped on a tile becomes its next version, and keeps the name', async ({ page }) => {
  const space = await newSpace(page, 'Drops');
  const id = await publishedPage(page, space.id, 'Drop page', 'x');
  const file = await addFile(page, id, 'plan.txt', 'first');
  await showFiles(page, id, [{ id: file.id, name: 'plan.txt' }]);
  await page.goto(`/pages/${id}/edit`);
  const tile = page.locator(`[data-file-tile="${file.id}"]`);
  await expect(tile).toBeVisible();
  const dataTransfer = await page.evaluateHandle(() => {
    const transfer = new DataTransfer();
    transfer.items.add(new File(['a longer second text'], 'renamed.txt', { type: 'text/plain' }));
    return transfer;
  });
  await tile.dispatchEvent('dragover', { dataTransfer });
  await expect(tile).toHaveClass(/drop-target/);
  await tile.dispatchEvent('drop', { dataTransfer });
  await expect(page.getByText('plan.txt is now version 2.')).toBeVisible();
  await expect(tile).not.toHaveClass(/drop-target/);
  await expect(page.locator('.file-tile')).toHaveCount(1);
  await expect(tile.getByText('20 B')).toBeVisible();
});

test('a tile can be large, and that stays after publishing', async ({ page }) => {
  const space = await newSpace(page, 'Tiles');
  const id = await publishedPage(page, space.id, 'Tile page', 'x');
  const file = await addFile(page, id, 'plan.txt', 'words');
  await showFiles(page, id, [{ id: file.id, name: 'plan.txt' }]);
  await page.goto(`/pages/${id}/edit`);
  await page.locator(`[data-file-tile="${file.id}"]`).click();
  await page.getByLabel('File tile').selectOption('large');
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/pages/${id}$`));
  await expect(page.locator(`[data-file-tile="${file.id}"]`)).toHaveAttribute('data-display', 'large');
});

test('a preview shows a file on top of the page and steps to the next', async ({ page }) => {
  const space = await newSpace(page, 'Previews');
  const id = await publishedPage(page, space.id, 'Preview page', 'x');
  const code = await addFile(page, id, 'main.go', 'package main\n\nfunc main() {}\n');
  const notes = await addFile(page, id, 'notes.txt', 'plain words');
  await showFiles(page, id, [
    { id: code.id, name: 'main.go' },
    { id: notes.id, name: 'notes.txt' },
  ]);
  await page.goto(`/pages/${id}`);
  await page.locator(`[data-file-tile="${code.id}"]`).getByRole('button', { name: 'Preview' }).click();
  const first = page.getByRole('dialog', { name: 'main.go' });
  await expect(first.locator('.file-text .hljs-keyword').first()).toHaveText('package');
  await expect(first.getByRole('button', { name: 'Previous' })).toBeDisabled();
  await checkAccessibility(page);
  await first.getByRole('button', { name: 'Next' }).click();
  const second = page.getByRole('dialog', { name: 'notes.txt' });
  await expect(second.getByText('plain words')).toBeVisible();
  await expect(second.getByRole('button', { name: 'Next' })).toBeDisabled();
  await expect(second.getByRole('link', { name: 'Download' })).toHaveAttribute(
    'href',
    /content\?download=1$/,
  );
});

test('the list of a page says who added a file, and whether the page shows it', async ({ page }) => {
  const space = await newSpace(page, 'Lists');
  const id = await publishedPage(page, space.id, 'List page', 'x');
  const shown = await addFile(page, id, 'shown.txt', 'x');
  await addFile(page, id, 'beside.txt', 'x');
  await showFiles(page, id, [{ id: shown.id, name: 'shown.txt' }]);
  const dialog = await openAttachments(page, id);
  const rows = dialog.getByRole('listitem');
  await expect(rows.filter({ hasText: 'beside.txt' })).toContainText('not shown on the page');
  await expect(rows.filter({ hasText: 'shown.txt' }).first()).not.toContainText('not shown on the page');
  await expect(rows.filter({ hasText: 'beside.txt' })).toContainText('v1');
  await checkAccessibility(page);
});

test('labels, a kept comment and the link to one version in the details of a file', async ({ page }) => {
  const space = await newSpace(page, 'Details');
  const id = await publishedPage(page, space.id, 'Details page', 'x');
  await addFile(page, id, 'plan.txt', 'first');
  const file = await addFile(page, id, 'plan.txt', 'second');
  const list = await openAttachments(page, id);
  await list.getByRole('button', { name: 'Details' }).click();
  const dialog = page.getByRole('dialog', { name: 'plan.txt' });
  await dialog.getByLabel('Add a label').fill('Signed Contract');
  await dialog.getByRole('button', { name: 'Add', exact: true }).click();
  await expect(dialog.getByText('signed-contract')).toBeVisible();
  await dialog.getByLabel('Comment').fill('for the board');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(dialog.getByLabel('Comment')).toHaveValue('for the board');
  await expect(dialog.getByRole('link', { name: 'Download' }).last()).toHaveAttribute(
    'href',
    `/api/v1/attachments/${file.id}/versions/1/content`,
  );
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Remove label signed-contract' }).click();
  await expect(dialog.getByText('signed-contract')).toBeHidden();
});

test('the Files screen filters by type and label, and deletes many at once', async ({ page }) => {
  const space = await newSpace(page, 'Bulk');
  const id = await publishedPage(page, space.id, 'Bulk page', 'x');
  const first = await addFile(page, id, 'first.txt', 'x');
  await addFile(page, id, 'second.txt', 'x');
  await addFile(page, id, 'third.pdf', '%PDF-1.4 x');
  const labelled = await page.request.post(`/api/v1/attachments/${first.id}/labels`, {
    data: { label: 'minutes' },
  });
  expect(labelled.ok(), await labelled.text()).toBeTruthy();
  await page.goto(`/spaces/${space.key}/files`);
  const links = page
    .locator('.file-table')
    .first()
    .getByRole('link', { name: /\.(txt|pdf)$/ });
  await expect(links).toHaveCount(3);

  await page.getByLabel('Filter by type').selectOption('pdf');
  await expect(links).toHaveCount(1);
  await page.getByLabel('Filter by type').selectOption('');
  await page.getByLabel('Filter by label').selectOption('minutes');
  await expect(links).toHaveCount(1);
  await page.getByLabel('Filter by label').selectOption('');
  await expect(links).toHaveCount(3);
  await checkAccessibility(page);

  await page.getByLabel('Select first.txt').check();
  await page.getByLabel('Select second.txt').check();
  await page.getByRole('button', { name: 'Delete 2 files' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Delete 2 files' }).click();
  await expect(page.getByText('2 files went to the trash.')).toBeVisible();
  await expect(links).toHaveCount(1);
});

test('the history of a page tells what happened to its files', async ({ page }) => {
  const space = await newSpace(page, 'File history');
  const id = await publishedPage(page, space.id, 'History page', 'x');
  const file = await addFile(page, id, 'plan.txt', 'x');
  const gone = await page.request.delete(`/api/v1/attachments/${file.id}`);
  expect(gone.ok(), await gone.text()).toBeTruthy();
  await page.goto(`/pages/${id}/history`);
  const list = page.getByRole('region', { name: 'Files of this page' });
  await expect(list.getByText(/added plan\.txt/)).toBeVisible();
  await expect(list.getByText(/removed plan\.txt/)).toBeVisible();
  await checkAccessibility(page);
});

test('an administrator gives a space a storage limit, and a full space says so', async ({ page }) => {
  const space = await newSpace(page, `Limited ${Date.now()}`);
  const id = await publishedPage(page, space.id, 'Limited page', 'x');
  await addFile(page, id, 'fills.bin', 'x'.repeat(TWO_MEBIBYTES));
  await page.goto('/admin/spaces');
  const row = page.getByRole('row').filter({ hasText: space.key });
  await row.getByRole('button', { name: 'Storage limit' }).click();
  await page.getByRole('dialog').getByLabel('Storage limit').fill('1');
  await page.getByRole('dialog').getByRole('button', { name: 'Save' }).click();
  await expect(row).toContainText('/ 1');
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page
    .locator(ADD_FILE)
    .setInputFiles({ name: 'more.txt', mimeType: 'text/plain', buffer: Buffer.from('more') });
  await expect(page.getByText('This space has reached its storage limit.')).toBeVisible();
  await page.goto(`/spaces/${space.key}/files`);
  await expect(page.getByText(/uses 2.* of 1/)).toBeVisible();
});
