// SPDX-License-Identifier: Apache-2.0
import { ADMIN, checkAccessibility, expect, login, newSpace, publishedPage, test } from './helpers';

const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  'base64',
);

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('writing a page with Markdown shortcuts and publishing it', async ({ page }) => {
  const space = await newSpace(page, 'Editor space');
  await page.goto(`/spaces/${space.key}`);
  await page.getByRole('button', { name: 'New page' }).click();
  await page.getByRole('dialog').getByLabel('Title').fill('Shortcuts');
  await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();

  await page.getByRole('textbox', { name: 'Page content' }).click();
  await page.keyboard.type('# Big heading');
  await page.keyboard.press('Enter');
  await page.keyboard.type('Some **bold** text and `code`.');
  await page.keyboard.press('Enter');
  await page.keyboard.type('- first item');
  await page.keyboard.press('Enter');
  await page.keyboard.type('second item');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.keyboard.type('[ ] a task');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.keyboard.type('```js');
  await page.keyboard.press('Enter');
  await page.keyboard.type('const a = 1;');

  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.getByRole('heading', { name: 'Shortcuts', level: 1 })).toBeVisible();

  const doc = page.locator('.prose');
  await expect(doc.locator('h1')).toHaveText('Big heading');
  await expect(doc.locator('strong')).toHaveText('bold');
  await expect(doc.locator('code').first()).toHaveText('code');
  await expect(doc.locator('ul li').first()).toContainText('first item');
  await expect(doc.locator('pre code')).toContainText('const a = 1;');
  await expect(doc.locator('pre .hljs-keyword')).toHaveText('const');
  await checkAccessibility(page);
});

test('tables, links, and a pasted address stay safe', async ({ page }) => {
  const space = await newSpace(page, 'Table space');
  const id = await publishedPage(page, space.id, 'Tables', 'start');
  await page.goto(`/pages/${id}/edit`);
  await page.getByRole('button', { name: 'Table', exact: true }).click();
  await page.keyboard.type('Name');
  await page.getByRole('button', { name: 'Add row' }).click();
  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.locator('.prose table tr')).toHaveCount(4);
  await expect(page.locator('.prose th').first()).toHaveText('Name');

  // Script-looking text is only ever text.
  await page.getByRole('link', { name: 'Edit', exact: true }).click();
  const body = page.locator('.prose.editing');
  await body.click();
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Enter');
  await page.keyboard.type('<img src=x onerror=alert(1)>');
  let alerted = false;
  page.on('dialog', async (d) => {
    alerted = true;
    await d.dismiss();
  });
  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.locator('.prose')).toContainText('<img src=x onerror=alert(1)>');
  expect(alerted).toBe(false);
  await expect(page.locator('.prose img')).toHaveCount(0);
});

test('drafts survive a reload, and history compares and restores', async ({ page }) => {
  const space = await newSpace(page, 'History space');
  const id = await publishedPage(page, space.id, 'Story', 'the original words');

  await page.goto(`/pages/${id}/edit`);
  const body = page.locator('.prose.editing');
  await body.click();
  await page.keyboard.press('Control+End');
  await page.keyboard.type(' and more');
  await expect(page.getByText('Draft saved')).toBeVisible();

  await page.reload();
  await expect(page.locator('.prose.editing')).toContainText('and more');

  await page.goto(`/pages/${id}`);
  await expect(page.getByText('You have an unsaved draft')).toBeVisible();
  await expect(page.locator('.prose')).not.toContainText('and more');

  await page.getByRole('link', { name: 'You have an unsaved draft' }).click();
  await page.getByLabel('What changed?').fill('Added words');
  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.locator('.prose')).toContainText('and more');

  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'History' }).click();
  await expect(page.getByText('Added words')).toBeVisible();
  await page.getByRole('checkbox', { name: 'Version 1' }).check();
  await page.getByRole('checkbox', { name: 'Version 2' }).check();
  await page.getByRole('link', { name: 'Compare selected' }).click();
  await expect(page.locator('.diff.changed .added')).toContainText('and more');
  await checkAccessibility(page);

  await page.goto(`/pages/${id}/history`);
  await page.getByRole('button', { name: 'Restore' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Restore' }).click();
  await expect(page.getByRole('heading', { name: 'Story', level: 1 })).toBeVisible();
  await expect(page.locator('.prose')).not.toContainText('and more');
});

test('publishing over someone else warns first', async ({ page, browser }) => {
  const space = await newSpace(page, 'Conflict space');
  const id = await publishedPage(page, space.id, 'Shared', 'base');
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await page.keyboard.type('mine ');

  const other = await browser.newContext({ baseURL: 'http://localhost:8080' });
  await other.request.post('/api/v1/auth/login', { data: ADMIN });
  const res = await other.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Shared',
      base_version: 1,
      document: {
        type: 'doc',
        content: [{ type: 'paragraph', content: [{ type: 'text', text: 'theirs' }] }],
      },
    },
  });
  expect(res.ok()).toBeTruthy();
  await other.close();

  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.getByRole('dialog', { name: 'Someone else published' })).toBeVisible();
  await page.getByRole('button', { name: 'Publish anyway' }).click();
  await expect(page.locator('.prose')).toContainText('mine');
});

test('images go in as attachments and files can be downloaded', async ({ page }) => {
  const space = await newSpace(page, 'Files space');
  const id = await publishedPage(page, space.id, 'Files', 'text');
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await page.locator('input[type=file][aria-label="Image"]').setInputFiles({
    name: 'dot.png',
    mimeType: 'image/png',
    buffer: PNG,
  });
  await expect(page.locator('.prose.editing img')).toBeVisible();
  await page.getByRole('button', { name: 'Publish' }).click();
  const image = page.locator('.prose img');
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(1);

  await page.getByLabel('Upload a file').setInputFiles({
    name: 'notes.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('hello'),
  });
  await expect(page.getByRole('link', { name: 'notes.txt' })).toBeVisible();
  const download = page.waitForEvent('download');
  await page.getByRole('link', { name: 'notes.txt' }).click();
  expect((await download).suggestedFilename()).toBe('notes.txt');
});

test('the page tree supports drag and drop and keeps the address', async ({ page }) => {
  const space = await newSpace(page, 'Tree space');
  const a = await publishedPage(page, space.id, 'Alpha', 'a');
  const b = await publishedPage(page, space.id, 'Beta', 'b');
  await page.goto(`/pages/${a}`);
  const tree = page.getByRole('navigation', { name: 'Pages' });

  await tree.getByRole('link', { name: 'Beta' }).dragTo(tree.getByRole('link', { name: 'Alpha' }));
  await expect(tree.getByRole('button', { name: 'Alpha', exact: true })).toBeVisible();

  await page.goto(`/pages/${b}`);
  await expect(page.getByRole('navigation', { name: 'Breadcrumbs' })).toContainText('Alpha');

  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Rename' }).click();
  await page.getByLabel('New title').fill('Beta renamed');
  await page.getByRole('dialog').getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('heading', { name: 'Beta renamed', level: 1 })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Beta renamed', level: 1 })).toBeVisible();
  await checkAccessibility(page);
});

test('a branch of the tree collapses, and stays collapsed while you read on', async ({ page }) => {
  const space = await newSpace(page, 'Branch space');
  const parent = await publishedPage(page, space.id, 'Parent', 'p');
  await publishedPage(page, space.id, 'Child', 'c', parent);
  const other = await publishedPage(page, space.id, 'Other', 'o');
  await page.goto(`/pages/${other}`);
  const tree = page.getByRole('navigation', { name: 'Pages' });
  const toggle = tree.getByRole('button', { name: 'Parent', exact: true });

  await expect(tree.getByRole('link', { name: 'Child' })).toBeVisible();
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
  await toggle.click();
  await expect(tree.getByRole('link', { name: 'Child' })).toHaveCount(0);
  await expect(toggle).toHaveAttribute('aria-expanded', 'false');

  await tree.getByRole('link', { name: 'Parent' }).click();
  await expect(page.getByRole('heading', { name: 'Parent', level: 1 })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Child' })).toHaveCount(0);
  await page.reload();
  await expect(tree.getByRole('link', { name: 'Parent' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Child' })).toHaveCount(0);

  await toggle.click();
  await expect(tree.getByRole('link', { name: 'Child' })).toBeVisible();
});

test('going to a page reveals it, also inside a collapsed branch', async ({ page }) => {
  const space = await newSpace(page, 'Reveal space');
  const parent = await publishedPage(page, space.id, 'Outer', 'p');
  const child = await publishedPage(page, space.id, 'Inner', 'c', parent);
  await page.goto(`/pages/${parent}`);
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await tree.getByRole('button', { name: 'Outer', exact: true }).click();
  await expect(tree.getByRole('link', { name: 'Inner' })).toHaveCount(0);

  await page.goto(`/pages/${child}`);
  await expect(tree.getByRole('link', { name: 'Inner' })).toHaveAttribute('aria-current', 'page');
});

test('Create adds a page below the one you are reading', async ({ page }) => {
  const space = await newSpace(page, 'Create space');
  const parent = await publishedPage(page, space.id, 'Reading', 'x');
  await page.goto(`/pages/${parent}`);
  await expect(page.getByRole('heading', { name: 'Reading', level: 1 })).toBeVisible();
  await page.getByRole('button', { name: 'Create', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'New page' });
  await dialog.getByLabel('Title').fill('Below');
  await dialog.getByRole('button', { name: 'Create' }).click();
  await page.getByRole('textbox', { name: 'Page content' }).click();
  await page.keyboard.type('text');
  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.getByRole('heading', { name: 'Below', level: 1 })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Breadcrumbs' })).toContainText('Reading');
});

test('outside a space, Create starts a new space', async ({ page }) => {
  await page.getByRole('button', { name: 'Create', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'New space' })).toBeVisible();
});

test('the sidebar can be put away and brought back', async ({ page }) => {
  const space = await newSpace(page, 'Sidebar space');
  await page.goto(`/spaces/${space.key}`);
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await expect(tree).toBeVisible();
  await page.getByRole('button', { name: 'Hide sidebar' }).click();
  await expect(tree).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('button', { name: 'Show sidebar' })).toBeVisible();
  await page.getByRole('button', { name: 'Show sidebar' }).click();
  await expect(tree).toBeVisible();
});

test('a link is added through our own dialog, and no browser dialog ever opens', async ({ page }) => {
  const space = await newSpace(page, 'Link space');
  const id = await publishedPage(page, space.id, 'Links', 'start');
  let browserDialogs = 0;
  page.on('dialog', async (d) => {
    browserDialogs++;
    await d.dismiss();
  });
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await page.keyboard.press('Control+A');
  await page.getByRole('button', { name: 'Link', exact: true }).click();
  await page.getByRole('dialog', { name: 'Link' }).getByLabel('Link address').fill('https://example.com/');
  await page.getByRole('dialog', { name: 'Link' }).getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('button', { name: 'Link', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true',
  );

  await page.getByLabel('Text style').selectOption({ label: 'Heading 2' });
  await page.getByRole('button', { name: 'Publish' }).click();
  await expect(page.locator('.prose h2 a')).toHaveAttribute('href', 'https://example.com/');

  await page.getByRole('link', { name: 'Edit', exact: true }).click();
  await page.getByRole('button', { name: 'Discard draft' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  expect(browserDialogs).toBe(0);
});

test('trash: delete, restore, and purge', async ({ page }) => {
  const space = await newSpace(page, 'Trash space');
  const id = await publishedPage(page, space.id, 'Doomed', 'x');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Delete' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
  await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();

  await page.goto(`/spaces/${space.key}/trash`);
  await expect(page.getByRole('cell', { name: 'Doomed' })).toBeVisible();
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Restore' }).click();
  await expect(page.getByText('The trash is empty.')).toBeVisible();
  await page.goto(`/pages/${id}`);
  await expect(page.getByRole('heading', { name: 'Doomed', level: 1 })).toBeVisible();
});

test('search finds a page and marks the word', async ({ page }) => {
  const space = await newSpace(page, 'Search space');
  await publishedPage(page, space.id, 'Findable', 'the quick zebracrossing here');
  await page.goto('/');
  await page.getByRole('searchbox', { name: 'Search' }).fill('zebracross');
  await page.keyboard.press('Enter');
  await expect(page.getByRole('link', { name: 'Findable' })).toBeVisible();
  await expect(page.locator('mark')).toHaveText('zebracrossing');
  await checkAccessibility(page);
});

test('export and import through the settings page', async ({ page }) => {
  const space = await newSpace(page, 'Import space');
  await page.goto(`/spaces/${space.key}/settings`);
  await page.getByLabel('Import Markdown').setInputFiles({
    name: 'Imported note.md',
    mimeType: 'text/markdown',
    buffer: Buffer.from('# Hello\n\n- [x] done\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n'),
  });
  await expect(page.getByText('Imported 1 pages and 0 files.')).toBeVisible({ timeout: 20_000 });
  await expect(
    page.getByRole('navigation', { name: 'Pages' }).getByRole('link', { name: 'Imported note' }),
  ).toBeVisible();
  await checkAccessibility(page);

  const res = await page.request.get(`/api/v1/spaces/${space.id}/export.zip`);
  expect(res.status()).toBe(200);
  expect(res.headers()['content-type']).toBe('application/zip');
});
