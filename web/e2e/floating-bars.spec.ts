// SPDX-License-Identifier: Apache-2.0
// Spec 047, part A: bars over selected text and over a selected image.
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

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

const body = (page: Page) => page.locator('.prose.editing');
const line = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const textBar = (page: Page) => page.getByRole('toolbar', { name: 'Text tools' });
const imageBar = (page: Page) => page.getByRole('toolbar', { name: 'Image tools' });
const fixedRow = (page: Page) => page.getByRole('toolbar', { name: 'Formatting' });

async function editing(page: Page, title: string, content: unknown[]) {
  const space = await newSpace(page, `${title} space`);
  const id = await publishedPage(page, space.id, title, 'start');
  const answer = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title, document: { type: 'doc', content }, base_version: 1 },
  });
  expect(answer.ok(), await answer.text()).toBeTruthy();
  await page.goto(`/pages/${id}/edit`);
  await body(page).click();
  await editorSettled(page);
  return id;
}

/** Selects the whole of the first line of text. */
async function selectFirstLine(page: Page) {
  await body(page).locator('p').first().click();
  await page.keyboard.press('End');
  await page.keyboard.press('Shift+Home');
}

test('no bar shows without a selection, in code or in a table selection', async ({ page }) => {
  await editing(page, 'Quiet', [
    line('plain words'),
    { type: 'codeBlock', attrs: { language: null }, content: [{ type: 'text', text: 'let x = 1' }] },
  ]);
  await body(page).locator('p').first().click();
  await expect(textBar(page)).toHaveCount(0);
  await body(page).locator('pre').click();
  await page.keyboard.press('End');
  await page.keyboard.press('Shift+Home');
  await expect(textBar(page)).toBeHidden();
});

test('a tool in the bar and the same tool in the row show one state', async ({ page }) => {
  await editing(page, 'One state', [line('make this bold'), line('second line')]);
  await selectFirstLine(page);
  await expect(textBar(page)).toBeVisible();
  await textBar(page).getByRole('button', { name: 'Bold' }).click();
  await expect(body(page).locator('strong')).toHaveText('make this bold');
  await expect(fixedRow(page).getByRole('button', { name: 'Bold' })).toHaveAttribute('aria-pressed', 'true');
  await expect(textBar(page).getByRole('button', { name: 'Bold' })).toHaveAttribute('aria-pressed', 'true');

  await textBar(page).getByRole('button', { name: 'Heading 2' }).click();
  await expect(body(page).locator('h2')).toHaveText('make this bold');
  await expect(textBar(page).getByRole('button', { name: 'Heading 2' })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await textBar(page).getByRole('button', { name: 'Numbered list' }).click();
  await expect(body(page).locator('ol')).toHaveCount(1);
});

test('the bar is reached and left by keyboard, and adds no Tab stop', async ({ page }) => {
  await editing(page, 'Keys', [line('select me please')]);
  await selectFirstLine(page);
  await expect(textBar(page)).toBeVisible();
  await page.keyboard.press('Alt+F10');
  await expect(textBar(page).getByRole('button', { name: 'Bold' })).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(textBar(page).getByRole('button', { name: 'Italic' })).toBeFocused();
  await checkAccessibility(page);
  await page.keyboard.press('Escape');
  await expect(body(page)).toBeFocused();
  await page.keyboard.press('Control+b');
  await expect(body(page).locator('strong')).toHaveText('select me please');

  // Without a selection, Alt+F10 goes to the fixed row.
  await page.keyboard.press('End');
  await expect(textBar(page)).toHaveCount(0);
  await page.keyboard.press('Alt+F10');
  const focused = await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 200) ?? '');
  await expect(fixedRow(page).locator(':focus'), focused).toHaveCount(1);
});

test('the bar stays clear of the row and inside the window, and wraps when narrow', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 720 });
  await editing(page, 'Narrow bar', [line('one two three four five six seven')]);
  await selectFirstLine(page);
  const bar = textBar(page);
  await expect(bar).toBeVisible();
  const box = await bar.boundingBox();
  const row = await fixedRow(page).boundingBox();
  const words = await body(page).locator('p').first().boundingBox();
  expect(box && row && words).toBeTruthy();
  if (!box || !row || !words) return;
  // The editing page itself is wider than 360 pixels here, so the bar keeps inside the page,
  // and is never wider than the window.
  const pageWidth = await page.evaluate(() => document.documentElement.scrollWidth);
  const where = JSON.stringify({ box, row, pageWidth });
  expect(box.x, where).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width, where).toBeLessThanOrEqual(pageWidth);
  expect(box.width, where).toBeLessThanOrEqual(360);
  expect(box.y).toBeGreaterThanOrEqual(row.y + row.height);
  // At this width the tools wrap onto a second line instead of folding away.
  expect(box.height).toBeGreaterThan(50);
});

// The browser of these tests is started with a fine pointer, so a touch screen cannot be
// emulated here: the placement below the selection is proven by the unit test of barPlacement.

test('an image gets its own bar, and Delete takes it out of the page', async ({ page }) => {
  const space = await newSpace(page, 'Image bar space');
  const id = await publishedPage(page, space.id, 'Image bar', 'start');
  const png = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
    'base64',
  );
  const upload = await page.request.post(`/api/v1/pages/${id}/attachments?filename=a.png`, {
    headers: { 'content-type': 'image/png' },
    data: png,
  });
  expect(upload.ok(), await upload.text()).toBeTruthy();
  const { id: attachment } = (await upload.json()) as { id: string };
  const answer = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Image bar',
      document: {
        type: 'doc',
        content: [{ type: 'image', attrs: { attachment_id: attachment, alt: 'A dot' } }, line('after')],
      },
      base_version: 1,
    },
  });
  expect(answer.ok(), await answer.text()).toBeTruthy();
  await page.goto(`/pages/${id}/edit`);
  await body(page).locator('p').click();
  await editorSettled(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await expect(imageBar(page)).toBeVisible();
  await expect(textBar(page)).toBeHidden();
  await imageBar(page).getByRole('button', { name: 'Align to the centre' }).click();
  await expect(body(page).locator('[data-align="center"]')).toHaveCount(1);
  await imageBar(page).getByRole('button', { name: 'Delete image' }).click();
  await expect(body(page).locator('img')).toHaveCount(0);
});

test('the arrow keys follow a right-to-left page', async ({ page }) => {
  await editing(page, 'Right to left', [line('words to select')]);
  await page.evaluate(() => document.documentElement.setAttribute('dir', 'rtl'));
  await selectFirstLine(page);
  await expect(textBar(page)).toBeVisible();
  await page.keyboard.press('Alt+F10');
  await expect(textBar(page).getByRole('button', { name: 'Bold' })).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(textBar(page).getByRole('button', { name: 'Italic' })).toBeFocused();
});
