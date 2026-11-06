// SPDX-License-Identifier: Apache-2.0
// Spec 020, J1: the small things a writer feels every day.
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

const paragraph = (text: string) =>
  text ? { type: 'paragraph', content: [{ type: 'text', text }] } : { type: 'paragraph' };
const body = (page: Page) => page.locator('.prose.editing');
const title = (page: Page) => page.getByRole('textbox', { name: 'Title' });

async function editing(page: Page, name: string, content: unknown[]) {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, `${name} space`);
  const id = await publishedPage(page, space.id, name, 'x');
  const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title: name, document: { type: 'doc', content }, base_version: 1 },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  await page.goto(`/pages/${id}/edit`);
  await expect(body(page)).toBeVisible();
  return id;
}

async function intoBody(page: Page) {
  await body(page).click();
  await editorSettled(page);
}

async function clickBelowTheText(page: Page) {
  const area = page.locator('.editor-area');
  const box = await area.boundingBox();
  await area.click({ position: { x: 40, y: (box?.height ?? 0) - 10 } });
}

async function setTypingHelp(page: Page, on: boolean) {
  const current = await page.request.get('/api/v1/me');
  const me = (await current.json()) as { preferences: Record<string, unknown> };
  const res = await page.request.put('/api/v1/me/preferences', {
    data: { ...me.preferences, typing_help: on },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

test.describe('the title and the text', () => {
  test('a new page starts in the title, and Enter moves to the text', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'New title space');
    const res = await page.request.post('/api/v1/pages', {
      data: { space_id: space.id, parent_id: null, title: '', untitled: true },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    const { id } = (await res.json()) as { id: string };
    await page.goto(`/pages/${id}/edit`);
    await expect(title(page)).toBeFocused();
    await page.keyboard.type('My title');
    await page.keyboard.press('Enter');
    await editorSettled(page);
    await page.keyboard.type('First words');
    await expect(body(page)).toContainText('First words');
    await expect(title(page)).toHaveValue('My title');
  });

  test('Backspace at the very start goes back to the title and removes nothing', async ({ page }) => {
    await editing(page, 'Back to title', [paragraph('Some words')]);
    await intoBody(page);
    await page.keyboard.press('Control+Home');
    await editorSettled(page);
    await page.keyboard.press('Backspace');
    await expect(title(page)).toBeFocused();
    await expect(body(page)).toContainText('Some words');
  });

  test('Backspace in the middle of the text stays in the text', async ({ page }) => {
    await editing(page, 'Stay in text', [paragraph('Some words')]);
    await intoBody(page);
    await page.keyboard.press('Control+End');
    await page.keyboard.press('Backspace');
    await expect(body(page)).toContainText('Some word');
    await expect(body(page)).not.toContainText('Some words');
    await expect(body(page)).toBeFocused();
  });

  test('a page with a title does not take the focus for it', async ({ page }) => {
    await editing(page, 'Has a title', [paragraph('Some words')]);
    await expect(title(page)).not.toBeFocused();
  });
});

test.describe('below the text', () => {
  test('a click below the end goes on after the last word', async ({ page }) => {
    await editing(page, 'Below text', [paragraph('Last line')]);
    await clickBelowTheText(page);
    await editorSettled(page);
    await page.keyboard.type(' goes on');
    await expect(body(page)).toContainText('Last line goes on');
  });

  test('after a block that is not text it starts a new paragraph', async ({ page }) => {
    await editing(page, 'Below code', [
      { type: 'codeBlock', content: [{ type: 'text', text: 'let x = 1' }] },
    ]);
    await clickBelowTheText(page);
    await editorSettled(page);
    await page.keyboard.type('After the code');
    await expect(body(page).locator('p', { hasText: 'After the code' })).toBeVisible();
    await expect(body(page).locator('pre')).not.toContainText('After the code');
  });
});

test.describe('word count', () => {
  test('counts the text on request, and the selection when there is one', async ({ page }) => {
    await editing(page, 'Counting', [paragraph('one two three four')]);
    const count = page.getByRole('status').filter({ hasText: /word/ });
    await expect(count).toHaveCount(0);
    await page.getByRole('button', { name: 'Word count' }).click();
    await expect(count).toHaveText('4 words');
    await intoBody(page);
    await page.keyboard.press('Control+Home');
    await page.keyboard.press('Shift+Control+ArrowRight');
    await expect(count).toHaveText('1 of 4 words selected');
    await checkAccessibility(page);
  });
});

test.describe('typing help', () => {
  test('makes arrows, an ellipsis, curly quotes and an emoji', async ({ page }) => {
    await editing(page, 'Typing', [paragraph('')]);
    await intoBody(page);
    await page.keyboard.type(`a -> b <- c... "it's" :) ok`);
    await expect(body(page)).toContainText('a → b ← c… “it’s” 🙂 ok');
  });

  test('a face with a letter waits for the space', async ({ page }) => {
    await editing(page, 'Face', [paragraph('')]);
    await intoBody(page);
    await page.keyboard.type('nice :D ');
    await expect(body(page)).toContainText('nice 😀');
  });

  test('leaves addresses and times alone', async ({ page }) => {
    await editing(page, 'Times', [paragraph('')]);
    await intoBody(page);
    await page.keyboard.type('at 10:30) and a:)');
    await expect(body(page)).toContainText('at 10:30) and a:)');
  });

  test('stays out of code', async ({ page }) => {
    await editing(page, 'In code', [{ type: 'codeBlock' }]);
    await body(page).locator('pre').click();
    await editorSettled(page);
    await page.keyboard.type(`a -> "b" ...`);
    await expect(body(page).locator('pre')).toHaveText(`a -> "b" ...`);
  });

  test('Backspace right after it brings back what was typed', async ({ page }) => {
    await editing(page, 'Undo typing', [paragraph('')]);
    await intoBody(page);
    await page.keyboard.type('a ->');
    await expect(body(page)).toContainText('a →');
    await page.keyboard.press('Backspace');
    await expect(body(page)).toContainText('a ->');
  });

  test('a preference turns it off', async ({ page }) => {
    await editing(page, 'Help off', [paragraph('')]);
    await setTypingHelp(page, false);
    await page.reload();
    await intoBody(page);
    await page.keyboard.type('a -> b :)');
    await expect(body(page)).toContainText('a -> b :)');
    await setTypingHelp(page, true);
  });
});

test.describe('shortcuts that come with the editor', () => {
  test('Backspace after a shortcut undoes it, also at the very start', async ({ page }) => {
    await editing(page, 'Undo shortcut', [paragraph('')]);
    await intoBody(page);
    await page.keyboard.type('## ');
    await expect(body(page).locator('h2')).toBeVisible();
    await page.keyboard.press('Backspace');
    await expect(body(page).locator('h2')).toHaveCount(0);
    await expect(body(page)).toContainText('##');
    await expect(body(page)).toBeFocused();
  });

  test('Backspace after a list shortcut brings back what was typed', async ({ page }) => {
    await editing(page, 'Undo list', [paragraph('First')]);
    await intoBody(page);
    await page.keyboard.press('Control+End');
    await page.keyboard.press('Enter');
    await page.keyboard.type('- ');
    await expect(body(page).locator('ul')).toBeVisible();
    await page.keyboard.press('Backspace');
    await expect(body(page).locator('ul')).toHaveCount(0);
    await expect(body(page)).toContainText('-');
  });

  test('a list becomes a task list and back, and keeps its text and nesting', async ({ page }) => {
    const item = (text: string, more: unknown[] = []) => ({
      type: 'listItem',
      content: [paragraph(text), ...more],
    });
    await editing(page, 'Convert list', [
      { type: 'bulletList', content: [item('top', [{ type: 'bulletList', content: [item('inner')] }])] },
    ]);
    await body(page).getByText('top').click();
    await editorSettled(page);
    await page.keyboard.press('Control+Shift+9');
    await expect(body(page).locator('ul[data-type=taskList] ul')).toContainText('inner');
    await page.keyboard.press('Control+Shift+8');
    await expect(body(page).locator('ul ul')).toContainText('inner');
    await expect(body(page)).toContainText('top');
  });

  test('pasting an address over selected text makes a link', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Paste link', [paragraph('Read this')]);
    await intoBody(page);
    await page.evaluate(() => navigator.clipboard.writeText('https://example.com/read'));
    await page.keyboard.press('Control+End');
    await page.keyboard.press('Shift+Control+ArrowLeft');
    await editorSettled(page);
    await page.keyboard.press('Control+v');
    await expect(body(page).locator('a[href="https://example.com/read"]')).toHaveText('this');
  });

  test('code pasted into a code block stays exactly as it is', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Paste code', [{ type: 'codeBlock' }]);
    await body(page).locator('pre').click();
    await editorSettled(page);
    const code = '# heading\n  - item\n    **x** -> "y"';
    await page.evaluate((text) => navigator.clipboard.writeText(text), code);
    await page.keyboard.press('Control+v');
    await expect(body(page).locator('pre')).toHaveText(code);
  });

  test('Ctrl Shift V pastes without formatting', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Plain paste', [paragraph('')]);
    await intoBody(page);
    await page.evaluate(async () => {
      const html = new Blob(['<h2>Big</h2>'], { type: 'text/html' });
      const text = new Blob(['Big'], { type: 'text/plain' });
      await navigator.clipboard.write([new ClipboardItem({ 'text/html': html, 'text/plain': text })]);
    });
    await page.keyboard.press('Control+Shift+v');
    await expect(body(page)).toContainText('Big');
    await expect(body(page).locator('h2')).toHaveCount(0);
  });

  test('Tab in the last cell adds a row', async ({ page }) => {
    const row = (type: string, text: string) => ({
      type: 'tableRow',
      content: [{ type, content: [paragraph(text)] }],
    });
    await editing(page, 'Tab row', [
      { type: 'table', content: [row('tableHeader', 'Name'), row('tableCell', 'Robin')] },
    ]);
    await body(page).locator('td').first().click();
    await editorSettled(page);
    await page.keyboard.press('Tab');
    await expect(body(page).locator('tr')).toHaveCount(3);
  });
});

test.describe('the / menu', () => {
  test('has group headings, and what was used lately comes first', async ({ page }) => {
    await editing(page, 'Slash menu', [paragraph('')]);
    await intoBody(page);
    await page.keyboard.type('/');
    const list = page.getByRole('listbox', { name: 'Insert' });
    await expect(list.getByRole('group', { name: 'Text and lists' })).toBeVisible();
    await expect(list.getByRole('group', { name: 'Used lately' })).toHaveCount(0);
    await checkAccessibility(page);
    await page.keyboard.type('quote');
    await page.keyboard.press('Enter');
    await expect(body(page).locator('blockquote')).toBeVisible();
    await page.keyboard.press('Enter');
    await page.keyboard.press('Enter');
    await page.keyboard.type('/');
    await expect(list.getByRole('option').first()).toContainText('Quote');
    await expect(list.getByRole('group', { name: 'Used lately' })).toBeVisible();
  });
});

test.describe('find', () => {
  test('Whole word skips matches inside longer words', async ({ page }) => {
    await editing(page, 'Whole word', [paragraph('cat concat cats cat')]);
    await intoBody(page);
    await page.keyboard.press('Control+f');
    await page.getByRole('textbox', { name: 'Find' }).fill('cat');
    await expect(page.locator('.find-count')).toContainText('of 4');
    await page.getByRole('checkbox', { name: 'Whole word' }).check();
    await expect(page.locator('.find-count')).toContainText('of 2');
  });
});

test.describe('pasted Markdown', () => {
  const pasteText = async (page: Page, text: string) => {
    await page.evaluate((value) => navigator.clipboard.writeText(value), text);
    await page.keyboard.press('Control+v');
  };

  test('becomes blocks, and Undo brings back the plain text', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Paste markdown', [paragraph('')]);
    await intoBody(page);
    await pasteText(page, '## Plan\n\n- one\n- two');
    await expect(body(page).locator('h2')).toHaveText('Plan');
    await expect(body(page).locator('ul li')).toHaveCount(2);
    await page.keyboard.press('Control+z');
    await expect(body(page).locator('h2')).toHaveCount(0);
    await expect(body(page)).toContainText('## Plan');
  });

  test('stays plain text when the server does not answer', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Paste offline', [paragraph('')]);
    await intoBody(page);
    await page.route('**/api/v1/markdown/document', (route) => route.abort());
    await pasteText(page, '## Plan');
    await expect(body(page)).toContainText('## Plan');
    await expect(body(page).locator('h2')).toHaveCount(0);
  });

  test('leaves plain sentences alone, and asks the server nothing', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Paste plain', [paragraph('')]);
    await intoBody(page);
    let asked = false;
    await page.route('**/api/v1/markdown/document', (route) => {
      asked = true;
      return route.continue();
    });
    await pasteText(page, 'Just a sentence.');
    await expect(body(page)).toContainText('Just a sentence.');
    expect(asked).toBe(false);
  });

  test('Ctrl Shift V keeps it plain', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await editing(page, 'Paste plain md', [paragraph('')]);
    await intoBody(page);
    await page.evaluate(() => navigator.clipboard.writeText('## Plan'));
    await page.keyboard.press('Control+Shift+v');
    await expect(body(page)).toContainText('## Plan');
    await page.waitForTimeout(300);
    await expect(body(page).locator('h2')).toHaveCount(0);
  });
});

test.describe('links that lead nowhere', () => {
  const linkTo = (id: string, text: string) => ({
    type: 'paragraph',
    content: [{ type: 'text', text, marks: [{ type: 'link', attrs: { href: `/pages/${id}` } }] }],
  });

  test('a deleted page is struck through and says why, a live one is not', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Links space');
    const live = await publishedPage(page, space.id, 'Live', 'x');
    const gone = await publishedPage(page, space.id, 'Gone', 'x');
    const id = await publishedPage(page, space.id, 'Source', 'x');
    const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
      data: {
        title: 'Source',
        document: { type: 'doc', content: [linkTo(live, 'to live'), linkTo(gone, 'to gone')] },
        base_version: 1,
      },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    const removed = await page.request.delete(`/api/v1/pages/${gone}`);
    expect(removed.ok(), await removed.text()).toBeTruthy();
    await page.goto(`/pages/${id}`);
    const prose = page.locator('.prose');
    await expect(prose.locator('.link-gone')).toHaveText('to gone');
    await expect(prose.locator('.link-gone')).toHaveAttribute(
      'title',
      'This page does not exist, or you cannot see it.',
    );
    await expect(prose.getByRole('link', { name: 'to live' }).locator('.link-gone')).toHaveCount(0);
    await checkAccessibility(page);
  });
});

test.describe('a table for a reader', () => {
  const cell = (type: string, text: string) => ({ type, content: [paragraph(text)] });
  const row = (type: string, ...texts: string[]) => ({
    type: 'tableRow',
    content: texts.map((text) => cell(type, text)),
  });
  const names = ['item 10', 'item 2', 'Robin', 'Sam', 'Tess', 'Uma', 'Vic'];

  async function reading(page: Page, rows: string[]) {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Reader tables');
    const id = await publishedPage(page, space.id, 'Reader table', 'x');
    const content = [
      {
        type: 'table',
        content: [row('tableHeader', 'Name', 'Team'), ...rows.map((name) => row('tableCell', name, 'Docs'))],
      },
    ];
    const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
      data: { title: 'Reader table', document: { type: 'doc', content }, base_version: 1 },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    await page.goto(`/pages/${id}`);
    return id;
  }

  const firstColumn = (page: Page) => page.locator('.prose td:first-child');

  test('sorts by a column with a real button, up and then down', async ({ page }) => {
    await reading(page, names);
    const header = page.getByRole('columnheader', { name: 'Name' });
    const sort = page.getByRole('button', { name: 'Sort by Name' });
    await expect(header).not.toHaveAttribute('aria-sort', /.*/);
    await sort.click();
    await expect(header).toHaveAttribute('aria-sort', 'ascending');
    await expect(firstColumn(page).first()).toHaveText('item 2');
    await expect(page.getByRole('button', { name: 'Sort by Name' })).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(header).toHaveAttribute('aria-sort', 'descending');
    await expect(firstColumn(page).first()).toHaveText('Vic');
    await checkAccessibility(page);
  });

  test('filters rows and says how many are left', async ({ page }) => {
    await reading(page, names);
    await page.getByRole('searchbox', { name: 'Filter this table' }).fill('item');
    await expect(firstColumn(page).filter({ visible: true })).toHaveCount(2);
    await expect(page.getByRole('status').filter({ hasText: 'rows' })).toHaveText('2 of 7 rows');
    await expect(page.getByRole('searchbox', { name: 'Filter this table' })).toBeFocused();
    await page.getByRole('searchbox', { name: 'Filter this table' }).fill('');
    await expect(firstColumn(page).filter({ visible: true })).toHaveCount(7);
  });

  test('a short table has no filter box', async ({ page }) => {
    await reading(page, ['one', 'two']);
    await expect(page.getByRole('button', { name: 'Sort by Name' })).toBeVisible();
    await expect(page.getByRole('searchbox', { name: 'Filter this table' })).toHaveCount(0);
  });

  test('sorting changes nothing stored', async ({ page }) => {
    const id = await reading(page, names);
    const writes: string[] = [];
    page.on('request', (r) => {
      if (/\/(publish|draft)$/.test(r.url()) && r.url().includes(id)) writes.push(r.url());
    });
    await page.getByRole('button', { name: 'Sort by Name' }).click();
    await expect(firstColumn(page).first()).toHaveText('item 2');
    expect(writes).toEqual([]);
    await page.reload();
    await expect(firstColumn(page).first()).toHaveText('item 10');
  });

  test('a long table keeps its header in view', async ({ page }) => {
    await reading(
      page,
      Array.from({ length: 40 }, (_, at) => `row ${at}`),
    );
    const frame = page.locator('.tableWrapper.table-long');
    await frame.evaluate((element) => element.scrollTo(0, 400));
    const header = await page.getByRole('columnheader', { name: 'Name' }).boundingBox();
    const box = await frame.boundingBox();
    expect(Math.abs((header?.y ?? 0) - (box?.y ?? 0))).toBeLessThan(4);
  });

  test('the editor shows none of this', async ({ page }) => {
    const id = await reading(page, names);
    await page.goto(`/pages/${id}/edit`);
    await expect(body(page).locator('td').first()).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sort by Name' })).toHaveCount(0);
  });
});

test.describe('files in the text', () => {
  const PNG = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
    'base64',
  );

  async function upload(page: Page, id: string, name: string, data: Buffer | string) {
    const res = await page.request.post(`/api/v1/pages/${id}/attachments?filename=${name}`, { data });
    expect(res.status(), await res.text()).toBe(201);
    return (await res.json()) as { id: string };
  }

  test('a file on the page can be added again without an upload', async ({ page }) => {
    const id = await editing(page, 'Pick file', [paragraph('')]);
    await upload(page, id, 'plan.txt', 'hello');
    await intoBody(page);
    await page.keyboard.type('/file on this');
    await page.keyboard.press('Enter');
    await page
      .getByRole('dialog')
      .getByRole('button', { name: /plan\.txt/ })
      .click();
    await expect(body(page).locator('.file-name')).toHaveText('plan.txt');
    const files = await page.request.get(`/api/v1/pages/${id}/attachments`);
    expect(((await files.json()) as { attachments: unknown[] }).attachments).toHaveLength(1);
  });

  test('a tile shows the type and size, and a deleted file says so', async ({ page }) => {
    const id = await editing(page, 'Tiles', [paragraph('x')]);
    const kept = await upload(page, id, 'plan.txt', 'hello');
    const gone = await upload(page, id, 'old.txt', 'bye');
    const tile = (file: { id: string }, filename: string) => ({
      type: 'file',
      attrs: { attachment_id: file.id, filename },
    });
    const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
      data: {
        title: 'Tiles',
        document: { type: 'doc', content: [tile(kept, 'plan.txt'), tile(gone, 'old.txt')] },
        base_version: 2,
      },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    const removed = await page.request.delete(`/api/v1/attachments/${gone.id}`);
    expect(removed.ok(), await removed.text()).toBeTruthy();
    await page.goto(`/pages/${id}`);
    await expect(page.getByRole('link', { name: /plan\.txt/ })).toContainText('TXT · 5 B');
    await expect(page.getByText('This file is no longer available.')).toBeVisible();
    await expect(page.getByRole('link', { name: /old\.txt/ })).toHaveCount(0);
    await checkAccessibility(page);
  });

  test('a missing image shows a clear placeholder', async ({ page }) => {
    const id = await editing(page, 'Missing image', [paragraph('x')]);
    const picture = await upload(page, id, 'pic.png', PNG);
    const image = { type: 'image', attrs: { attachment_id: picture.id, alt: 'A chart' } };
    const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
      data: { title: 'Missing image', document: { type: 'doc', content: [image] }, base_version: 2 },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    const removed = await page.request.delete(`/api/v1/attachments/${picture.id}`);
    expect(removed.ok(), await removed.text()).toBeTruthy();
    await page.goto(`/pages/${id}`);
    await expect(page.getByText('A chart: This image is no longer available.')).toBeVisible();
    await expect(page.locator('.prose img')).toHaveCount(0);
  });

  test('a file moves to another page from its details, and the tile keeps working', async ({ page }) => {
    const from = await editing(page, 'Move from', [paragraph('x')]);
    const details = (await (await page.request.get(`/api/v1/pages/${from}`)).json()) as { space_id: string };
    const to = await publishedPage(page, details.space_id, 'Move target', 'x');
    const file = await upload(page, from, 'plan.txt', 'hello');
    const tile = { type: 'file', attrs: { attachment_id: file.id, filename: 'plan.txt' } };
    const res = await page.request.post(`/api/v1/pages/${from}/publish`, {
      data: { title: 'Move from', document: { type: 'doc', content: [tile] }, base_version: 2 },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    await page.goto(`/pages/${from}`);
    await page.getByRole('button', { name: 'More actions' }).click();
    await page.getByRole('menuitem', { name: 'Attachments' }).click();
    await page.getByRole('button', { name: 'Details' }).click();
    await page.getByLabel('Find a page').fill('Move target');
    await page.getByRole('dialog').getByRole('button', { name: 'Move target' }).click();
    await page.getByRole('button', { name: 'Move here' }).click();
    await expect(page.getByText('plan.txt')).toHaveCount(1);
    const after = await page.request.get(`/api/v1/pages/${to}/attachments`);
    expect(((await after.json()) as { attachments: unknown[] }).attachments).toHaveLength(1);
    await page.goto(`/pages/${from}`);
    await expect(page.getByRole('link', { name: /plan\.txt/ })).toContainText('TXT');
  });
});

test('an administrator blocks a file type, and the upload says why', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const settings = (await (await page.request.get('/api/v1/admin/settings')).json()) as Record<
    string,
    unknown
  >;
  await page.goto('/admin/settings');
  await page.getByLabel('Blocked file types').fill('exe,bat');
  await page.getByRole('button', { name: 'Save' }).first().click();
  await expect(page.getByText('Saved', { exact: false }).first()).toBeVisible();
  const space = await newSpace(page, 'Blocked');
  const id = await publishedPage(page, space.id, 'Blocked page', 'x');
  const res = await page.request.post(`/api/v1/pages/${id}/attachments?filename=setup.EXE`, { data: 'x' });
  expect(res.status()).toBe(400);
  expect(await res.text()).toContain('not allowed');
  const undone = await page.request.put('/api/v1/admin/settings', {
    data: { ...settings, blocked_file_types: [] },
  });
  expect(undone.ok(), await undone.text()).toBeTruthy();
});

test('the body asks the browser to check spelling', async ({ page }) => {
  await editing(page, 'Spelling', [paragraph('text')]);
  await expect(body(page)).toHaveAttribute('spellcheck', 'true');
});
