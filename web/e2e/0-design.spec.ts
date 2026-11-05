// SPDX-License-Identifier: Apache-2.0
// The design standard as pictures (docs/design/README.md). A screen that drifts
// from its approved look fails here. When a change is meant to look different:
//   scripts/e2e design.spec.ts --update-snapshots
// and look at the new pictures in web/e2e/design-baselines/ before committing.
// The 0 makes this file run first: the pictures need a database that the other
// tests have not touched yet, because those share spaces with all members.
import type { Browser, Locator, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createNamedUser,
  EDITOR,
  expect,
  grant,
  login,
  PASSWORD,
  publishedPage,
  test,
} from './helpers';

const READER = { email: 'anna.visser@example.com', name: 'Anna Visser' };
const NARROW_SCREEN = { width: 420, height: 800 };

const FILE_NAME = 'Welcome pack.pdf';

const documentWithEverything = (fileId: string) => ({
  type: 'doc',
  content: [
    {
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Welcome to the team. This page explains how we ' },
        { type: 'text', marks: [{ type: 'bold' }], text: 'work together' },
        { type: 'text', text: ', and where to find what you need in your ' },
        { type: 'text', marks: [{ type: 'code' }], text: 'first week' },
        { type: 'text', text: '.' },
      ],
    },
    { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'Your first day' }] },
    {
      type: 'bulletList',
      content: ['Meet your buddy', 'Set up your laptop', 'Read the handbook'].map((text) => ({
        type: 'listItem',
        content: [{ type: 'paragraph', content: [{ type: 'text', text }] }],
      })),
    },
    {
      type: 'blockquote',
      content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Ask early, ask often.' }] }],
    },
    {
      type: 'codeBlock',
      attrs: { language: 'js' },
      content: [{ type: 'text', text: "const greeting = 'hello';" }],
    },
    { type: 'file', attrs: { attachment_id: fileId, filename: FILE_NAME } },
  ],
});

const words = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const coloured = (color: string) => ({
  type: 'text',
  marks: [{ type: 'textColor', attrs: { color } }],
  text: `${color} `,
});
const cell = (text: string, background?: string) => ({
  type: 'tableCell',
  attrs: { background },
  content: [words(text)],
});

/** One of everything a writer can add, to see them together in both themes. */
const documentWithBlocks = {
  type: 'doc',
  content: [
    {
      type: 'paragraph',
      content: [
        ...['grey', 'slate', 'red', 'orange', 'amber', 'green'].map(coloured),
        ...['teal', 'blue', 'indigo', 'purple', 'pink', 'brown'].map(coloured),
      ],
    },
    {
      type: 'paragraph',
      content: ['grey', 'yellow', 'green', 'blue', 'purple', 'red'].flatMap((color) => [
        { type: 'text', marks: [{ type: 'highlight', attrs: { color } }], text: color },
        { type: 'text', text: ' ' },
        { type: 'status', attrs: { text: 'Status', color } },
        { type: 'text', text: ' ' },
      ]),
    },
    {
      type: 'paragraph',
      content: [
        { type: 'text', text: 'Agreed on ' },
        { type: 'date', attrs: { value: '2026-11-05' } },
        { type: 'text', text: ' with H' },
        { type: 'text', marks: [{ type: 'subscript' }], text: '2' },
        { type: 'text', text: 'O and E=mc' },
        { type: 'text', marks: [{ type: 'superscript' }], text: '2' },
      ],
    },
    { type: 'paragraph', attrs: { align: 'center' }, content: [{ type: 'text', text: 'In the centre' }] },
    { type: 'paragraph', attrs: { indent: 1 }, content: [{ type: 'text', text: 'One step in' }] },
    ...['info', 'note', 'success', 'warning', 'error'].map((kind) => ({
      type: 'panel',
      attrs: { kind },
      content: [words(`A panel of the kind ${kind}.`)],
    })),
    {
      type: 'expand',
      attrs: { title: 'A section that starts open', open: true },
      content: [
        words('Words inside the section.'),
        { type: 'expand', attrs: { title: 'A section inside it' }, content: [words('Hidden.')] },
      ],
    },
    {
      type: 'columns',
      attrs: { layout: 'wide-start' },
      content: [
        { type: 'column', content: [words('The wide first column holds the main text of this part.')] },
        { type: 'column', content: [words('A narrow second column.')] },
      ],
    },
    { type: 'decision', content: [{ type: 'text', text: 'We go ahead with the new editor.' }] },
    {
      type: 'codeBlock',
      attrs: { language: 'js', title: 'greeting.js', line_numbers: true },
      content: [{ type: 'text', text: "const greeting = 'hello';\nconsole.log(greeting);" }],
    },
    {
      type: 'table',
      content: [
        {
          type: 'tableRow',
          content: [
            { type: 'tableHeader', content: [words('Name')] },
            { type: 'tableHeader', content: [words('State')] },
          ],
        },
        { type: 'tableRow', content: [cell('Editor', 'green'), cell('Done')] },
        { type: 'tableRow', content: [cell('Reading', 'yellow'), cell('Next')] },
      ],
    },
  ],
};

/**
 * Pictures must not depend on which font the test image calls its system font,
 * so they use one font on purpose. The app itself keeps the system font.
 */
async function useFixedFont(page: Page) {
  await page.evaluate(() => {
    document.documentElement.style.setProperty('--font-system', "'Liberation Sans', sans-serif");
  });
}

async function picture(page: Page, name: string, mask: Locator[] = []) {
  await useFixedFont(page);
  await page.evaluate(() => document.fonts.ready);
  await expect(page).toHaveScreenshot(`${name}.png`, { mask });
}

async function newReaderPage(browser: Browser, colorScheme: 'light' | 'dark'): Promise<Page> {
  const context = await browser.newContext({ baseURL: 'http://localhost:8080', colorScheme });
  const page = await context.newPage();
  await login(page, READER.email, PASSWORD);
  return page;
}

test.describe('the look of every kind of screen', () => {
  // One set-up for all pictures: a failure must not run it a second time.
  test.describe.configure({ mode: 'serial' });
  let pageId = '';
  let blocksPageId = '';

  test.beforeAll(async ({ browser }) => {
    const setup = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
    await login(setup, ADMIN.email, ADMIN.password);
    const created = await setup.request.post('/api/v1/spaces', {
      data: { key: 'HANDBOOK', name: 'Engineering Handbook', description: 'How we build and run things.' },
    });
    expect(created.status(), await created.text()).toBe(201);
    const space = (await created.json()) as { id: string; home_page_id: string };
    const reader = await createNamedUser(browser, READER.email, READER.name);
    await grant(setup, space.id, { type: 'user', id: reader }, [
      ...EDITOR,
      'page.delete_any',
      'space.administer',
    ]);

    const onboarding = await publishedPage(setup, space.id, 'Onboarding', 'x', space.home_page_id);
    await publishedPage(setup, space.id, 'Tools we use', 'x', onboarding);
    await publishedPage(setup, space.id, 'Decisions', 'x', space.home_page_id);
    pageId = await publishedPage(setup, space.id, 'Your first week', 'x', onboarding);
    const uploaded = await setup.request.post(
      `/api/v1/pages/${pageId}/attachments?filename=${encodeURIComponent(FILE_NAME)}`,
      { headers: { 'Content-Type': 'application/octet-stream' }, data: Buffer.from('%PDF-1.4 welcome') },
    );
    expect(uploaded.status(), await uploaded.text()).toBe(201);
    const file = (await uploaded.json()) as { id: string };
    const published = await setup.request.post(`/api/v1/pages/${pageId}/publish`, {
      data: {
        title: 'Your first week',
        document: documentWithEverything(file.id),
        base_version: 1,
        summary: 'Added the first day',
      },
    });
    expect(published.ok(), await published.text()).toBeTruthy();
    blocksPageId = await publishedPage(setup, space.id, 'Every block', 'x', onboarding);
    const blocks = await setup.request.post(`/api/v1/pages/${blocksPageId}/publish`, {
      data: { title: 'Every block', document: documentWithBlocks, base_version: 1 },
    });
    expect(blocks.ok(), await blocks.text()).toBeTruthy();
    await setup.context().close();
  });

  test('LoginView: the login card', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByRole('heading', { name: 'Log in' })).toBeVisible();
    await picture(page, 'login');
  });

  test('HomeView: the start screen', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await expect(page.getByRole('heading', { name: 'Your first week', level: 1 })).toBeVisible();
    await page.goto('/');
    await expect(page.getByRole('link', { name: 'Your first week' })).toBeVisible();
    await picture(page, 'start');
    await page.context().close();
  });

  for (const colorScheme of ['light', 'dark'] as const) {
    test(`PageView: a page in the space, ${colorScheme}`, async ({ browser }) => {
      const page = await newReaderPage(browser, colorScheme);
      await page.goto(`/pages/${pageId}`);
      await expect(page.locator('.prose pre .hljs-keyword')).toBeVisible();
      await expect(page.getByRole('link', { name: 'Tools we use' })).toBeVisible();
      await picture(page, `page-${colorScheme}`, [page.getByTestId('last-updated')]);
      await page.context().close();
    });
  }

  test('the more-actions menu', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await page.getByRole('button', { name: 'More actions' }).click();
    await expect(page.getByRole('menuitem', { name: 'History' })).toBeVisible();
    await picture(page, 'page-menu', [page.getByTestId('last-updated')]);
    await page.context().close();
  });

  test('the attachments of a page', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await page.getByRole('button', { name: 'More actions' }).click();
    await page.getByRole('menuitem', { name: 'Attachments' }).click();
    await expect(page.getByRole('dialog').getByRole('link', { name: FILE_NAME })).toBeVisible();
    await picture(page, 'attachments', [page.getByTestId('last-updated')]);
    await page.context().close();
  });

  test('a collapsed branch', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    const tree = page.getByRole('navigation', { name: 'Pages' });
    await tree.getByRole('button', { name: 'Onboarding', exact: true }).click();
    await expect(tree.getByRole('link', { name: 'Tools we use' })).toHaveCount(0);
    await picture(page, 'tree-collapsed', [page.getByTestId('last-updated')]);
    await page.context().close();
  });

  for (const colorScheme of ['light', 'dark'] as const) {
    test(`EditView: the editor, ${colorScheme}`, async ({ browser }) => {
      const page = await newReaderPage(browser, colorScheme);
      await page.goto(`/pages/${pageId}/edit`);
      await expect(page.getByRole('toolbar', { name: 'Formatting' })).toBeVisible();
      await picture(page, `editor-${colorScheme}`);
      await page.context().close();
    });
  }

  for (const colorScheme of ['light', 'dark'] as const) {
    test(`every block a writer can add, ${colorScheme}`, async ({ browser }) => {
      const page = await newReaderPage(browser, colorScheme);
      await page.setViewportSize({ width: 1280, height: 1500 });
      await page.goto(`/pages/${blocksPageId}`);
      await expect(page.locator('.prose .decision')).toBeVisible();
      await expect(page.locator('.prose details details')).toHaveCount(1);
      await picture(page, `blocks-${colorScheme}`, [page.getByTestId('last-updated')]);
      await checkAccessibility(page);
      await page.context().close();
    });
  }

  test('the blocks on a narrow screen', async ({ browser }) => {
    const context = await browser.newContext({
      baseURL: 'http://localhost:8080',
      viewport: { ...NARROW_SCREEN, height: 1900 },
    });
    const page = await context.newPage();
    await login(page, READER.email, PASSWORD);
    await page.goto(`/pages/${blocksPageId}`);
    await expect(page.locator('.prose .decision')).toBeVisible();
    await picture(page, 'blocks-narrow', [page.getByTestId('last-updated')]);
    await context.close();
  });

  test('the editor with its handle and the slash menu', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}/edit`);
    const body = page.locator('.prose.editing');
    await body.locator('h2').click();
    await page.keyboard.press('End');
    await page.keyboard.press('Enter');
    await page.keyboard.type('/');
    await expect(page.getByRole('listbox', { name: 'Insert' })).toBeVisible();
    await body.locator('h2').hover();
    await expect(page.locator('.block-grip')).toBeVisible();
    await picture(page, 'editor-slash');
    await page.keyboard.press('Escape');
    await page.keyboard.press('Control+z');
    await page.context().close();
  });

  test('a confirmation', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await page.getByRole('button', { name: 'More actions' }).click();
    await page.getByRole('menuitem', { name: 'Delete' }).click();
    await expect(page.getByRole('dialog', { name: 'Delete' })).toBeVisible();
    await picture(page, 'confirm', [page.getByTestId('last-updated')]);
    await page.context().close();
  });

  test('SpaceSettingsView: space settings', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/settings');
    await expect(page.getByRole('heading', { name: 'Permissions' })).toBeVisible();
    await picture(page, 'space-settings');
    await page.context().close();
  });

  test('AdminView: the admin console', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    await page.goto('/admin/settings');
    await expect(page.getByRole('heading', { name: 'Appearance' })).toBeVisible();
    await picture(page, 'admin-settings');
  });

  test('HistoryView: version history', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}/history`);
    await expect(page.getByText('Added the first day')).toBeVisible();
    await picture(page, 'history', [page.locator('td .muted')]);
    await page.context().close();
  });

  test('CompareView: two versions side by side', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}/compare?from=1&to=2`);
    await expect(page.getByRole('heading', { name: 'Compare versions' })).toBeVisible();
    await expect(page.locator('.diff').first()).toBeVisible();
    await picture(page, 'compare');
    await page.context().close();
  });

  test('SearchView: search results', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/search?q=buddy');
    await expect(page.locator('mark')).toHaveText('buddy');
    await picture(page, 'search');
    await page.context().close();
  });

  test('ProfileView: your account', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/me');
    await expect(page.getByRole('heading', { name: 'Change password' })).toBeVisible();
    await picture(page, 'profile', [page.locator('table')]);
    await page.context().close();
  });

  test('TrashView: an empty trash', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/trash');
    await expect(page.getByText('The trash is empty.')).toBeVisible();
    await picture(page, 'trash');
    await page.context().close();
  });

  test('a narrow screen', async ({ browser }) => {
    const context = await browser.newContext({
      baseURL: 'http://localhost:8080',
      viewport: NARROW_SCREEN,
    });
    const page = await context.newPage();
    await login(page, READER.email, PASSWORD);
    await page.goto(`/pages/${pageId}`);
    await expect(page.getByRole('heading', { name: 'Your first week', level: 1 })).toBeVisible();
    await picture(page, 'narrow', [page.getByTestId('last-updated')]);
    await context.close();
  });

  test('right-to-left', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await expect(page.getByRole('heading', { name: 'Your first week', level: 1 })).toBeVisible();
    await page.evaluate(() => document.documentElement.setAttribute('dir', 'rtl'));
    await picture(page, 'right-to-left', [page.getByTestId('last-updated')]);
    await page.context().close();
  });
});
