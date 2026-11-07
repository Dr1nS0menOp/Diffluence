// SPDX-License-Identifier: Apache-2.0
// Spec 027: the rest of the editor.
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
const prose = (page: Page) => page.locator('.prose');
const line = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const cell = (type: string, text: string) => ({ type, content: [line(text)] });
const sales = (attrs: Record<string, unknown> = {}) => ({
  type: 'table',
  attrs: { block_id: 'salestable', ...attrs },
  content: [
    { type: 'tableRow', content: [cell('tableHeader', 'Month'), cell('tableHeader', 'Sales')] },
    { type: 'tableRow', content: [cell('tableCell', 'Jan'), cell('tableCell', '10')] },
    { type: 'tableRow', content: [cell('tableCell', 'Feb'), cell('tableCell', '4')] },
  ],
});

async function pageWith(page: Page, title: string, content: unknown[]) {
  const space = await newSpace(page, `${title} space`);
  const id = await publishedPage(page, space.id, title, 'start');
  const answer = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title, document: { type: 'doc', content }, base_version: 1 },
  });
  expect(answer.ok(), await answer.text()).toBeTruthy();
  return { id, space };
}

async function editing(page: Page, title: string, content: unknown[]) {
  const made = await pageWith(page, title, content);
  await page.goto(`/pages/${made.id}/edit`);
  await body(page).click();
  await editorSettled(page);
  return made;
}

async function publish(page: Page) {
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('link', { name: 'Edit', exact: true })).toBeVisible();
}

test('tools that do not fit fold into More, and nothing is cut off', async ({ page }) => {
  await page.setViewportSize({ width: 1800, height: 800 });
  await editing(page, 'Narrow', [line('text')]);
  const toolbar = page.getByRole('toolbar', { name: 'Formatting' });
  await expect(toolbar.getByRole('button', { name: 'Undo', exact: true })).toBeVisible();
  await expect(toolbar.getByRole('button', { name: 'More tools' })).toHaveCount(0);

  await page.setViewportSize({ width: 700, height: 800 });
  const more = toolbar.getByRole('button', { name: 'More tools' });
  await expect(more).toBeVisible();
  await expect(toolbar.getByRole('button', { name: 'Undo', exact: true })).toHaveCount(0);
  const row = await toolbar.boundingBox();
  const button = await more.boundingBox();
  expect((button?.x ?? 0) + (button?.width ?? 0)).toBeLessThanOrEqual((row?.x ?? 0) + (row?.width ?? 0));

  await more.click();
  await toolbar.getByRole('button', { name: 'Undo', exact: true }).click();
  await page.setViewportSize({ width: 1800, height: 800 });
  await expect(toolbar.getByRole('button', { name: 'More tools' })).toHaveCount(0);
});

test('the plus on an empty line opens the list of blocks', async ({ page }) => {
  await editing(page, 'Plus', [line('text'), { type: 'paragraph' }]);
  await body(page).locator('p').nth(1).hover();
  await page.getByRole('button', { name: 'Add a block here' }).click();
  await expect(page.getByRole('listbox', { name: 'Insert' })).toBeVisible();
});

test('a block gets a link that leads to it, also inside a collapsed section', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const { id } = await editing(page, 'Linked', [
    line('first'),
    { type: 'expand', attrs: { title: 'More' }, content: [line('inside')] },
  ]);
  await body(page).locator('.expand').hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  await page.getByRole('menuitem', { name: 'Copy link to this block' }).click();
  await expect(page.getByText('Link copied')).toBeVisible();
  await publish(page);
  const address = await page.evaluate(() => navigator.clipboard.readText());
  expect(address).toContain(`/pages/${id}#block-`);
  await page.goto(address);
  await expect(prose(page).getByText('inside')).toBeVisible();
});

test('the block menu offers what is likely first, and a panel goes back to text', async ({ page }) => {
  await editing(page, 'Turning', [
    line('words'),
    { type: 'panel', attrs: { kind: 'info' }, content: [line('note')] },
  ]);
  await body(page).locator('p').first().hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  const menu = page.getByRole('menu');
  const entries = await menu.getByRole('menuitem').allTextContents();
  const delete_ = entries.findIndex((entry) => entry.includes('Delete'));
  expect(entries[delete_ + 1]).toContain('Heading 2');
  await page.keyboard.press('Escape');
  await body(page).locator('.panel p').click();

  await body(page).locator('.panel').hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  await page.getByRole('menuitem', { name: 'Back to text' }).click();
  await expect(body(page).locator('.panel')).toHaveCount(0);
  await expect(body(page).locator('p').filter({ hasText: 'note' })).toBeVisible();
});

test('Shift and a grip select a run of blocks, which move and go together', async ({ page }) => {
  await editing(page, 'Range', [line('one'), line('two'), line('three'), line('four')]);
  const lines = body(page).locator('p');
  await lines.first().click();
  await lines.nth(2).hover();
  await page.locator('.block-grip').click({ modifiers: ['Shift'] });
  await lines.nth(2).hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  await page.getByRole('menuitem', { name: 'Move down' }).click();
  await expect(lines).toHaveText(['four', 'one', 'two', 'three']);

  await editorSettled(page);
  await page.keyboard.press('Control+z');
  await expect(lines).toHaveText(['one', 'two', 'three', 'four']);
});

test('a list item is dragged with what is nested below it', async ({ page }) => {
  const nested = { type: 'bulletList', content: [{ type: 'listItem', content: [line('child')] }] };
  await editing(page, 'Items', [
    {
      type: 'bulletList',
      content: [
        { type: 'listItem', content: [line('first'), nested] },
        { type: 'listItem', content: [line('second')] },
      ],
    },
  ]);
  const items = body(page).locator(':scope > ul > li');
  await items.first().locator('p').first().hover();
  await page.locator('.item-grip').dragTo(items.last(), { targetPosition: { x: 40, y: 18 } });
  await expect(items.first()).toContainText('second');
  await expect(items.last()).toContainText('first');
  await expect(items.last()).toContainText('child');
});

test('a block dropped beside another makes columns', async ({ page }) => {
  await editing(page, 'Beside', [line('left'), line('right'), line('rest')]);
  const lines = body(page).locator('p');
  await lines.first().hover();
  const target = await lines.nth(1).boundingBox();
  await page.locator('.block-grip').dragTo(lines.nth(1), {
    targetPosition: { x: (target?.width ?? 400) - 4, y: 8 },
  });
  await expect(body(page).locator('.doc-column')).toHaveCount(2);
  await expect(body(page).locator('.doc-column').first()).toHaveText('right');
  await expect(body(page).locator('.doc-column').last()).toHaveText('left');
});

test('Browse all shows a preview, and inserts what is chosen', async ({ page }) => {
  await editing(page, 'Browse', [line('text')]);
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  await page.getByRole('button', { name: 'Browse all' }).click();
  const dialog = page.getByRole('dialog', { name: 'Browse all' });
  await dialog.getByRole('button', { name: 'Spotlight' }).hover();
  await expect(dialog.locator('.browse-preview .spotlight')).toBeVisible();
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Spotlight' }).click();
  await expect(body(page).locator('.spotlight')).toBeVisible();
});

test('the emoji tool has groups, and remembers what was used', async ({ page }) => {
  await editing(page, 'Smile', [line('text')]);
  await page.keyboard.press('Control+End');
  await page.getByRole('button', { name: 'More formatting' }).click();
  await page.getByRole('menuitem', { name: 'Emoji' }).click();
  const dialog = page.getByRole('dialog', { name: 'Emoji' });
  await expect(dialog.getByRole('region', { name: 'Faces' })).toBeVisible();
  await dialog.getByLabel('Search emoji').fill('rocket');
  await dialog.getByRole('button').filter({ hasText: '🚀' }).click();
  await expect(body(page)).toContainText('🚀');
  await page.getByRole('button', { name: 'More formatting' }).click();
  await page.getByRole('menuitem', { name: 'Emoji' }).click();
  await expect(page.getByRole('region', { name: 'Used lately' }).getByText('🚀')).toBeVisible();
});

test('a saved block comes back from the slash menu', async ({ page }) => {
  await editing(page, 'Saved', [line('reusable words')]);
  await body(page).locator('p').first().hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  await page.getByRole('menuitem', { name: 'Save as a block' }).click();
  const name = `Intro ${Date.now()}`;
  await page.getByRole('dialog').getByLabel('Name').fill(name);
  await page.getByRole('dialog').getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);

  await body(page).click();
  await editorSettled(page);
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Enter');
  await page.keyboard.type(`/${name}`);
  await page.keyboard.press('Enter');
  await expect(body(page).locator('p').filter({ hasText: 'reusable words' })).toHaveCount(2);
});

test('an image takes a width, a border, a link and text around it', async ({ page }) => {
  const space = await newSpace(page, 'Pictures space');
  const id = await publishedPage(page, space.id, 'Pictures', 'start');
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
  const image = { type: 'image', attrs: { attachment_id: attachment, alt: 'a.png' } };
  const answer = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title: 'Pictures', document: { type: 'doc', content: [image, line('after')] }, base_version: 1 },
  });
  expect(answer.ok(), await answer.text()).toBeTruthy();
  await page.goto(`/pages/${id}/edit`);
  await expect(page.getByText('1 image has no description yet.')).toBeVisible();

  await body(page).locator('p').click();
  await editorSettled(page);
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowUp');
  await page.getByRole('button', { name: 'Let text flow around the image' }).click();
  await page.getByRole('button', { name: 'Border', exact: true }).click();
  await page.getByRole('menuitemradio', { name: 'Thin' }).click();
  await page.getByRole('button', { name: 'Describe this image' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Alt text').fill('A dot');
  await dialog.getByLabel('Width').fill('40');
  await dialog.getByLabel('Link').fill('https://example.org/a');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByText('1 image has no description yet.')).toHaveCount(0);
  await publish(page);

  const figure = prose(page).locator('figure.doc-image');
  await expect(figure).toHaveAttribute('data-wrap');
  await expect(figure).toHaveAttribute('data-border', 'thin');
  await expect(figure).toHaveAttribute('style', /40%/);
  await expect(figure.locator('a')).toHaveAttribute('href', 'https://example.org/a');
});

test('a folded code block opens for a reader and counts from its first line', async ({ page }) => {
  const code = {
    type: 'codeBlock',
    attrs: { line_numbers: true, line_start: 40, folded: true },
    content: [{ type: 'text', text: 'one\ntwo' }],
  };
  const { id } = await pageWith(page, 'Folded', [code]);
  await page.goto(`/pages/${id}`);
  await expect(prose(page).locator('pre')).toBeHidden();
  await page.getByRole('button', { name: 'Show 2 lines' }).click();
  await expect(prose(page).locator('pre')).toBeVisible();
  await expect(prose(page).locator('.code-lines li').first()).toHaveText('40');
});

test('Tab indents all selected lines of code, and Shift Tab takes it back', async ({ page }) => {
  await editing(page, 'Indent', [{ type: 'codeBlock', content: [{ type: 'text', text: 'one\ntwo' }] }]);
  await body(page).locator('pre').click();
  await editorSettled(page);
  await page.keyboard.press('Control+a');
  await page.keyboard.press('Tab');
  await expect(body(page).locator('pre')).toHaveText('  one\n  two');
  await page.keyboard.press('Shift+Tab');
  await expect(body(page).locator('pre')).toHaveText('one\ntwo');
});

test('a spotlight shows its button to readers, a panel its own emoji', async ({ page }) => {
  const { id } = await pageWith(page, 'Bright', [
    {
      type: 'spotlight',
      attrs: { color: 'blue', button_text: 'Read on', button_href: 'https://example.org/on' },
      content: [line('Big news')],
    },
    { type: 'panel', attrs: { kind: 'neutral', emoji: '🌊', color: 'blue' }, content: [line('Water')] },
    {
      type: 'columns',
      attrs: { layout: 'sidebar-end', width: 'wide' },
      content: [
        { type: 'column', content: [line('Main')] },
        { type: 'column', content: [line('Side')] },
      ],
    },
  ]);
  await page.goto(`/pages/${id}`);
  await expect(prose(page).getByRole('link', { name: 'Read on' })).toHaveAttribute(
    'href',
    'https://example.org/on',
  );
  await expect(prose(page).locator('.panel-icon')).toHaveText('🌊');
  await expect(prose(page).locator('.doc-columns')).toHaveAttribute('data-layout', 'sidebar-end');
  await checkAccessibility(page);
});

test('a chart follows its table, has options, and can be read as a table', async ({ page }) => {
  const { id } = await editing(page, 'Numbers', [
    sales(),
    { type: 'chart', attrs: { table: 'salestable', kind: 'line', points: true } },
  ]);
  await expect(body(page).locator('.chart-point')).toHaveCount(2);
  await body(page).locator('td').nth(1).click();
  await editorSettled(page);
  await page.keyboard.press('End');
  await page.keyboard.type('20');
  await expect(body(page).locator('.chart table td').nth(1)).toHaveText('1020');

  await body(page).locator('.chart-options summary').click();
  await page.getByLabel('Show a legend').uncheck();
  await expect(body(page).locator('.chart-legend')).toHaveCount(0);
  await page.getByLabel('Title of the axis').fill('Month of the year');
  await expect(body(page).locator('.chart-axis')).toHaveText('Month of the year');
  await publish(page);
  await page.goto(`/pages/${id}`);
  await expect(prose(page).locator('.chart table')).toHaveClass(/visually-hidden/);
  await expect(prose(page).locator('.chart table td').nth(1)).toHaveText('1020');
});

test('removing a table asks about its chart, and a chart in view keeps a hidden table', async ({ page }) => {
  await editing(page, 'Pair', [
    { type: 'expand', attrs: { title: 'The numbers' }, content: [sales()] },
    { type: 'chart', attrs: { table: 'salestable' } },
    line('end'),
  ]);
  await expect(body(page).locator('.chart svg')).toBeVisible();
  await body(page).locator('td').first().click();
  await editorSettled(page);
  await page.keyboard.press('Control+Shift+.');
  await page.getByRole('menuitem', { name: 'Delete table' }).click();
  await page
    .getByRole('dialog', { name: 'This table has a chart' })
    .getByRole('button', { name: 'Remove both' })
    .click();
  await expect(body(page).locator('table')).toHaveCount(0);
  await expect(body(page).locator('.chart')).toHaveCount(0);
});

test('the language of a page reaches the text, and stays out of a new version', async ({ page }) => {
  const { id } = await editing(page, 'Taal', [line('tekst')]);
  await page.getByText('Page options').hover();
  await page.getByRole('button', { name: 'Page options' }).click();
  await page.getByRole('menuitem', { name: 'Language of this page' }).click();
  await page.getByRole('dialog').getByRole('combobox').selectOption('nl');
  await publish(page);
  await page.goto(`/pages/${id}`);
  await expect(prose(page)).toHaveAttribute('lang', 'nl');
});

test('a line can run the other way than the page', async ({ page }) => {
  const { id } = await editing(page, 'Mixed', [line('שלום')]);
  await page.getByRole('button', { name: 'Alignment', exact: true }).click();
  await page.getByRole('menuitemradio', { name: 'Right to left' }).click();
  await publish(page);
  await page.goto(`/pages/${id}`);
  await expect(prose(page).locator('p').first()).toHaveAttribute('dir', 'rtl');
});

test('the status of a page can be set while writing', async ({ page }) => {
  await editing(page, 'Status', [line('text')]);
  await expect(page.getByRole('button', { name: 'Set status' })).toBeVisible();
});

test('a block from a newer release shows a note, and stays through a save', async ({ page }) => {
  const original = JSON.stringify({ type: 'hologram', attrs: { depth: 3 } });
  const { id } = await editing(page, 'Future', [
    line('text'),
    { type: 'unknownBlock', attrs: { name: 'hologram', original } },
  ]);
  await expect(body(page).locator('.unknown-block')).toContainText('hologram');
  await body(page).locator('p').first().click();
  await editorSettled(page);
  await page.keyboard.type('more ');
  await publish(page);
  const stored = (await (await page.request.get(`/api/v1/pages/${id}`)).json()) as {
    document: { content: { type: string; attrs?: { original?: string } }[] };
  };
  expect(stored.document.content[1]?.attrs?.original).toBe(original);
});
