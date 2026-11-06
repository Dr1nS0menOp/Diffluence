// SPDX-License-Identifier: Apache-2.0
// Spec 009: reading a page and getting around, by mouse and by keyboard.
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

interface PageJson {
  header_position: number;
}

const heading = (level: number, text: string) => ({
  type: 'heading',
  attrs: { level },
  content: [{ type: 'text', text }],
});
const words = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });

async function publish(page: Page, id: string, title: string, version: number, content: unknown[]) {
  const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title, document: { type: 'doc', content }, base_version: version },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

const longText = Array.from({ length: 40 }, () => 'A sentence that fills the page with enough words.').join(
  ' ',
);

async function longPage(page: Page, title: string) {
  const space = await newSpace(page, `${title} space`);
  const id = await publishedPage(page, space.id, title, 'x');
  await publish(page, id, title, 1, [
    heading(2, 'First part'),
    words(longText),
    heading(2, 'Second part'),
    words(longText),
    heading(3, 'A detail'),
    words(longText),
    heading(2, 'Third part'),
    words(longText),
    words(longText),
    words(longText),
  ]);
  return { id, space };
}

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('a chosen theme beats the system, and is right after a reload', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await page.goto('/me/preferences');
  await page.getByLabel('Theme').selectOption('dark');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  const dark = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  expect(dark).toBe('rgb(17, 22, 31)');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await page.getByLabel('Theme').selectOption('system');
  await expect(page.locator('html')).not.toHaveAttribute('data-theme', /./);
  await checkAccessibility(page);
});

test('the top bar menus show where a person has been', async ({ page }) => {
  const space = await newSpace(page, 'Menu space');
  const id = await publishedPage(page, space.id, 'Menu page', 'text');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Add to favourites' }).click();

  await page.getByRole('button', { name: 'Recent', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: /Menu page/ })).toBeVisible();
  await page.keyboard.press('Escape');

  await page.getByRole('button', { name: 'Favourites', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: /Menu page/ })).toBeVisible();
  await page.getByRole('menuitem', { name: 'All favourites' }).click();
  await expect(page.getByRole('heading', { name: 'Favourites', level: 1 })).toBeVisible();

  await page.getByRole('button', { name: 'Spaces', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: 'Menu space' })).toBeVisible();
  await page.getByRole('menuitem', { name: 'All spaces' }).click();
  await expect(page).toHaveURL(/\/spaces$/);
});

test('quick search works from the keyboard alone', async ({ page }) => {
  const space = await newSpace(page, 'Quick space');
  const word = unique('quickword');
  const id = await publishedPage(page, space.id, `${word} page`, 'text');
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();

  await page.keyboard.press('/');
  const box = page.getByRole('combobox', { name: 'Search' });
  await expect(box).toBeFocused();
  await page.keyboard.type(word);
  const option = page.getByRole('option', { name: new RegExp(`${word} page`) });
  await expect(option).toBeVisible();
  await expect(page.getByRole('option', { name: `All results for "${word}"` })).toBeVisible();

  await page.keyboard.press('ArrowDown');
  await expect(box).toHaveAttribute('aria-activedescendant', /.+/);
  await expect(option).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(new RegExp(`/pages/${id}$`));

  // Enter without a marked row goes to the full results, and the search is remembered.
  await page.keyboard.press('/');
  await page.keyboard.type(word);
  await expect(option).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/search\?/);

  await box.click();
  await box.fill('');
  await expect(page.getByRole('option', { name: word, exact: true })).toBeVisible();
  await page.getByRole('option', { name: 'Clear recent searches' }).click();
  await expect(page.getByRole('option', { name: word, exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(box).toHaveAttribute('aria-expanded', 'false');
});

test('inside a space a chip limits quick search, and Backspace removes it', async ({ page }) => {
  const word = unique('chipword');
  const here = await newSpace(page, 'Chip space');
  const other = await newSpace(page, 'Other chip space');
  const id = await publishedPage(page, here.id, 'Here', 'text');
  await publishedPage(page, other.id, `${word} elsewhere`, 'text');
  await page.goto(`/pages/${id}`);

  const box = page.getByRole('combobox', { name: 'Search' });
  await box.click();
  await expect(page.locator('.search-chip')).toContainText('Chip space');
  await box.fill(word);
  await expect(page.getByRole('option', { name: `All results for "${word}"` })).toBeVisible();
  await expect(page.getByRole('option', { name: new RegExp(`${word} elsewhere`) })).toHaveCount(0);

  await box.fill('');
  await page.keyboard.press('Backspace');
  await expect(page.locator('.search-chip')).toHaveCount(0);
  await box.fill(word);
  await expect(page.getByRole('option', { name: new RegExp(`${word} elsewhere`) })).toBeVisible();
});

test('the tree filters, keeps parents, and marks what matches', async ({ page }) => {
  const space = await newSpace(page, 'Filter space');
  const parent = await publishedPage(page, space.id, 'Handbook', 'x');
  const child = await publishedPage(page, space.id, 'Holiday plan', 'x', parent);
  await publishedPage(page, space.id, 'Unrelated', 'x');
  await page.goto(`/pages/${child}`);

  const tree = page.getByRole('navigation', { name: 'Pages' });
  await page.getByLabel('Filter pages').fill('holiday');
  await expect(tree.getByRole('link', { name: 'Holiday plan' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Handbook' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Unrelated' })).toHaveCount(0);
  await expect(tree.locator('mark')).toHaveText('Holiday');

  await page.getByLabel('Filter pages').fill('nothing like this');
  await expect(tree).toContainText('No page has that in its title.');
  await page.getByLabel('Filter pages').press('Escape');
  await expect(tree.getByRole('link', { name: 'Unrelated' })).toBeVisible();

  await page.getByLabel('Order of the pages').selectOption('alphabet');
  await expect(tree).toContainText('Pages are arranged in the tree view.');
  await page.getByLabel('Order of the pages').selectOption('tree');
});

test('the tree works from the keyboard and is one Tab stop', async ({ page }) => {
  const space = await newSpace(page, 'Keys space');
  const parent = await publishedPage(page, space.id, 'Alpha', 'x');
  await publishedPage(page, space.id, 'Beta', 'x', parent);
  const other = await publishedPage(page, space.id, 'Gamma', 'x');
  await page.goto(`/pages/${other}`);

  const tree = page.getByRole('navigation', { name: 'Pages' });
  const gamma = tree.getByRole('link', { name: 'Gamma' });
  await expect(gamma).toHaveAttribute('tabindex', '0');
  await expect(tree.getByRole('link', { name: 'Alpha' })).toHaveAttribute('tabindex', '-1');
  await expect(tree.getByRole('button', { name: 'Actions for Alpha' })).toHaveAttribute('tabindex', '-1');
  await expect(tree.getByRole('button', { name: 'Actions for Gamma' })).toHaveAttribute('tabindex', '0');

  await gamma.focus();
  await page.keyboard.press('Home');
  await expect(tree.locator('a').first()).toBeFocused();
  await page.keyboard.press('End');
  await expect(gamma).toBeFocused();
  await page.keyboard.press('a');
  await expect(tree.getByRole('link', { name: 'Alpha' })).toBeFocused();

  const beta = tree.getByRole('link', { name: 'Beta' });
  await page.keyboard.press('ArrowLeft');
  await expect(beta).toHaveCount(0);
  await page.keyboard.press('ArrowRight');
  await expect(beta).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await expect(beta).toBeFocused();
  await page.keyboard.press('ArrowLeft');
  await expect(tree.getByRole('link', { name: 'Alpha' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(new RegExp(`/pages/${parent}$`));
});

test('a row menu renames a page, and offers only what the person may do', async ({ page, browser }) => {
  const space = await newSpace(page, 'Row space');
  const id = await publishedPage(page, space.id, 'Before', 'x');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Actions for Before' }).click();
  await page.getByRole('menuitem', { name: 'Rename' }).click();
  await page.getByLabel('New title').fill('After');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(
    page.getByRole('navigation', { name: 'Pages' }).getByRole('link', { name: 'After' }),
  ).toBeVisible();

  const email = `${unique('viewer')}@example.com`;
  const viewerId = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: viewerId }, VIEWER);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const viewer = await context.newPage();
  await login(viewer, email);
  await viewer.goto(`/pages/${id}`);
  await viewer.getByRole('button', { name: 'Actions for After' }).click();
  await expect(viewer.getByRole('menuitem', { name: 'Copy link' })).toBeVisible();
  await expect(viewer.getByRole('menuitem', { name: /Add to favourites/ })).toBeVisible();
  await expect(viewer.getByRole('menuitem', { name: 'Rename' })).toHaveCount(0);
  await expect(viewer.getByRole('menuitem', { name: 'Delete' })).toHaveCount(0);
  await context.close();
});

test('the sidebar width follows the keyboard, and comes back after a reload', async ({ page }) => {
  const space = await newSpace(page, 'Width space');
  const id = await publishedPage(page, space.id, 'Wide', 'x');
  await page.goto(`/pages/${id}`);
  const edge = page.getByRole('separator', { name: 'Width of the sidebar' });
  const before = Number(await edge.getAttribute('aria-valuenow'));
  await edge.focus();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect(edge).toHaveAttribute('aria-valuenow', String(before + 2));
  await page.reload();
  await expect(edge).toHaveAttribute('aria-valuenow', String(before + 2));
  await edge.focus();
  await page.keyboard.press('ArrowLeft');
  await page.keyboard.press('ArrowLeft');
  await expect(edge).toHaveAttribute('aria-valuenow', String(before));
});

test('a space administrator adds and orders shortcuts, and outside addresses open safely', async ({
  page,
}) => {
  const space = await newSpace(page, 'Shortcut space');
  await page.goto(`/spaces/${space.key}/settings`);
  const shortcuts = page.locator('section', { has: page.getByRole('heading', { name: 'Shortcuts' }) });
  await shortcuts.getByLabel('Label', { exact: true }).fill('Runbook');
  await shortcuts.getByPlaceholder(/Address/).fill(`/pages/${space.home_page_id}`);
  await shortcuts.getByRole('button', { name: 'Add', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Move Runbook up' })).toBeVisible();
  await shortcuts.getByLabel('Label', { exact: true }).fill('Status');
  await shortcuts.getByPlaceholder(/Address/).fill('https://status.example.com');
  await shortcuts.getByRole('button', { name: 'Add', exact: true }).click();
  await page.getByRole('button', { name: 'Move Status up' }).click();

  const links = page.getByRole('navigation', { name: 'Space links' });
  await expect(
    links
      .locator('.side-link')
      .filter({ hasText: /Status|Runbook/ })
      .first(),
  ).toContainText('Status');
  const outside = links.getByRole('link', { name: /Status/ });
  await expect(outside).toHaveAttribute('target', '_blank');
  await expect(outside).toHaveAttribute('rel', /noopener/);

  await shortcuts.getByLabel('Label', { exact: true }).fill('Bad');
  await shortcuts.getByPlaceholder(/Address/).fill('javascript:alert(1)');
  await shortcuts.getByRole('button', { name: 'Add', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('A shortcut goes to');
});

test('the overview and the directory count only what a person can see', async ({ page, browser }) => {
  const space = await newSpace(page, unique('Counted'));
  await publishedPage(page, space.id, 'Open', 'x');
  const hidden = await publishedPage(page, space.id, 'Hidden from the reader', 'x');
  const res = await page.request.put(`/api/v1/pages/${hidden}/restrictions`, {
    data: {
      kind: 'view',
      subjects: [{ type: 'user', id: (await (await page.request.get('/api/v1/me')).json()).user.id }],
    },
  });
  expect(res.status()).toBe(204);

  const email = `${unique('reader')}@example.com`;
  await grant(page, space.id, { type: 'user', id: await createUser(browser, email) }, VIEWER);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const reader = await context.newPage();
  await login(reader, email);

  await reader.goto(`/spaces/${space.key}/overview`);
  await expect(reader.getByRole('link', { name: 'Open', exact: true }).last()).toBeVisible();
  await expect(reader.locator('.details-facts')).toContainText('2');
  await expect(reader.getByText('Hidden from the reader')).toHaveCount(0);
  await checkAccessibility(reader);

  await reader.goto(`/spaces?q=${space.key}`);
  const row = reader.getByRole('row', { name: new RegExp(space.key) });
  await expect(row.getByRole('cell').nth(4)).toHaveText('2');
  await checkAccessibility(reader);
  await context.close();
});

test('an archived space only shows under Archived', async ({ page }) => {
  const name = unique('Archived');
  const space = await newSpace(page, name);
  const res = await page.request.post(`/api/v1/spaces/${space.id}/archive`, { data: {} });
  expect(res.status()).toBe(204);

  await page.goto('/spaces');
  await page.getByLabel('Filter by name or key').fill(name);
  await expect(page.getByText('No space fits this filter.')).toBeVisible();
  await page.getByLabel('Which spaces').selectOption('archived');
  await expect(page.getByRole('link', { name })).toBeVisible();
  await expect(page).toHaveURL(/kind=archived/);
  await page.goto('/');
  await expect(page.getByRole('link', { name })).toHaveCount(0);
});

test('recent has tabs, and a draft can be deleted from there', async ({ page }) => {
  const space = await newSpace(page, 'Recent space');
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, title: 'Unfinished work' },
  });
  expect(created.ok()).toBeTruthy();
  await page.goto('/recent');
  const tabs = page.getByRole('tablist', { name: 'Recent' });
  await tabs.getByRole('tab', { name: 'Visited' }).focus();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  const drafts = tabs.getByRole('tab', { name: 'Drafts' });
  await expect(drafts).toHaveAttribute('aria-selected', 'true');
  await expect(drafts).toBeFocused();

  const row = page.getByRole('listitem').filter({ hasText: 'Unfinished work' });
  await expect(row).toBeVisible();
  await checkAccessibility(page);
  await row.getByRole('button', { name: 'Delete' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
  await expect(row).toHaveCount(0);
});

test('favourites can be ordered without dragging, and removed together', async ({ page }) => {
  const space = await newSpace(page, 'Star space');
  const first = await publishedPage(page, space.id, unique('Star one'), 'x');
  const second = await publishedPage(page, space.id, unique('Star two'), 'x');
  for (const id of [first, second]) {
    expect((await page.request.put(`/api/v1/pages/${id}/favourite`, { data: {} })).status()).toBe(204);
  }
  await page.goto('/favourites');
  await page.getByLabel('Filter by title').fill('Star');
  await page.getByLabel('Filter by title').fill('');
  const rows = page.locator('.page-list').first().getByRole('listitem');
  const firstTitle = await rows.first().getByRole('link').innerText();
  await rows.first().getByRole('button', { name: /down$/ }).click();
  await expect(rows.nth(1).getByRole('link')).toHaveText(firstTitle);
  await page.reload();
  await expect(rows.nth(1).getByRole('link')).toHaveText(firstTitle);

  const before = await rows.count();
  await rows.first().getByRole('checkbox').check();
  await rows.nth(1).getByRole('checkbox').check();
  await page.getByRole('button', { name: 'Remove 2 favourites' }).click();
  await expect(rows).toHaveCount(before - 2);
});

test('the look of a page: emoji, width, title and header image', async ({ page }) => {
  const space = await newSpace(page, 'Look space');
  const id = await publishedPage(page, space.id, 'Looks', 'text');
  await page.goto(`/pages/${id}/edit`);
  const head = page.locator('.page-head');
  await head.hover();
  await head.getByRole('button', { name: 'Add emoji' }).click();
  const dialog = page.getByRole('dialog', { name: 'Emoji' });
  await dialog
    .getByRole('button')
    .filter({ hasText: /\p{Extended_Pictographic}/u })
    .first()
    .click();
  await expect(head.getByRole('button', { name: /^Change emoji/ })).toBeVisible();
  await page.getByRole('button', { name: 'Full width' }).click();
  await expect(page.getByRole('button', { name: 'Full width' })).toHaveAttribute('aria-pressed', 'true');
  await head.hover();
  await head.getByRole('button', { name: 'Page options' }).click();
  await page.getByRole('menuitemcheckbox', { name: 'Centre the title' }).click();
  await head
    .locator('input[type=file]')
    .setInputFiles({ name: 'head.png', mimeType: 'image/png', buffer: PNG });
  await expect(head.getByRole('button', { name: 'Remove image' })).toBeVisible();
  await checkAccessibility(page);
  await head.getByLabel('Position of the image').fill('20');
  await expect
    .poll(
      async () =>
        ((await (await page.request.get(`/api/v1/pages/${id}/summary`)).json()) as PageJson).header_position,
    )
    .toBe(20);

  await page.goto(`/pages/${id}`);
  await expect(page.locator('h1 .title-emoji')).toBeVisible();
  await expect(page.locator('h1')).toHaveClass(/title-center/);
  await expect(page.locator('.page-layout')).toHaveClass(/full/);
  await expect(page.locator('.page-header-image')).toBeVisible();
  await expect(page).toHaveTitle(/Looks · Diffluence/);
  await expect(page.getByRole('navigation', { name: 'Pages' }).locator('.page-emoji')).toBeVisible();
});

test('a heading link jumps to its heading and marks it', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const { id } = await longPage(page, 'Anchors');
  await page.goto(`/pages/${id}`);
  const third = page.locator('.prose h2').filter({ hasText: 'Third part' });
  await third.hover();
  await third.getByRole('button', { name: /Copy link to this heading/ }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Link to this heading copied.' })).toBeVisible();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).toContain(`/pages/${id}#third-part`);

  await page.goto(copied);
  await expect(page.locator('#third-part')).toHaveClass(/heading-target/);
  await expect(page.locator('#third-part')).toBeInViewport();
});

test('the contents follow the scroll, and can be turned off', async ({ page }) => {
  await page.setViewportSize({ width: 1500, height: 800 });
  const { id } = await longPage(page, 'Contents');
  await page.goto(`/pages/${id}`);
  const contents = page.getByRole('navigation', { name: 'On this page' });
  await expect(contents.getByRole('link')).toHaveCount(4);
  await contents.getByRole('link', { name: 'Third part' }).click();
  await expect(page.locator('#third-part')).toBeFocused();
  await expect(contents.getByRole('link', { name: 'Third part' })).toHaveAttribute(
    'aria-current',
    'location',
  );
  await checkAccessibility(page);

  await page.goto('/me/preferences');
  await page.getByLabel('Show the contents beside long pages').uncheck();
  await page.goto(`/pages/${id}`);
  await expect(page.locator('.prose h2').first()).toBeVisible();
  await expect(contents).toHaveCount(0);
  await page.goto('/me/preferences');
  await page.getByLabel('Show the contents beside long pages').check();
});

test('the byline gives a read time, and the details panel lists links and who worked on it', async ({
  page,
}) => {
  const { id, space } = await longPage(page, 'Facts');
  const linker = await publishedPage(page, space.id, 'Linker page', 'x');
  await publish(page, linker, 'Linker page', 1, [
    {
      type: 'paragraph',
      content: [{ type: 'text', text: 'see', marks: [{ type: 'link', attrs: { href: `/pages/${id}` } }] }],
    },
  ]);
  await page.goto(`/pages/${id}`);
  await expect(page.locator('.byline')).toContainText('min read');
  await page.locator('.byline-author').click();
  const panel = page.getByRole('complementary', { name: 'Details' });
  await expect(page.getByRole('heading', { name: 'Details' })).toBeFocused();
  await expect(panel.getByRole('link', { name: 'Linker page' })).toBeVisible();
  await expect(panel).toContainText('2 versions');
  await checkAccessibility(page);
  await page.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);

  await page.goto(`/pages/${linker}`);
  await page.locator('.prose a').click();
  await expect(page).toHaveURL(new RegExp(`/pages/${id}$`));
  await expect(page.locator('h1')).toContainText('Facts');
});

test('child pages show below a page, and images open large', async ({ page }) => {
  const space = await newSpace(page, 'Kids space');
  const id = await publishedPage(page, space.id, 'Parent', 'x');
  await publishedPage(page, space.id, 'Kid', 'x', id);
  const upload = async (name: string) => {
    const res = await page.request.post(`/api/v1/pages/${id}/attachments?filename=${name}`, {
      data: PNG,
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    expect(res.status()).toBe(201);
    return ((await res.json()) as { id: string }).id;
  };
  const image = (attachment: string, caption: string) => ({
    type: 'image',
    attrs: { attachment_id: attachment, alt: caption, caption },
  });
  await publish(page, id, 'Parent', 1, [
    image(await upload('a.png'), 'First image'),
    image(await upload('b.png'), 'Second image'),
  ]);

  await page.goto(`/pages/${id}`);
  await expect(
    page.getByRole('region', { name: 'Child pages' }).getByRole('link', { name: 'Kid' }),
  ).toBeVisible();

  await page.locator('.image-open').first().click();
  const viewer = page.getByRole('dialog', { name: 'First image' });
  await expect(viewer).toBeVisible();
  await expect(viewer.getByRole('status')).toHaveText('1 of 2');
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'Second image' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('reader mode and presentation work from the keyboard', async ({ page }) => {
  const { id } = await longPage(page, 'Slides');
  await page.goto(`/pages/${id}`);
  await expect(page.locator('h1')).toBeVisible();

  await page.keyboard.press('r');
  await expect(page.locator('header.top')).toBeHidden();
  await expect(page.getByRole('button', { name: 'Leave reader mode' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('header.top')).toBeVisible();

  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Present' }).click();
  const show = page.getByRole('dialog', { name: 'Slides' });
  await expect(show.getByRole('status')).toHaveText('1 of 3');
  await expect(show.locator('.prose')).toContainText('First part');
  await page.keyboard.press('ArrowRight');
  await expect(show.getByRole('status')).toHaveText('2 of 3');
  await expect(show.locator('.prose')).toContainText('A detail');
  await page.keyboard.press('End');
  await expect(show.getByRole('status')).toHaveText('3 of 3');
  await page.keyboard.press('Home');
  await expect(show.getByRole('status')).toHaveText('1 of 3');
  await show.getByRole('button', { name: 'Close' }).click();
  await expect(show).toHaveCount(0);
});

test('printing shows only the page, with every section open', async ({ page }) => {
  const space = await newSpace(page, 'Print space');
  const id = await publishedPage(page, space.id, 'Printed', 'x');
  await publish(page, id, 'Printed', 1, [
    { type: 'expand', attrs: { title: 'Closed section' }, content: [words('Inside the section')] },
  ]);
  await page.goto(`/pages/${id}`);
  // Hidden is also true before the page is there, and then nothing hears beforeprint.
  await expect(page.getByText('Closed section')).toBeVisible();
  await expect(page.getByText('Inside the section')).toBeHidden();

  await page.emulateMedia({ media: 'print' });
  await page.evaluate(() => window.dispatchEvent(new Event('beforeprint')));
  await expect(page.getByText('Inside the section')).toBeVisible();
  await expect(page.locator('header.top')).toBeHidden();
  await expect(page.locator('.sidebar')).toBeHidden();
  await expect(page.locator('.page-actions')).toBeHidden();

  await page.evaluate(() => window.dispatchEvent(new Event('afterprint')));
  await page.emulateMedia({ media: 'screen' });
  await expect(page.getByText('Inside the section')).toBeHidden();
});

test('a delete names what it touches, and Undo brings the page back', async ({ page }) => {
  const space = await newSpace(page, 'Delete space');
  const parent = await publishedPage(page, space.id, 'Going', 'x');
  await publishedPage(page, space.id, 'Below it', 'x', parent);
  const linker = await publishedPage(page, space.id, 'Points at it', 'x');
  await publish(page, linker, 'Points at it', 1, [
    {
      type: 'paragraph',
      content: [{ type: 'text', text: 'x', marks: [{ type: 'link', attrs: { href: `/pages/${parent}` } }] }],
    },
  ]);
  await page.goto(`/pages/${parent}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Delete' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('2 pages in all');
  await expect(dialog).toContainText('those links will stop working');
  await expect(dialog.getByRole('link', { name: 'Points at it' })).toBeVisible();
  await dialog.getByRole('button', { name: 'Delete' }).click();

  const undo = page.getByRole('button', { name: 'Undo' });
  await expect(undo).toBeVisible();
  await undo.click();
  await expect(page).toHaveURL(new RegExp(`/pages/${parent}$`));
  await expect(page.locator('h1')).toHaveText('Going');
});

test('an old version says what it is, and a comparison folds what did not change', async ({ page }) => {
  const space = await newSpace(page, 'Fold space');
  const id = await publishedPage(page, space.id, 'Folds', 'x');
  const lines = Array.from({ length: 12 }, (_, i) => words(`Line ${i}`));
  await publish(page, id, 'Folds', 1, lines);
  await publish(page, id, 'Folds', 2, [...lines, words('A new last line')]);

  await page.goto(`/pages/${id}/compare?from=2&to=3`);
  await expect(page.getByText('A new last line')).toBeVisible();
  await expect(page.getByText('Line 2', { exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: /Show 10 unchanged blocks/ }).click();
  await expect(page.getByText('Line 2', { exact: true })).toBeVisible();

  await page.goto(`/pages/${id}/history`);
  await page.getByRole('button', { name: 'View' }).last().click();
  await expect(page.getByRole('dialog')).toContainText('This is version 1');
  await expect(
    page.getByRole('dialog').getByRole('link', { name: 'Go to the current version' }),
  ).toBeVisible();
});

test('shortcuts work, never fire while typing, and can be turned off', async ({ page }) => {
  const space = await newSpace(page, 'Keys');
  const id = await publishedPage(page, space.id, 'Shortcut page', 'x');
  await page.goto(`/pages/${id}`);
  await expect(page.locator('h1')).toBeVisible();

  await page.keyboard.press('Shift+?');
  const overview = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
  await expect(overview).toBeVisible();
  await overview.getByLabel('Find a shortcut').fill('reader');
  await expect(overview.getByText('Reader mode')).toBeVisible();
  await expect(overview.getByText('Go home')).toHaveCount(0);
  await checkAccessibility(page);
  await overview.getByLabel('Find a shortcut').fill('');
  await page.keyboard.press('Escape');
  await expect(overview).toHaveCount(0);

  await page.keyboard.press('s');
  await expect(page.getByRole('button', { name: 'Remove from favourites' })).toBeVisible();
  await page.keyboard.press('s');
  await expect(page.getByRole('button', { name: 'Add to favourites' })).toBeVisible();

  await page.keyboard.press('i');
  await expect(page.getByRole('complementary', { name: 'Details' })).toBeVisible();
  await page.keyboard.press('Escape');

  await page.keyboard.press('[');
  await expect(page.locator('.sidebar')).toHaveCount(0);
  await page.keyboard.press('[');
  await expect(page.locator('.sidebar')).toBeVisible();

  await page.keyboard.press('e');
  await expect(page).toHaveURL(/\/edit$/);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.type('gh');
  await expect(page).toHaveURL(/\/edit$/);
  await expect(page.locator('.prose.editing')).toContainText('gh');

  await page.getByRole('link', { name: 'Close', exact: true }).click();
  // Close waits for the save, and a shortcut must not fire in the editor.
  await expect(page).not.toHaveURL(/\/edit$/);
  await page.keyboard.press('g');
  await page.keyboard.press('s');
  await expect(page).toHaveURL(/\/spaces$/);

  await page.goto('/me/preferences');
  await page.getByLabel('Keyboard shortcuts').uncheck();
  await page.locator('h1').click();
  await page.keyboard.press('g');
  await page.keyboard.press('h');
  await expect(page).toHaveURL(/\/me\/preferences$/);
  await page.getByLabel('Keyboard shortcuts').check();
});

test('a new space suggests its key, and says at once when it is taken', async ({ page }) => {
  const taken = await newSpace(page, 'Taken space');
  await page.goto('/spaces');
  await page.getByRole('button', { name: 'New space' }).click();
  const dialog = page.getByRole('dialog', { name: 'New space' });
  await dialog.getByLabel('Name').fill('Team Handbook');
  await expect(dialog.getByRole('textbox', { name: /^Key/ })).toHaveValue('TEAMHANDBOOK');
  await dialog.getByRole('textbox', { name: /^Key/ }).fill(taken.key);
  await expect(dialog.getByRole('alert')).toHaveText('This key is already in use.');
  await dialog.getByLabel('Name').fill('Something else');
  await expect(dialog.getByRole('textbox', { name: /^Key/ })).toHaveValue(taken.key);
});
