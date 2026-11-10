// SPDX-License-Identifier: Apache-2.0
// Spec 008: everything wave A adds to the editor, as a writer and as a reader.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  editorSettled,
  expect,
  grant,
  login,
  newSpace,
  publishedPage,
  test,
  unique,
  VIEWER,
} from './helpers';

const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  'base64',
);

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

/** Opens a page with one line of text in the editor, with the cursor at the end. */
async function editing(page: Page, title: string, text = 'start') {
  const space = await newSpace(page, `${title} space`);
  const id = await publishedPage(page, space.id, title, text);
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.press('Control+End');
  return { id, space };
}

const body = (page: Page) => page.locator('.prose.editing');
const menu = (page: Page) => page.getByRole('listbox', { name: 'Insert' });

async function slash(page: Page, query: string) {
  await page.keyboard.press('Enter');
  await page.keyboard.type(`/${query}`);
  await expect(menu(page)).toBeVisible();
  await page.keyboard.press('Enter');
}

async function tableMenu(page: Page) {
  await page.getByRole('button', { name: 'Table options' }).click();
  return page.getByRole('menu', { name: 'Table options' });
}

async function publish(page: Page) {
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('link', { name: 'Edit', exact: true })).toBeVisible();
}

test('the slash menu filters, inserts with Enter and with a click, and Escape keeps the text', async ({
  page,
}) => {
  await editing(page, 'Slash');
  await page.keyboard.press('Enter');
  await page.keyboard.type('/');
  await expect(menu(page)).toBeVisible();
  await expect(body(page)).toHaveAttribute('aria-activedescendant', /.+/);

  await page.keyboard.type('warn');
  await expect(menu(page).getByRole('option')).toHaveCount(1);
  await page.keyboard.press('Enter');
  await expect(body(page).locator('.panel')).toBeVisible();
  await expect(body(page)).not.toContainText('/warn');

  await page.keyboard.type('in the panel');
  await page.keyboard.press('Enter');
  await page.keyboard.type('/xyz');
  await expect(menu(page)).toHaveCount(0);
  await expect(body(page)).toContainText('/xyz');

  await page.keyboard.press('Enter');
  await page.keyboard.type('/quo');
  await page.keyboard.press('Escape');
  await expect(menu(page)).toHaveCount(0);
  await expect(body(page)).toContainText('/quo');

  await page.keyboard.press('Enter');
  await page.keyboard.type('/deci');
  await menu(page)
    .getByRole('option', { name: /^Decision / })
    .click();
  await expect(body(page).locator('.decision')).toBeVisible();
});

test('the slash menu stays away from code, numbers and addresses', async ({ page }) => {
  await editing(page, 'No slash');
  await page.keyboard.type(' 1/2');
  await expect(menu(page)).toHaveCount(0);
  await page.keyboard.type(' https://example.com/a');
  await expect(menu(page)).toHaveCount(0);
  await page.keyboard.press('Enter');
  await page.keyboard.type('```');
  await page.keyboard.press('Enter');
  await page.keyboard.type('/usr/bin');
  await expect(menu(page)).toHaveCount(0);
});

test('the Insert tool offers the same list in a dialog', async ({ page }) => {
  await editing(page, 'Insert');
  await page.getByRole('button', { name: 'Insert', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Insert' });
  await dialog.getByLabel('What do you want to add?').fill('rule');
  await page.keyboard.press('Enter');
  await expect(dialog).toHaveCount(0);
  await expect(body(page).locator('hr')).toBeVisible();
});

test('every block survives publishing and reloading, and reads well', async ({ page }) => {
  await editing(page, 'Everything');
  await slash(page, 'note');
  await page.keyboard.type('a note');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Backspace');
  await slash(page, 'colla');
  await page.keyboard.type('hidden words');
  await page.getByLabel('Title of this section').fill('More about this');
  await page.locator('.prose.editing').click({ position: { x: 5, y: 5 } });
  await page.keyboard.press('Control+End');
  await slash(page, 'two col');
  await page.keyboard.type('left side');
  await page.keyboard.press('Control+End');
  await slash(page, 'decision');
  await page.keyboard.type('We go ahead');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.keyboard.type('Today is ');
  await page.keyboard.type('//');
  await page.keyboard.type(' and ');
  await page.keyboard.type('/status');
  await page.keyboard.press('Enter');
  await page.getByLabel('Words of the status').fill('Ready');
  await page.getByRole('button', { name: 'Green', exact: true }).click();
  await publish(page);
  await page.reload();

  const doc = page.locator('.prose');
  await expect(doc.locator('.panel')).toContainText('a note');
  await expect(doc.locator('.panel')).toHaveAttribute('role', 'note');
  await expect(doc.locator('details summary')).toHaveText('More about this');
  await expect(doc.locator('.doc-columns .doc-column')).toHaveCount(2);
  await expect(doc.locator('.decision')).toContainText('We go ahead');
  await expect(doc.locator('.status-mark')).toHaveText('Ready');
  await expect(doc.locator('.status-mark')).toHaveClass(/mark-green/);
  await expect(doc.locator('time.date-mark')).toHaveAttribute('datetime', /^\d{4}-\d{2}-\d{2}$/);
  await checkAccessibility(page);
});

test('typing shortcuts make a panel, a decision, a date, an emoji and a link', async ({ page }) => {
  await editing(page, 'Shortcuts');
  await page.keyboard.press('Enter');
  await page.keyboard.type('!! careful');
  await expect(body(page).locator('.panel')).toContainText('careful');
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Backspace');
  await page.keyboard.type('<> decided');
  await expect(body(page).locator('.decision')).toContainText('decided');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Enter');
  await page.keyboard.type('see [the site](https://example.com/) today :rock');
  await expect(body(page).locator('a')).toHaveAttribute('href', 'https://example.com/');
  await expect(page.getByRole('listbox', { name: 'Emoji' })).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(body(page)).toContainText('🚀');
});

test('a colon in a sentence leaves the text alone', async ({ page }) => {
  await editing(page, 'Colon');
  await page.keyboard.type(' note: this');
  await expect(page.getByRole('listbox', { name: 'Emoji' })).toHaveCount(0);
  await page.keyboard.type(' 10:30');
  await expect(page.getByRole('listbox', { name: 'Emoji' })).toHaveCount(0);
});

test('colours, highlight, alignment and heading levels survive publishing', async ({ page }) => {
  await editing(page, 'Colours', 'colourful words');
  await page.keyboard.press('Control+A');
  await page.getByRole('button', { name: 'Text colour' }).click();
  await page.getByRole('menuitemradio', { name: 'Purple' }).click();
  await page.getByRole('button', { name: 'Highlight', exact: true }).click();
  await page.getByRole('menuitemradio', { name: 'Yellow' }).click();
  await page.getByRole('button', { name: 'Alignment' }).click();
  await page.getByRole('menuitemradio', { name: 'Align to the centre' }).click();
  await page.getByLabel('Text style').selectOption({ label: 'Heading 5' });
  await publish(page);
  await page.reload();

  const heading = page.locator('.prose h5');
  await expect(heading).toHaveAttribute('data-align', 'center');
  await expect(heading.locator('mark.mark-yellow')).toBeVisible();
  await expect(heading.locator('.text-purple')).toHaveText('colourful words');
});

test('indent steps in and out, and clear formatting removes marks', async ({ page }) => {
  await editing(page, 'Indent', 'indented');
  await page.getByRole('button', { name: 'Indent', exact: true }).click();
  await page.getByRole('button', { name: 'Indent', exact: true }).click();
  await expect(body(page).locator('p')).toHaveAttribute('data-indent', '2');
  await page.getByRole('button', { name: 'Outdent' }).click();
  await expect(body(page).locator('p')).toHaveAttribute('data-indent', '1');

  await page.keyboard.press('Control+A');
  await page.keyboard.press('Control+b');
  await expect(body(page).locator('strong')).toBeVisible();
  await page.getByRole('button', { name: 'More formatting' }).click();
  await page.getByRole('menuitem', { name: 'Clear formatting' }).click();
  await expect(body(page).locator('strong')).toHaveCount(0);
});

test('a block moves with the menu and with Alt and the arrows, and Undo reverses it', async ({ page }) => {
  await editing(page, 'Moves', 'first');
  await page.keyboard.press('Enter');
  await page.keyboard.type('second');
  const lines = body(page).locator('p');
  await expect(lines).toHaveText(['first', 'second']);

  await page.keyboard.press('Alt+ArrowUp');
  await expect(lines).toHaveText(['second', 'first']);
  await page.keyboard.press('Control+z');
  await expect(lines).toHaveText(['first', 'second']);

  await lines.first().hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  await page.getByRole('menuitem', { name: 'Move down' }).click();
  await expect(lines).toHaveText(['second', 'first']);

  await editorSettled(page);
  await page.keyboard.press('Control+d');
  await expect(lines).toHaveCount(3);
});

test('a block can be dragged by its grip', async ({ page }) => {
  await editing(page, 'Drag', 'first');
  await page.keyboard.press('Enter');
  await page.keyboard.type('second');
  await page.keyboard.press('Enter');
  await page.keyboard.type('third');
  const lines = body(page).locator('p');
  await lines.first().hover();
  await page.locator('.block-grip').dragTo(lines.last(), { targetPosition: { x: 10, y: 10 } });
  await expect(lines).toHaveText(['second', 'first', 'third']);
});

test('a column gets wider by dragging its border, and Equal columns undoes that', async ({ page }) => {
  await editing(page, 'Widths');
  await page.keyboard.press('Enter');
  await page.getByRole('button', { name: 'Table', exact: true }).click();
  await page.getByRole('dialog', { name: 'New table' }).getByRole('button', { name: 'Insert table' }).click();
  const first = body(page).locator('th').first();
  const box = await first.boundingBox();
  if (!box) throw new Error('the table has no size');
  await page.mouse.move(box.x + box.width - 1, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width + 80, box.y + box.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect(first).toHaveAttribute('colwidth', /\d+/);

  const menu = await tableMenu(page);
  await menu.getByRole('menuitem', { name: 'Equal columns' }).click();
  await expect(first).not.toHaveAttribute('colwidth', /\d+/);
});

test('Tab leaves the text, so a keyboard user is never trapped', async ({ page }) => {
  await editing(page, 'Tab away');
  await page.keyboard.press('Tab');
  await expect(body(page)).not.toBeFocused();
});

test('a list nests with Tab and comes back with Shift Tab', async ({ page }) => {
  await editing(page, 'Nesting');
  await page.keyboard.press('Enter');
  await page.keyboard.type('- one');
  await page.keyboard.press('Enter');
  await page.keyboard.press('Tab');
  await page.keyboard.type('two');
  await expect(body(page).locator('ul ul li')).toHaveText('two');
  await page.keyboard.press('Shift+Tab');
  await expect(body(page).locator('ul ul li')).toHaveCount(0);
});

test('the handle never covers the text', async ({ page }) => {
  await editing(page, 'Gutter', 'text');
  const line = body(page).locator('p');
  await line.hover();
  const handle = await page.locator('.block-handle').boundingBox();
  const text = await line.boundingBox();
  expect((handle?.x ?? 0) + (handle?.width ?? 0)).toBeLessThanOrEqual(text?.x ?? 0);
});

test('pasted formatted text arrives as blocks, under the strict policy', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await editing(page, 'Paste');
  await page.evaluate(async () => {
    const html = new Blob(['<h2>Pasted title</h2><ul><li>one</li></ul>'], { type: 'text/html' });
    await navigator.clipboard.write([new ClipboardItem({ 'text/html': html })]);
  });
  await page.keyboard.press('Enter');
  await page.keyboard.press('Control+v');
  await expect(body(page).locator('h2')).toHaveText('Pasted title');
  await expect(body(page).locator('li')).toHaveText('one');
});

test('the block menu turns a line into another block', async ({ page }) => {
  await editing(page, 'Turn', 'a line');
  await body(page).locator('p').hover();
  await page.getByRole('button', { name: 'Block actions' }).click();
  await page.getByRole('menuitem', { name: 'Heading 2' }).click();
  await expect(body(page).locator('h2')).toHaveText('a line');
});

test('find marks all matches, Replace all replaces them, and one Undo restores them', async ({ page }) => {
  await editing(page, 'Find', 'cat and cat and Cat');
  await page.keyboard.press('Control+f');
  const find = page.getByRole('search', { name: 'Find' });
  await find.getByRole('textbox', { name: 'Find' }).fill('cat');
  await expect(find.getByRole('status')).toHaveText('1 of 3');
  await expect(body(page).locator('.find-match')).toHaveCount(3);
  await page.keyboard.press('Enter');
  await expect(find.getByRole('status')).toHaveText('2 of 3');

  await find.getByLabel('Match case').check();
  await expect(find.getByRole('status')).toHaveText('1 of 2');

  await find.getByLabel('Replace with').fill('dog');
  await find.getByRole('button', { name: 'Replace all' }).click();
  await expect(body(page)).toHaveText('dog and dog and Cat');

  await page.getByRole('button', { name: 'Undo' }).click();
  await expect(body(page)).toHaveText('cat and cat and Cat');
  await page.keyboard.press('Escape');
});

test('Escape closes find and removes the marks', async ({ page }) => {
  await editing(page, 'Find close', 'word');
  await page.getByRole('button', { name: 'Find', exact: true }).click();
  await page.getByRole('textbox', { name: 'Find' }).fill('word');
  await expect(body(page).locator('.find-match')).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('search', { name: 'Find' })).toHaveCount(0);
  await expect(body(page).locator('.find-match')).toHaveCount(0);
});

test('a link to a page keeps working after a rename', async ({ page }) => {
  const { space } = await editing(page, 'Linking');
  const target = await publishedPage(page, space.id, 'Zebrapage target', 'x');
  await page.keyboard.press(' ');
  await page.keyboard.press('Control+k');
  const dialog = page.getByRole('dialog', { name: 'Link' });
  await dialog.getByLabel('Find a page').fill('Zebrapage');
  await dialog.getByRole('button', { name: 'Zebrapage target' }).click();
  await expect(dialog.getByLabel('Link address')).toHaveValue(`/pages/${target}`);
  await dialog.getByLabel('Text to show').fill('Zebrapage target');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await publish(page);

  const renamed = await page.request.patch(`/api/v1/pages/${target}`, { data: { title: 'A new name' } });
  expect(renamed.ok(), await renamed.text()).toBeTruthy();
  const href = await page.locator('.prose a').getAttribute('href');
  expect(href).toBe(`/pages/${target}`);
  await page.goto(href ?? '');
  await expect(page.getByRole('heading', { name: 'A new name', level: 1 })).toBeVisible();
});

test('the link dialog refuses an address we do not allow, before publishing', async ({ page }) => {
  await editing(page, 'Bad link');
  await page.keyboard.press('Control+k');
  const dialog = page.getByRole('dialog', { name: 'Link' });
  await dialog.getByLabel('Link address').fill('javascript:alert(1)');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(dialog.getByRole('alert')).toContainText('Use an address');
  await expect(body(page).locator('a')).toHaveCount(0);
});

test('a reader opens a section with the keyboard and copies a code block', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const space = await newSpace(page, 'Reader space');
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, parent_id: null, title: 'Reader' },
  });
  const { id } = (await created.json()) as { id: string };
  const paragraph = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Reader',
      base_version: 0,
      document: {
        type: 'doc',
        content: [
          { type: 'expand', attrs: { title: 'Open me' }, content: [paragraph('hidden words')] },
          {
            type: 'codeBlock',
            attrs: { language: 'js', title: 'hello.js', line_numbers: true },
            content: [{ type: 'text', text: 'const a = 1;\nconst b = 2;' }],
          },
        ],
      },
    },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  await page.goto(`/pages/${id}`);

  await expect(page.getByText('hidden words')).toBeHidden();
  await page.locator('summary').focus();
  await page.keyboard.press('Enter');
  await expect(page.getByText('hidden words')).toBeVisible();

  await expect(page.locator('.code-title')).toHaveText('hello.js');
  await expect(page.locator('.code-lines li')).toHaveCount(2);
  await page.getByRole('button', { name: 'Copy', exact: true }).click();
  await expect(page.locator('.code-message')).toHaveText('Copied');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('const a = 1;\nconst b = 2;');
  await checkAccessibility(page);
});

test('table tools: header, cell colour and sort survive publishing', async ({ page }) => {
  await editing(page, 'Tables');
  await page.keyboard.press('Enter');
  await page.getByRole('button', { name: 'Table', exact: true }).click();
  await page.getByRole('dialog', { name: 'New table' }).getByRole('button', { name: 'Insert table' }).click();
  await editorSettled(page);
  await page.keyboard.type('Name');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await page.keyboard.type('item 10');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await page.keyboard.press('Tab');
  await page.keyboard.type('item 2');
  const menu = await tableMenu(page);
  await menu.getByRole('menuitem', { name: 'Sort ascending' }).click();
  const cells = body(page).locator('tr td:first-child');
  await expect(cells).toHaveText(['item 2', 'item 10']);
  await expect(body(page).locator('th').first()).toHaveText('Name');
  await page.keyboard.press('Escape');

  await body(page).locator('tr td').first().click();
  const again = await tableMenu(page);
  await again.getByRole('menuitemradio', { name: 'Cell: Green' }).click();
  await again.getByRole('menuitemcheckbox', { name: 'Header column' }).click();
  await page.keyboard.press('Escape');
  await publish(page);
  await page.reload();
  await expect(page.locator('.prose [data-background="green"]')).toHaveCount(1);
  await expect(page.locator('.prose tbody th, .prose tr:nth-child(2) th')).not.toHaveCount(0);
});

test('merged cells survive publishing, and take the sort tools away', async ({ page }) => {
  await editing(page, 'Merge');
  await page.keyboard.press('Enter');
  await page.getByRole('button', { name: 'Table', exact: true }).click();
  await page.getByRole('dialog', { name: 'New table' }).getByRole('button', { name: 'Insert table' }).click();
  await editorSettled(page);
  await page.keyboard.press('Shift+ArrowRight');
  await page.keyboard.press('Shift+ArrowRight');
  const menu = await tableMenu(page);
  await menu.getByRole('menuitem', { name: 'Merge or split cells' }).click();
  await expect(menu.getByRole('menuitem', { name: 'Sort ascending' })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await publish(page);
  await page.reload();
  await expect(page.locator('.prose [colspan]')).toHaveCount(1);
});

test('changing the number of columns loses nothing', async ({ page }) => {
  await editing(page, 'Columns');
  await slash(page, 'three col');
  await page.keyboard.type('one');
  await body(page).locator('.doc-column').nth(1).click();
  await page.keyboard.type('two');
  await body(page).locator('.doc-column').nth(2).click();
  await page.keyboard.type('three');

  await page.getByLabel('Number of columns').selectOption({ label: '2 columns' });
  await expect(body(page).locator('.doc-column')).toHaveCount(2);
  await expect(body(page).locator('.doc-column').nth(1)).toContainText('two');
  await expect(body(page).locator('.doc-column').nth(1)).toContainText('three');

  await body(page).locator('.doc-column').first().click();
  await page.getByRole('button', { name: 'Remove the columns, keep the text' }).click();
  await expect(body(page).locator('.doc-column')).toHaveCount(0);
  await expect(body(page)).toContainText('three');
});

test('a panel changes kind, and goes away while its text stays', async ({ page }) => {
  await editing(page, 'Panels');
  await slash(page, 'info');
  await page.keyboard.type('careful now');
  await expect(body(page).locator('.panel')).toHaveClass(/mark-blue/);
  await page.getByRole('button', { name: 'Error panel' }).click();
  await expect(body(page).locator('.panel')).toHaveClass(/mark-red/);
  await page.getByRole('button', { name: 'Remove the panel, keep the text' }).click();
  await expect(body(page).locator('.panel')).toHaveCount(0);
  await expect(body(page)).toContainText('careful now');
});

test('an image gets a size, an alignment, alt text and a caption', async ({ page }) => {
  await editing(page, 'Image options');
  await page.locator('input[type=file][aria-label="Add image or file"]').setInputFiles({
    name: 'dot.png',
    mimeType: 'image/png',
    buffer: PNG,
  });
  await body(page).locator('img').click();
  await page.getByRole('button', { name: 'Image size' }).click();
  await page.getByRole('menuitemradio', { name: 'Medium' }).click();
  await body(page).locator('img').click();
  await page.getByRole('button', { name: 'Align to the centre' }).click();
  await body(page).locator('img').click();
  await page.getByRole('button', { name: 'Describe this image' }).click();
  const dialog = page.getByRole('dialog', { name: 'Describe this image' });
  await dialog.getByLabel('Alt text').fill('A tiny dot');
  await dialog.getByLabel('Caption').fill('The smallest picture');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await publish(page);
  await page.reload();
  const figure = page.locator('.prose figure');
  await expect(figure).toHaveAttribute('data-size', 'medium');
  await expect(figure).toHaveAttribute('data-align', 'center');
  await expect(figure.locator('img')).toHaveAttribute('alt', 'A tiny dot');
  await expect(figure.locator('figcaption')).toHaveText('The smallest picture');
});

test('the code tools set the language, numbers and a title', async ({ page }) => {
  await editing(page, 'Code tools');
  await slash(page, 'code block');
  await page.keyboard.type('const a = 1;');
  await page.getByRole('button', { name: /^Language:/ }).click();
  const dialog = page.getByRole('dialog', { name: 'Language' });
  await dialog.getByLabel('Find a language').fill('javascript');
  await dialog.getByRole('button', { name: 'javascript', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: 'Line numbers' }).click();
  await page.getByRole('button', { name: 'Wrap long lines' }).click();
  await expect(body(page).locator('.code-frame')).toHaveClass(/wrap/);
  await page.getByLabel('Title of this code').fill('main.js');
  await publish(page);
  await page.reload();
  await expect(page.locator('.code-title')).toHaveText('main.js');
  await expect(page.locator('.prose .hljs-keyword')).toHaveText('const');
  await expect(page.locator('.code-lines li')).toHaveCount(1);
});

test('Tab in code indents, and Escape leads out to the tools', async ({ page }) => {
  await editing(page, 'Code keys');
  await slash(page, 'code block');
  await page.keyboard.press('Tab');
  await page.keyboard.type('x');
  await expect(body(page).locator('code')).toHaveText('  x');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('toolbar', { name: 'Formatting' }).locator(':focus')).toBeVisible();
});

test('the tools are one Tab stop with arrow keys, and Escape returns to the text', async ({ page }) => {
  await editing(page, 'Roving');
  const toolbar = page.getByRole('toolbar', { name: 'Formatting' });
  await page.keyboard.press('Alt+F10');
  await expect(toolbar.getByLabel('Text style')).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(toolbar.getByRole('button', { name: 'Bold' })).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(toolbar.getByRole('button', { name: 'Italic' })).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(toolbar.getByRole('button', { name: 'Bold' })).toBeFocused();
  await page.keyboard.press('End');
  await expect(toolbar.getByRole('button', { name: 'Full width' })).toBeFocused();
  await page.keyboard.press('Home');
  await expect(toolbar.getByLabel('Text style')).toBeFocused();
  expect(await toolbar.locator('[tabindex="0"]').count()).toBe(1);

  await page.keyboard.press('Escape');
  await expect(body(page)).toBeFocused();
});

test('a menu follows Home, End and letters, and Escape returns to its button', async ({ page }) => {
  await editing(page, 'Menu keys');
  await page.getByRole('button', { name: 'Text colour' }).click();
  await expect(page.getByRole('menuitemradio', { name: 'Default' })).toBeFocused();
  await page.keyboard.press('End');
  await expect(page.getByRole('menuitemradio', { name: 'Brown' })).toBeFocused();
  await page.keyboard.press('Home');
  await expect(page.getByRole('menuitemradio', { name: 'Default' })).toBeFocused();
  await page.keyboard.press('p');
  await expect(page.getByRole('menuitemradio', { name: 'Purple' })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Text colour' })).toBeFocused();
});

test('the editor with every block passes the accessibility check', async ({ page }) => {
  await editing(page, 'Editor axe');
  await slash(page, 'warning');
  await page.keyboard.type('x');
  await page.keyboard.press('Control+End');
  await checkAccessibility(page);
});

// A small real PDF: the offsets in its table must be right, or a reader refuses it.
function onePagePdf(): Buffer {
  const objects = [
    '<</Type/Catalog/Pages 2 0 R>>',
    '<</Type/Pages/Kids[3 0 R]/Count 1>>',
    '<</Type/Page/Parent 2 0 R/MediaBox[0 0 300 200]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>',
    '<</Length 44>>stream\nBT /F1 18 Tf 20 100 Td (Hello Diffluence) Tj ET\nendstream',
    '<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>',
  ];
  let body = '%PDF-1.4\n';
  const offsets = objects.map((object, i) => {
    const at = body.length;
    body += `${i + 1} 0 obj\n${object}\nendobj\n`;
    return at;
  });
  const table = body.length;
  body += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n`;
  for (const at of offsets) body += `${String(at).padStart(10, '0')} 00000 n \n`;
  body += `trailer<</Size ${objects.length + 1}/Root 1 0 R>>\nstartxref\n${table}\n%%EOF\n`;
  return Buffer.from(body, 'latin1');
}

test('on a phone the viewer keeps its way back in one piece', async ({ page }) => {
  await editing(page, 'A PDF on a phone');
  await page.locator('input[type=file][aria-label="Add image or file"]').setInputFiles({
    name: 'a long name for a report that goes on for a while.pdf',
    mimeType: 'application/pdf',
    buffer: onePagePdf(),
  });
  await expect(body(page).locator('.file-tile')).toContainText('a long name');
  await publish(page);
  await page.setViewportSize({ width: 390, height: 800 });
  await page.reload();
  await page.getByRole('button', { name: 'Open in PdfCraft' }).click();
  const close = page.getByRole('dialog').getByRole('button', { name: 'Close' });
  await expect(close).toBeVisible();
  const lineHeight = await close.evaluate((node) => parseFloat(getComputedStyle(node).lineHeight));
  const box = await close.boundingBox();
  expect(box?.height ?? 0).toBeLessThan(lineHeight * 2);
  const sideways = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
  expect(sideways).toBe(false);
});

test('a PDF opens in a sandboxed viewer that cannot reach the session', async ({ page }) => {
  await editing(page, 'A PDF');
  await page.locator('input[type=file][aria-label="Add image or file"]').setInputFiles({
    name: 'hello.pdf',
    mimeType: 'application/pdf',
    buffer: onePagePdf(),
  });
  await expect(body(page).locator('.file-tile')).toContainText('hello.pdf');
  await publish(page);
  await page.getByRole('button', { name: 'Open in PdfCraft' }).click();
  const dialog = page.getByRole('dialog', { name: 'hello.pdf' });
  await expect(dialog.getByRole('status')).toHaveCount(0, { timeout: 60_000 });
  await expect(dialog.getByRole('link', { name: 'Download the file' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Close' })).toContainText('Close');
  const window = page.viewportSize();
  const box = await dialog.locator('iframe.craft-frame').boundingBox();
  expect(box?.width ?? 0).toBeGreaterThan((window?.width ?? 0) * 0.95);
  expect(box?.height ?? 0).toBeGreaterThan((window?.height ?? 0) * 0.8);
  const viewer = page.frames().find((frame) => frame.url().includes('/vendor/pdfcraft/'));
  if (!viewer) throw new Error('the viewer has no frame');
  const seen = await viewer.evaluate(async () => {
    const answer = await fetch('/api/v1/me', { credentials: 'include' }).then(
      (r) => r.status,
      () => 0,
    );
    let cookie = 'readable';
    try {
      cookie = document.cookie;
    } catch {
      cookie = 'refused';
    }
    return { origin: self.origin, answer, cookie };
  });
  expect(seen).toEqual({ origin: 'null', answer: 0, cookie: 'refused' });
  await page.frameLocator('iframe.craft-frame').locator('canvas').press('Escape');
  await expect(dialog).toHaveCount(0);
});

/** Opens the PDF of the page in its viewer and returns the frame, once the file is in. */
async function openViewer(page: Page, name: string) {
  await page.getByRole('button', { name: 'Open in PdfCraft' }).click();
  const dialog = page.getByRole('dialog', { name });
  await expect(dialog.getByRole('status')).toHaveCount(0, { timeout: 60_000 });
  const viewer = page.frames().find((frame) => frame.url().includes('/vendor/pdfcraft/'));
  if (!viewer) throw new Error('the viewer has no frame');
  return { dialog, viewer };
}

/** Does inside the viewer what PdfCraft does when it saves: a download of a blob it made. */
async function saveInViewer(
  page: Page,
  viewer: Awaited<ReturnType<typeof openViewer>>['viewer'],
  text: string,
) {
  const download = page.waitForEvent('download');
  await viewer.evaluate((content) => {
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob([content], { type: 'application/pdf' }));
    link.download = 'saved.pdf';
    link.click();
  }, text);
  await download;
}

test('a PDF changed in the viewer is kept as a new version, and only what the frame sends counts', async ({
  page,
  browser,
}) => {
  const { id, space } = await editing(page, 'A changed PDF');
  await page.locator('input[type=file][aria-label="Add image or file"]').setInputFiles({
    name: 'changed.pdf',
    mimeType: 'application/pdf',
    buffer: onePagePdf(),
  });
  await expect(body(page).locator('.file-tile')).toContainText('changed.pdf');
  await publish(page);
  const listed = await page.request.get(`/api/v1/pages/${id}/attachments`);
  const { attachments } = (await listed.json()) as { attachments: { id: string }[] };
  expect(attachments).toHaveLength(1);
  const fileId = attachments[0]?.id ?? '';
  const pdf = onePagePdf().toString('latin1');

  const { dialog, viewer } = await openViewer(page, 'changed.pdf');
  // The page itself cannot pose as the viewer: only the frame's window counts.
  await page.evaluate((content) => {
    const bytes = new TextEncoder().encode(content).buffer;
    // The test forges a message from the page itself, so the target is deliberately open.
    // nosemgrep: javascript.browser.security.wildcard-postmessage-configuration.wildcard-postmessage-configuration
    window.postMessage({ craft: 'saved', bytes }, '*', [bytes]);
  }, pdf);
  await saveInViewer(page, viewer, 'not a PDF at all');
  await expect(dialog.getByText('The viewer gave back something that is not a PDF')).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Keep it as a new version' })).toHaveCount(0);

  await saveInViewer(page, viewer, pdf);
  await dialog.getByRole('button', { name: 'Keep it as a new version' }).click();
  await expect(dialog.getByRole('status')).toHaveText('Stored as a new version of this file.');
  const versions = await page.request.get(`/api/v1/attachments/${fileId}/versions`);
  expect(((await versions.json()) as { versions: unknown[] }).versions).toHaveLength(2);

  const email = `${unique('reader')}@example.com`;
  const reader = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: reader }, VIEWER);
  const context = await browser.newContext();
  const other = await context.newPage();
  await login(other, email);
  await other.goto(`/pages/${id}`);
  const seen = await openViewer(other, 'changed.pdf');
  await saveInViewer(other, seen.viewer, pdf);
  await expect(seen.dialog.getByRole('button', { name: 'Keep it as a new version' })).toHaveCount(0);
  await context.close();
});
