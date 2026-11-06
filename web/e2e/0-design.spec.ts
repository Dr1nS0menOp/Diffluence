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
  editorSettled,
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

const asDocument = (text: string) => ({ type: 'doc', content: [words(text)] });

const documentWithTask = (person: string) => ({
  type: 'doc',
  content: [
    words('We keep the kitchen rota up to date.'),
    {
      type: 'taskList',
      content: [
        {
          type: 'taskItem',
          attrs: { checked: false },
          content: [
            {
              type: 'paragraph',
              content: [
                { type: 'text', text: 'Empty the dishwasher ' },
                { type: 'mention', attrs: { id: person, kind: 'user', label: READER.name } },
                { type: 'text', text: ' ' },
                { type: 'date', attrs: { value: '2020-01-02' } },
              ],
            },
          ],
        },
      ],
    },
  ],
});

async function addComment(page: Page, pageId: string, data: object) {
  const res = await page.request.post(`/api/v1/pages/${pageId}/comments`, { data });
  expect(res.status(), await res.text()).toBe(201);
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
  let templateId = '';
  let readerId = '';
  let spaceId = '';

  test.beforeAll(async ({ browser }) => {
    const setup = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
    await login(setup, ADMIN.email, ADMIN.password);
    const created = await setup.request.post('/api/v1/spaces', {
      data: { key: 'HANDBOOK', name: 'Engineering Handbook', description: 'How we build and run things.' },
    });
    expect(created.status(), await created.text()).toBe(201);
    const space = (await created.json()) as { id: string; home_page_id: string };
    readerId = await createNamedUser(browser, READER.email, READER.name);
    spaceId = space.id;
    await grant(setup, space.id, { type: 'user', id: readerId }, [
      ...EDITOR,
      'comment.add',
      'comment.delete_own',
      'page.delete_any',
      'page.archive',
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
    await expect(page.getByRole('link', { name: 'Your first week' }).first()).toBeVisible();
    await picture(page, 'start', [page.locator('time')]);
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
    await editorSettled(page);
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
    // The width of a time moves the table, so it must not change between runs.
    await page.locator('td .muted').evaluateAll((times) => {
      times.forEach((time) => {
        time.textContent = '05/11/2026, 12:00:00';
      });
    });
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

  test('DirectoryView: all spaces', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces');
    await expect(page.getByRole('link', { name: 'Engineering Handbook' })).toBeVisible();
    await picture(page, 'directory', [page.locator('time')]);
    await page.context().close();
  });

  test('RecentView: recent pages', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/recent');
    await expect(page.getByRole('link', { name: 'Your first week' })).toBeVisible();
    await picture(page, 'recent', [page.locator('time')]);
    await page.context().close();
  });

  test('FavouritesView: favourites, empty', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/favourites');
    await expect(page.getByText('Press the star on a page')).toBeVisible();
    await picture(page, 'favourites');
    await page.context().close();
  });

  test('SpaceOverviewView: the overview of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/overview');
    await expect(page.getByRole('heading', { name: 'Latest changes' })).toBeVisible();
    await picture(page, 'overview', [page.locator('time')]);
    await page.context().close();
  });

  test('PreferencesView: preferences', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/me/preferences');
    await expect(page.getByLabel('Theme')).toBeVisible();
    await picture(page, 'preferences');
    await page.context().close();
  });

  test('the quick search panel', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();
    await page.keyboard.press('/');
    await page.keyboard.type('week');
    await expect(page.getByRole('option', { name: /Your first week/ })).toBeVisible();
    await picture(page, 'quick-search');
    await page.context().close();
  });

  test('a page with contents and details', async ({ browser }) => {
    const context = await browser.newContext({
      baseURL: 'http://localhost:8080',
      viewport: { width: 1500, height: 900 },
    });
    const page = await context.newPage();
    await login(page, READER.email, PASSWORD);
    const headed = await page.request.post('/api/v1/pages', {
      data: {
        space_id: (await (await page.request.get(`/api/v1/pages/${pageId}`)).json()).space_id,
        title: 'Long read',
      },
    });
    const id = ((await headed.json()) as { id: string }).id;
    const section = (text: string) => [
      { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text }] },
      words('Some words to read in this part of the page, so the column has something in it.'),
    ];
    const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
      data: {
        title: 'Long read',
        base_version: 0,
        document: { type: 'doc', content: [...section('One'), ...section('Two'), ...section('Three')] },
      },
    });
    expect(done.ok(), await done.text()).toBeTruthy();
    await page.goto(`/pages/${id}`);
    await expect(page.getByRole('navigation', { name: 'On this page' })).toBeVisible();
    await picture(page, 'page-contents', [page.getByTestId('last-updated')]);
    await page.keyboard.press('i');
    await expect(page.getByRole('complementary', { name: 'Details' })).toContainText('Contributors');
    await picture(page, 'page-details', [
      page.getByTestId('last-updated'),
      page.locator('.details-facts dd'),
    ]);
    await context.close();
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

  test('the home of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK');
    await expect(page.locator('.crumbs [aria-current]')).toHaveText('Home');
    await picture(page, 'space-home', [page.getByTestId('last-updated')]);
    await page.context().close();
  });

  test('the strip with more spaces than fit', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    for (const name of ['Customer care', 'Design', 'Finance', 'Human resources', 'Operations']) {
      const res = await page.request.post('/api/v1/spaces', {
        data: { key: name.slice(0, 4).toUpperCase(), name },
      });
      expect(res.status(), await res.text()).toBe(201);
    }
    await page.goto(`/pages/${pageId}`);
    await expect(page.getByRole('navigation', { name: 'Your spaces' }).getByRole('link')).toHaveCount(7);
    await picture(page, 'space-strip', [page.getByTestId('last-updated')]);
  });

  test('right-to-left', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await expect(page.getByRole('heading', { name: 'Your first week', level: 1 })).toBeVisible();
    await page.evaluate(() => document.documentElement.setAttribute('dir', 'rtl'));
    await picture(page, 'right-to-left', [page.getByTestId('last-updated')]);
    await page.context().close();
  });

  // Last on purpose: what these add to the handbook must not show up in the pictures above.
  test('a page with an owner, a status, labels and an approval', async ({ browser }) => {
    const setup = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
    await login(setup, ADMIN.email, ADMIN.password);
    const people = await setup.request.get('/api/v1/directory?q=Admin');
    const { entries } = (await people.json()) as { entries: { id: string; type: string }[] };
    const admin = entries.find((e) => e.type === 'user')?.id;
    await setup.context().close();

    const page = await newReaderPage(browser, 'light');
    const api = page.request;
    await api.put(`/api/v1/pages/${pageId}/stage`, { data: { text: 'Needs review', color: 'yellow' } });
    await api.put(`/api/v1/pages/${pageId}/verification`, { data: { until: '2099-01-01' } });
    await api.post(`/api/v1/pages/${pageId}/labels`, { data: { label: 'onboarding' } });
    await api.post(`/api/v1/pages/${pageId}/labels`, { data: { label: 'new-starters' } });
    const asked = await api.post(`/api/v1/pages/${pageId}/approvals`, {
      data: { user_ids: [admin], message: 'Is this still right?', due_on: '' },
    });
    expect(asked.status(), await asked.text()).toBe(201);
    await page.goto(`/pages/${pageId}`);
    await expect(page.getByText('Waiting for approval')).toBeVisible();
    await expect(page.getByRole('link', { name: 'onboarding', exact: true })).toBeVisible();
    await picture(page, 'page-organised', [page.getByTestId('last-updated')]);
    await checkAccessibility(page);
    await page.context().close();
  });

  test('LabelsView: labels of the space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/labels');
    await expect(page.getByRole('link', { name: /onboarding/ })).toBeVisible();
    await picture(page, 'labels');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('LabelView: pages with a label', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/labels/onboarding');
    await expect(page.getByRole('link', { name: 'Your first week' })).toBeVisible();
    await picture(page, 'label');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('ContentView: the table of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/content?sort=title');
    await expect(page.locator('#main').getByRole('link', { name: 'Your first week' })).toBeVisible();
    await page.getByRole('checkbox', { name: 'Your first week' }).check();
    await expect(page.getByText('1 selected')).toBeVisible();
    // A table takes its widths from its text, so "just now" must not move the columns.
    await page.locator('table time').evaluateAll((times) => {
      for (const time of times) time.textContent = 'a while ago';
    });
    await picture(page, 'content');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('BlogView: the blog of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    const space = (await (await page.request.get('/api/v1/space-keys/HANDBOOK')).json()) as { id: string };
    const made = await page.request.post('/api/v1/pages', {
      data: { space_id: space.id, title: 'We moved to the new office', kind: 'post' },
    });
    const post = (await made.json()) as { id: string };
    const done = await page.request.post(`/api/v1/pages/${post.id}/publish`, {
      data: { title: 'We moved to the new office', document: documentWithBlocks, base_version: 0 },
    });
    expect(done.ok(), await done.text()).toBeTruthy();
    await page.goto('/spaces/HANDBOOK/blog');
    await expect(page.getByRole('link', { name: 'We moved to the new office' })).toBeVisible();
    await picture(page, 'blog', [page.locator('.post-list .muted'), page.locator('h2')]);
    await checkAccessibility(page);
    await page.context().close();
  });

  test('ArchiveView: what left the tree', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    const space = (await (await page.request.get('/api/v1/space-keys/HANDBOOK')).json()) as {
      id: string;
      home_page_id: string;
    };
    const old = await publishedPage(page, space.id, 'The old way of working', 'x', space.home_page_id);
    const archived = await page.request.post(`/api/v1/pages/${old}/archive`, {
      data: { include_children: true },
    });
    expect(archived.ok(), await archived.text()).toBeTruthy();
    await page.goto('/spaces/HANDBOOK/archive');
    await expect(page.getByRole('link', { name: 'The old way of working' })).toBeVisible();
    await picture(page, 'archive', [page.locator('time')]);
    await checkAccessibility(page);
    await page.context().close();
  });

  test('TemplatesView: the templates of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    const space = (await (await page.request.get('/api/v1/space-keys/HANDBOOK')).json()) as { id: string };
    const made = await page.request.post(`/api/v1/spaces/${space.id}/templates`, {
      data: {
        name: 'Team update',
        description: 'What happened this week',
        category: 'Teams',
        labels: ['update'],
        title_pattern: 'Update {date}',
        steps: [],
        default_parent: null,
        hidden: false,
        promoted: false,
        document: {
          type: 'doc',
          content: [
            {
              type: 'paragraph',
              content: [
                { type: 'text', text: 'Goal: ' },
                { type: 'templateField', attrs: { name: 'Goal', kind: 'line' } },
                { type: 'text', text: ' ' },
                { type: 'instruction', attrs: { text: 'Say why this matters.' } },
              ],
            },
          ],
        },
      },
    });
    expect(made.status(), await made.text()).toBe(201);
    templateId = ((await made.json()) as { id: string }).id;
    await page.goto('/spaces/HANDBOOK/templates');
    await expect(page.getByRole('link', { name: 'Team update' })).toBeVisible();
    await picture(page, 'templates');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('TemplateView: writing a template', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/templates/${templateId}/edit`);
    await expect(page.locator('.template-field')).toBeVisible();
    await expect(page.locator('.instruction')).toBeVisible();
    await picture(page, 'template');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('the gallery of templates', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await page.getByRole('button', { name: 'More to create' }).click();
    await page.getByRole('menuitem', { name: 'From a template' }).click();
    const dialog = page.getByRole('dialog', { name: 'Start from a template' });
    await expect(dialog.getByRole('option').first()).toBeVisible();
    await expect(dialog.locator('.prose')).toBeVisible();
    await picture(page, 'gallery', [page.getByTestId('last-updated')]);
    await checkAccessibility(page);
    await page.context().close();
  });

  test('the publish options', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}/edit`);
    await page.getByRole('button', { name: 'Publish options' }).click();
    await expect(page.getByRole('dialog', { name: 'Publish options' })).toBeVisible();
    await picture(page, 'publish-options');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('a folder with its pages, dark', async ({ browser }) => {
    const page = await newReaderPage(browser, 'dark');
    const space = (await (await page.request.get('/api/v1/space-keys/HANDBOOK')).json()) as {
      id: string;
      home_page_id: string;
    };
    const folder = await publishedPage(page, space.id, 'Policies', 'x', space.home_page_id);
    await publishedPage(page, space.id, 'Travel', 'x', folder);
    const turned = await page.request.post(`/api/v1/pages/${folder}/kind`, { data: { kind: 'folder' } });
    expect(turned.ok(), await turned.text()).toBeTruthy();
    await page.goto(`/pages/${folder}`);
    await expect(page.getByRole('heading', { name: 'Child pages' })).toBeVisible();
    await picture(page, 'folder', [page.getByTestId('last-updated')]);
    await checkAccessibility(page);
    await page.context().close();
  });

  test('search with its filters', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/search?q=first');
    await expect(page.getByRole('link', { name: 'Your first week' }).first()).toBeVisible();
    await picture(page, 'search-filters');
    await checkAccessibility(page);
    await page.context().close();
  });

  // Last, because from here on the bell of the reader shows a number.
  test.describe('working together', () => {
    const QUOTE = 'the kitchen rota';
    let agreementsId = '';

    test.beforeAll(async ({ browser }) => {
      const setup = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
      await login(setup, ADMIN.email, ADMIN.password);
      agreementsId = await publishedPage(setup, spaceId, 'Team agreements', 'x');
      const published = await setup.request.post(`/api/v1/pages/${agreementsId}/publish`, {
        data: { title: 'Team agreements', base_version: 1, document: documentWithTask(readerId) },
      });
      expect(published.ok(), await published.text()).toBeTruthy();
      // Notifications arrive on their own time. The task comes first, so that the
      // order of the list is the same in every run.
      const reader = await newReaderPage(browser, 'light');
      await expect
        .poll(
          async () => {
            const res = await reader.request.get('/api/v1/me/notifications/count');
            return ((await res.json()) as { unread: number }).unread;
          },
          { timeout: 20_000 },
        )
        .toBeGreaterThan(0);
      await reader.context().close();
      const text = `We keep ${QUOTE} up to date.`;
      await addComment(setup, agreementsId, {
        body: asDocument('Could we also agree on the coffee machine?'),
      });
      await addComment(setup, agreementsId, {
        body: asDocument('This rota is out of date.'),
        anchor: {
          kind: 'text',
          exact: QUOTE,
          prefix: 'We keep ',
          suffix: ' up to date.',
          offset: text.indexOf(QUOTE),
          version: 2,
        },
      });
      const reacted = await setup.request.put(
        `/api/v1/pages/${agreementsId}/reactions/${encodeURIComponent('🎉')}`,
      );
      expect(reacted.ok(), await reacted.text()).toBeTruthy();
      const shared = await setup.request.post(`/api/v1/pages/${agreementsId}/share`, {
        data: {
          people: [{ type: 'user', id: readerId }],
          message: 'Please read this before Monday.',
          notify: true,
        },
      });
      expect(shared.ok(), await shared.text()).toBeTruthy();
      await setup.context().close();
    });

    test('NotificationsView: all notifications', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      await expect
        .poll(
          async () => {
            const res = await page.request.get('/api/v1/me/notifications/count');
            return ((await res.json()) as { unread: number }).unread;
          },
          { timeout: 20_000 },
        )
        .toBeGreaterThan(1);
      await page.goto('/notifications');
      await expect(page.getByText('Please read this before Monday.')).toBeVisible();
      await picture(page, 'notifications', [page.locator('time')]);
      await checkAccessibility(page);
      await page.context().close();
    });

    test('the bell with its panel', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      await page.getByRole('button', { name: /^Notifications, \d+ unread$/ }).click();
      const panel = page.getByRole('region', { name: 'Notifications' });
      await expect(panel.getByText('Please read this before Monday.')).toBeVisible();
      await picture(page, 'bell', [page.locator('time')]);
      await page.context().close();
    });

    for (const colorScheme of ['light', 'dark'] as const) {
      test(`comments under a page, ${colorScheme}`, async ({ browser }) => {
        const page = await newReaderPage(browser, colorScheme);
        await page.setViewportSize({ width: 1280, height: 1100 });
        await page.goto(`/pages/${agreementsId}`);
        await expect(page.getByRole('article', { name: /Comment by/ })).toBeVisible();
        await expect(page.locator('.comment-mark')).toBeVisible();
        await picture(page, `comments-${colorScheme}`, [
          page.locator('time'),
          page.getByTestId('last-updated'),
        ]);
        await checkAccessibility(page);
        await page.context().close();
      });
    }

    test('a thread beside the page', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      await page.goto(`/pages/${agreementsId}`);
      await page.locator('.comment-mark').click();
      const panel = page.getByRole('complementary', { name: 'Comments' });
      await expect(panel).toContainText('This rota is out of date.');
      await picture(page, 'thread-panel', [page.locator('time'), page.getByTestId('last-updated')]);
      await checkAccessibility(page);
      await page.context().close();
    });

    test('the share dialog', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      await page.goto(`/pages/${agreementsId}`);
      await page.getByRole('button', { name: 'Share' }).click();
      const dialog = page.getByRole('dialog', { name: 'Share this page' });
      await expect(dialog.getByText(READER.name)).toBeVisible();
      await picture(page, 'share', [page.getByTestId('last-updated')]);
      await page.context().close();
    });

    test('TasksView: your tasks', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      await page.goto('/tasks');
      await expect(page.getByRole('link', { name: /Empty the dishwasher/ })).toBeVisible();
      await picture(page, 'tasks');
      await checkAccessibility(page);
      await page.context().close();
    });

    test('WatchingView: what you watch', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      const watched = await page.request.put(`/api/v1/pages/${agreementsId}/watch`);
      expect(watched.ok(), await watched.text()).toBeTruthy();
      await page.goto('/me/watching');
      await expect(page.getByRole('link', { name: 'Team agreements' })).toBeVisible();
      await picture(page, 'watching', [page.locator('time')]);
      await page.context().close();
    });

    test('NotificationSettingsView: notification settings', async ({ browser }) => {
      const page = await newReaderPage(browser, 'light');
      await page.goto('/me/notifications');
      await expect(page.getByRole('heading', { name: 'Quiet hours' })).toBeVisible();
      await picture(page, 'notification-settings');
      await page.context().close();
    });

    test('UnsubscribeView: stopping a mail', async ({ page }) => {
      await page.goto('/unsubscribe/token');
      await expect(page.getByRole('heading', { name: 'Stop this mail' })).toBeVisible();
      await picture(page, 'unsubscribe');
    });
  });
});
