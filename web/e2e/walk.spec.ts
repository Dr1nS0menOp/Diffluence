// SPDX-License-Identifier: Apache-2.0
// What a walk through the real screens found (docs/handoff/2026-11-06-ui-test.md).
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  createUser,
  EDITOR,
  editorSettled,
  expect,
  grant,
  login,
  newSpace,
  publishedPage,
  test,
  unique,
} from './helpers';

const DESKTOP = { width: 1440, height: 900 };
const PHONE = { width: 390, height: 844 };
const NOT_FOUND = 'This page does not exist, or you cannot see it.';
const UNBROKEN_TITLE = `Unbroken${'y'.repeat(180)}`;

test.beforeEach(async ({ page }) => {
  await page.setViewportSize(DESKTOP);
  await login(page, ADMIN.email, ADMIN.password);
});

async function secondPerson(browser: Browser, email: string) {
  const context = await browser.newContext({ baseURL: process.env.BASE_URL ?? 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, email);
  return other;
}

const scrollsSideways = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);

/** Counts what the screen sends, for what a double click must not send twice. */
function countPosts(page: Page, path: RegExp) {
  const seen: string[] = [];
  page.on('request', (r) => {
    if (r.method() === 'POST' && path.test(new URL(r.url()).pathname)) seen.push(r.url());
  });
  return seen;
}

test('every placeholder in the administration fits its field', async ({ page }) => {
  for (const address of ['/admin/users', '/admin/templates']) {
    await page.goto(address);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    await expect(page.locator('main table')).toBeVisible();
    const cut = await page.locator('main input[placeholder]').evaluateAll((fields) => {
      const ruler = document.createElement('canvas').getContext('2d');
      return (fields as HTMLInputElement[])
        .filter((field) => {
          if (!ruler) return false;
          const style = getComputedStyle(field);
          ruler.font = `${style.fontSize} ${style.fontFamily}`;
          const room =
            field.clientWidth - parseFloat(style.paddingInlineStart) - parseFloat(style.paddingInlineEnd);
          return ruler.measureText(field.placeholder).width > room;
        })
        .map((field) => field.placeholder);
    });
    expect(cut).toEqual([]);
  }
});

test('a button on its own in a settings screen keeps the size of its words', async ({ page }) => {
  await page.goto('/admin/mail');
  const mail = await page.getByRole('button', { name: 'Send a test mail to yourself' }).boundingBox();
  expect(mail?.width ?? 0).toBeLessThan(400);
  await page.goto('/admin/settings');
  const example = await page.getByRole('button', { name: 'Add example space' }).boundingBox();
  expect(example?.width ?? 0).toBeLessThan(400);
});

test('an address in the administration that does not exist says so', async ({ page }) => {
  await page.goto('/admin/nosuch');
  await expect(page.getByRole('alert')).toHaveText(NOT_FOUND);
  await expect(page.getByRole('heading', { name: 'Administration' })).toHaveCount(0);
});

test('someone without spaces is not told about a filter they never set', async ({ browser }) => {
  const email = `${unique('guest')}@example.com`;
  await createUser(browser, email, 'guest');
  const guest = await secondPerson(browser, email);
  await guest.goto('/spaces');
  await expect(guest.getByText('There are no spaces you can see yet.')).toBeVisible();
  await expect(guest.getByText('No space fits this filter.')).toHaveCount(0);
  await guest.context().close();
});

test('the tabs of the comments panel stay on one line', async ({ page }) => {
  const space = await newSpace(page, 'Panel tabs');
  const id = await publishedPage(page, space.id, 'Tabs', 'text');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Comments', exact: true }).click();
  const tabs = page.locator('.comments-panel [role="tab"]');
  await expect(tabs.first()).toBeVisible();
  const linesPerTab = await tabs.evaluateAll((all) =>
    all.map((tab) => {
      const words = document.createRange();
      words.selectNodeContents(tab);
      return new Set(Array.from(words.getClientRects()).map((line) => Math.round(line.top))).size;
    }),
  );
  expect(linesPerTab.filter((lines) => lines > 1)).toEqual([]);
  const panel = await page.locator('.comments-panel').boundingBox();
  const last = await tabs.last().boundingBox();
  expect((last?.x ?? 0) + (last?.width ?? 0)).toBeLessThanOrEqual((panel?.x ?? 0) + (panel?.width ?? 0));
});

test('the menu of a page uses the height of the window', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const space = await newSpace(page, 'Long menu');
  const id = await publishedPage(page, space.id, 'Menu', 'text');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await expect(page.getByRole('menuitem', { name: 'Delete' })).toBeInViewport({ ratio: 1 });
});

test('an empty list says it is empty, instead of showing only a head', async ({ page }) => {
  await page.route('**/api/v1/admin/groups', (route) =>
    route.request().method() === 'GET' ? route.fulfill({ json: { groups: [] } }) : route.fallback(),
  );
  await page.route('**/api/v1/admin/scim-tokens', (route) =>
    route.request().method() === 'GET' ? route.fulfill({ json: { tokens: [] } }) : route.fallback(),
  );
  await page.goto('/admin/groups');
  await expect(page.getByText('No groups yet.')).toBeVisible();
  await expect(page.locator('main table')).toHaveCount(0);
  await page.goto('/admin/scim');
  await expect(page.getByText('No tokens yet.')).toBeVisible();
  await expect(page.locator('main table')).toHaveCount(0);
});

test('after publishing, the menu knows that the author watches the page', async ({ page }) => {
  const space = await newSpace(page, 'Watching');
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, parent_id: null, title: 'Mine' },
  });
  const { id } = (await created.json()) as { id: string };
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.type('Some text.');
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('link', { name: 'Edit', exact: true })).toBeVisible();
  // The watch starts a moment after the publish, so someone may have to look twice.
  await expect(async () => {
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: 'More actions' }).click();
    await expect(page.getByRole('menuitem', { name: 'Stop watching this page', exact: true })).toBeVisible({
      timeout: 1000,
    });
  }).toPass();
});

test('a double click on Create makes one page', async ({ page }) => {
  const space = await newSpace(page, 'Double create');
  await page.goto(`/spaces/${space.key}`);
  const posts = countPosts(page, /\/api\/v1\/pages$/);
  await page
    .getByRole('navigation', { name: 'Main' })
    .getByRole('button', { name: 'Create', exact: true })
    .click();
  const dialog = page.getByRole('dialog', { name: 'New page' });
  await dialog.getByLabel('Title').fill('Only once');
  await dialog.getByRole('button', { name: 'Create', exact: true }).dblclick();
  await expect(page.locator('.prose.editing')).toBeVisible();
  expect(posts).toHaveLength(1);
});

test('a double click on Post leaves one comment', async ({ page }) => {
  const space = await newSpace(page, 'Double post');
  const id = await publishedPage(page, space.id, 'Talk', 'text');
  await page.goto(`/pages/${id}`);
  const posts = countPosts(page, /\/comments$/);
  await page.locator('.comment-box .comment-input').click();
  await page.keyboard.type('Said once.');
  await page.getByRole('button', { name: 'Post', exact: true }).dblclick();
  await expect(page.getByRole('article').getByText('Said once.').first()).toBeVisible();
  expect(posts).toHaveLength(1);
  await page.locator('.comment-box .comment-input').last().click();
  await page.keyboard.type('And once more.');
  await page.getByRole('button', { name: 'Post', exact: true }).click();
  await expect(page.getByRole('article').getByText('And once more.')).toBeVisible();
});

test('a title without spaces stays inside the window', async ({ page }) => {
  const space = await newSpace(page, 'Long title');
  const id = await publishedPage(page, space.id, UNBROKEN_TITLE, 'text');
  await page.goto(`/pages/${id}`);
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  expect(await scrollsSideways(page)).toBe(false);
  await page.goto(`/spaces/${space.key}/content`);
  await expect(page.locator('#main').getByRole('link', { name: UNBROKEN_TITLE })).toBeVisible();
  expect(await scrollsSideways(page)).toBe(false);
});

test('the colours of a page status have names', async ({ page }) => {
  const space = await newSpace(page, 'Status colours');
  await page.goto(`/spaces/${space.key}/settings?section=statuses`);
  await expect(page.getByRole('heading', { name: 'Page statuses', level: 1 })).toBeVisible();
  const options = await page.locator('main option').allTextContents();
  expect(options.filter((text) => text.startsWith('editor.'))).toEqual([]);
});

test('the button of a tall dialog stays in reach', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'New space' }).click();
  const dialog = page.getByRole('dialog', { name: 'New space' });
  await expect(dialog.getByRole('button', { name: 'Create', exact: true })).toBeInViewport({ ratio: 1 });
});

test('the words on an empty page keep their distance from the sidebar', async ({ page }) => {
  const space = await newSpace(page, 'Empty start');
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, parent_id: null, title: 'Blank' },
  });
  const { id } = (await created.json()) as { id: string };
  await page.goto(`/pages/${id}/edit`);
  const hint = page.getByText('An empty page.');
  await expect(hint).toBeVisible();
  const inset = await hint.evaluate((node) => parseFloat(getComputedStyle(node).paddingInlineStart));
  expect(inset).toBeGreaterThan(0);
});

test('someone else in the editor does not push Publish onto a line of its own', async ({ page, browser }) => {
  const space = await newSpace(page, 'Two editors');
  const email = `${unique('second')}@example.com`;
  const id = await publishedPage(page, space.id, 'Shared', 'text');
  const userId = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: userId }, EDITOR);
  await page.goto(`/pages/${id}/edit`);
  const other = await secondPerson(browser, email);
  await other.setViewportSize(DESKTOP);
  await other.goto(`/pages/${id}/edit`);
  await expect(other.getByText(/^Also editing/)).toBeVisible();
  const close = await other.getByRole('link', { name: 'Close' }).boundingBox();
  const publish = await other.getByRole('button', { name: 'Publish', exact: true }).boundingBox();
  expect(Math.abs((close?.y ?? 0) - (publish?.y ?? 0))).toBeLessThan(8);
  await other.context().close();
});

test('on a phone, a word in a table stays whole and the table scrolls', async ({ page }) => {
  const space = await newSpace(page, 'Phone table');
  const cell = {
    type: 'tableCell',
    content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Handbook' }] }],
  };
  const row = { type: 'tableRow', content: [cell, cell, cell, cell, cell, cell] };
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: space.id, parent_id: null, title: 'Wide' },
  });
  const { id } = (await created.json()) as { id: string };
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Wide',
      document: { type: 'doc', content: [{ type: 'table', content: [row] }] },
      base_version: 0,
    },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  await page.setViewportSize(PHONE);
  await page.goto(`/pages/${id}`);
  const word = page.locator('.prose td p').first();
  await expect(word).toBeVisible();
  const lineHeight = await word.evaluate((node) => parseFloat(getComputedStyle(node).lineHeight));
  const box = await word.boundingBox();
  expect(box?.height ?? 0).toBeLessThan(lineHeight * 1.5);
  expect(await scrollsSideways(page)).toBe(false);
});
