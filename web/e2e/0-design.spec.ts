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
  adminRequest,
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

/** Gives every match the same text, so a time cannot change the width of what holds it. */
async function fixText(where: Locator, text: string) {
  await where.evaluateAll((all, fixed) => {
    all.forEach((element) => {
      element.textContent = fixed;
    });
  }, text);
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
      'site.publish',
      'space.automation',
      'space.templates',
      'space.audit',
      'space.labels',
      'space.statuses',
      'space.approvals',
      'space.bundle',
      'space.look',
      'page.watchers',
      'page.move_out',
      'space.invite_guests',
      'analytics.view',
      'space.export',
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

  for (const colorScheme of ['light', 'dark'] as const) {
    test(`SetupView: the first setup of a new install, ${colorScheme}`, async ({ browser }) => {
      // This instance has an administrator, so the server would say the setup is over. The
      // picture shows the form a new install shows; its rules are tested in Go (spec 038).
      const context = await browser.newContext({ baseURL: 'http://localhost:8080', colorScheme });
      const page = await context.newPage();
      await page.route('**/api/v1/setup', (route) =>
        route.request().method() === 'GET'
          ? route.fulfill({ status: 200, contentType: 'application/json', body: '{"open":true}' })
          : route.fallback(),
      );
      await page.goto('/setup');
      await expect(page.getByRole('heading', { name: 'Set up Diffluence' })).toBeVisible();
      await expect(page.getByLabel('Setup code')).toBeFocused();
      await checkAccessibility(page);
      await picture(page, `setup-${colorScheme}`);
      await context.close();
    });
  }

  test('SetupView: once set up, the screen says so', async ({ page }) => {
    await page.goto('/setup');
    await expect(page.getByRole('heading', { name: 'Already set up' })).toBeVisible();
    await expect(page.getByLabel('Setup code')).toHaveCount(0);
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

  test('the table button and its menu', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    // Tall enough that the table needs no scroll. Scrolled, a field ends under the sticky bar,
    // and axe then counts only buttons as cover: whether it passes depends on the gaps between them.
    await page.setViewportSize({ width: 1280, height: 1400 });
    await page.goto(`/pages/${blocksPageId}/edit`);
    await page.locator('.prose.editing td').first().click();
    await editorSettled(page);
    await page.getByRole('button', { name: 'Table options' }).click();
    await expect(page.getByRole('menu', { name: 'Table options' })).toBeVisible();
    await picture(page, 'table-menu');
    await checkAccessibility(page);
    await page.context().close();
  });

  test('the title area in the editor', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}/edit`);
    await page.locator('.page-head').hover();
    await expect(page.getByRole('button', { name: 'Add emoji' })).toBeVisible();
    await picture(page, 'editor-title');
    await page.context().close();
  });

  test('a stretched table and a large screen', async ({ browser }) => {
    const context = await browser.newContext({
      baseURL: 'http://localhost:8080',
      viewport: { width: 1920, height: 900 },
    });
    const page = await context.newPage();
    await login(page, READER.email, PASSWORD);
    const spaceId = (
      (await (await page.request.get(`/api/v1/pages/${pageId}`)).json()) as { space_id: string }
    ).space_id;
    const made = await page.request.post('/api/v1/pages', {
      data: { space_id: spaceId, title: 'Wide table' },
    });
    const id = ((await made.json()) as { id: string }).id;
    const row = (cells: string[], type: string) => ({
      type: 'tableRow',
      content: cells.map((text) => ({ type, content: [words(text)] })),
    });
    const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
      data: {
        title: 'Wide table',
        base_version: 0,
        document: {
          type: 'doc',
          content: [
            words('The text keeps a comfortable width, and the table below reaches past it.'),
            {
              type: 'table',
              attrs: { stretch: 100 },
              content: [
                row(['Team', 'Monday', 'Tuesday', 'Wednesday'], 'tableHeader'),
                row(['Docs', 'Review', 'Write', 'Publish'], 'tableCell'),
              ],
            },
          ],
        },
      },
    });
    expect(done.ok(), await done.text()).toBeTruthy();
    await page.goto(`/pages/${id}`);
    await expect(page.locator('.prose table')).toBeVisible();
    await picture(page, 'table-stretched', [page.getByTestId('last-updated')]);
    await context.close();
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

  test('FilesView: every file of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/files');
    await expect(page.getByRole('heading', { name: 'Files', exact: true })).toBeVisible();
    // The Changed column shows the day the file was added, so the picture would change each day.
    await fixText(page.locator('td').filter({ hasText: /^[A-Z][a-z]{2} \d{1,2}, \d{4}$/ }), 'Nov 8, 2026');
    await picture(page, 'files');
    await page.context().close();
  });

  test('SpaceSettingsView: space settings', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/settings');
    await expect(page.getByRole('heading', { name: 'Details', level: 1 })).toBeVisible();
    await picture(page, 'space-settings');
    await page.context().close();
  });

  test('SitesView: a public site of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    const { api, close } = await adminRequest(browser);
    const on = await api.put('/api/v1/admin/export-policy', {
      data: {
        rules: [],
        limits: {
          max_export_mib: 100,
          max_import_mib: 100,
          public_sites_enabled: true,
          public_links_enabled: true,
        },
      },
    });
    expect(on.status()).toBe(204);
    await close();
    const made = await page.request.post(`/api/v1/spaces/${spaceId}/sites`, {
      data: { name: 'Handbook online' },
    });
    expect(made.status(), await made.text()).toBe(201);
    await page.goto('/spaces/HANDBOOK/sites');
    await expect(page.getByRole('heading', { name: 'Handbook online' })).toBeVisible();
    await picture(page, 'sites');
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
    await expect(page.locator('.diff-block').first()).toBeVisible();
    await picture(page, 'compare');
    await page.context().close();
  });

  test('a version with its changes', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}/history`);
    await page.getByRole('button', { name: 'View' }).first().click();
    await expect(page.getByRole('dialog').locator('.diff-block').first()).toBeVisible();
    await picture(page, 'version-changes', [page.getByRole('dialog').getByRole('status')]);
    await page.context().close();
  });

  test('a picker with people', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await page.getByRole('button', { name: 'Restrictions' }).click();
    await page.getByRole('dialog').getByRole('combobox', { name: 'Check a person' }).click();
    await expect(page.getByRole('dialog').getByRole('listbox').getByRole('option').first()).toBeVisible();
    await picture(page, 'picker');
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
    await expect(page.getByRole('heading', { name: 'About you' })).toBeVisible();
    await picture(page, 'profile');
    await page.context().close();
  });

  test('SecurityView: how you sign in', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/me/security');
    await expect(page.getByRole('heading', { name: 'Change password' })).toBeVisible();
    await picture(page, 'security', [page.locator('table')]);
    await page.context().close();
  });

  test('TokensView: personal tokens', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/me/tokens');
    await expect(page.getByRole('heading', { name: 'New token' })).toBeVisible();
    await picture(page, 'tokens', [page.getByLabel('Expires')]);
    await page.context().close();
  });

  test('PersonalDataView: your data', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/me/data');
    await expect(page.getByRole('heading', { name: 'Ask for removal' })).toBeVisible();
    await picture(page, 'personal-data');
    await page.context().close();
  });

  test('PeopleView: the directory', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/people');
    await expect(page.getByRole('link', { name: READER.name })).toBeVisible();
    await picture(page, 'people');
    await page.context().close();
  });

  for (const colorScheme of ['light', 'dark'] as const) {
    test(`PersonView: a profile, ${colorScheme}`, async ({ browser }) => {
      const page = await newReaderPage(browser, colorScheme);
      const me = (await (await page.request.get('/api/v1/me')).json()) as { user: { id: string } };
      await page.request.put('/api/v1/me/profile', {
        data: {
          job_title: 'Site reliability engineer',
          department: 'Platform',
          location: 'Utrecht',
          about: 'Keeps the lights on, and writes things down.',
          visibility: {},
        },
      });
      await page.goto(`/people/${me.user.id}`);
      await expect(page.getByRole('heading', { name: READER.name, level: 1 })).toBeVisible();
      await picture(page, `person-${colorScheme}`);
      await page.context().close();
    });
  }

  test('GroupView: the screen of a group owner', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    const me = (await (await page.request.get('/api/v1/me')).json()) as { user: { id: string } };
    const admin = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
    await login(admin, ADMIN.email, ADMIN.password);
    const made = await admin.request.post('/api/v1/admin/groups', { data: { name: 'Platform team' } });
    expect(made.status(), await made.text()).toBe(201);
    const group = (await made.json()) as { id: string };
    await admin.request.put(`/api/v1/admin/groups/${group.id}/owners`, { data: { owners: [me.user.id] } });
    await admin.context().close();
    await page.goto(`/groups/${group.id}`);
    await expect(page.getByRole('heading', { name: 'Platform team' })).toBeVisible();
    await picture(page, 'group-owner');
    await page.context().close();
  });

  test('SpaceAccessView: who has access', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/access');
    await expect(page.getByRole('tab', { name: 'People' })).toBeVisible();
    // People and groups with a role each, not a matrix of ticks (spec 043).
    await expect(page.locator('.member-row').first()).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Give access' })).toBeVisible();
    await picture(page, 'space-access');
    await page.getByRole('tab', { name: 'People' }).click();
    await expect(page.getByRole('link', { name: /CSV/ })).toBeVisible();
    await expect(page.getByRole('table')).toBeVisible();
    await picture(page, 'space-access-people');
    await page.context().close();
  });

  test('SpacePeopleView: people in a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/people');
    await expect(page.getByRole('heading', { name: 'People', level: 1 })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Everyone' })).toBeVisible();
    await expect(page.locator('.member-row').first()).toBeVisible();
    await picture(page, 'space-people');
    await page.context().close();
  });

  test('SpaceAuditView: the audit of a space', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/spaces/HANDBOOK/audit');
    await expect(page.getByRole('heading', { name: 'Audit of this space' })).toBeVisible();
    await expect(page.getByRole('table')).toBeVisible();
    await picture(page, 'space-audit', [page.locator('table')]);
    await page.context().close();
  });

  test('ForgotView: a link for a new password', async ({ page }) => {
    await page.goto('/forgot');
    await expect(page.getByRole('heading', { name: 'Forgot your password' })).toBeVisible();
    await picture(page, 'forgot');
  });

  test('ResetView: a new password', async ({ page }) => {
    await page.goto('/reset?token=x');
    await expect(page.getByRole('heading', { name: 'Choose a new password' })).toBeVisible();
    await picture(page, 'reset');
  });

  test('InvitationView: accepting an invitation', async ({ browser, page }) => {
    const admin = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
    await login(admin, ADMIN.email, ADMIN.password);
    const made = await admin.request.post('/api/v1/admin/invitations', {
      data: { email: 'new.colleague@example.com', type: 'member', groups: [] },
    });
    expect(made.status(), await made.text()).toBe(201);
    const { link } = (await made.json()) as { link: string };
    await admin.context().close();
    await page.goto(link.replace(/^https?:\/\/[^/]+/, ''));
    await expect(page.getByText('new.colleague@example.com')).toBeVisible();
    await picture(page, 'invitation');
  });

  test('SignupView: signing up', async ({ page }) => {
    await page.goto('/signup');
    await expect(page.getByRole('heading', { name: 'Sign up' })).toBeVisible();
    await picture(page, 'signup');
  });

  test('the console summary, and what asks for attention', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    await page.goto('/admin/summary');
    await expect(page.getByText('You have no passkey')).toBeVisible();
    await expect(page.getByText('No identity provider is set up')).toBeVisible();
    await picture(page, 'admin-summary', [page.locator('.tiles')]);
  });

  test('the rules and the look of an instance', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    await page.goto('/admin/look');
    await expect(page.getByRole('heading', { name: 'Look of this instance' })).toBeVisible();
    await picture(page, 'admin-look');
    await page.goto('/admin/rules');
    await expect(page.getByRole('heading', { name: 'Features' })).toBeVisible();
    await picture(page, 'admin-rules');
  });

  test('the roles of an instance, and who holds them in a space', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    await page.goto('/admin/permissions');
    await expect(page.getByRole('heading', { name: 'Roles for a space' })).toBeVisible();
    await picture(page, 'admin-roles');
    // The editor shows the permissions in groups, each with what it allows.
    await page.getByRole('button', { name: 'New role' }).first().click();
    await expect(page.getByRole('group', { name: 'People and access' })).toBeVisible();
    await picture(page, 'role-editor');
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
    await page.goto('/spaces/HANDBOOK/settings?section=roles');
    await expect(page.getByRole('heading', { name: 'People and roles', level: 1 })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Give access' })).toBeVisible();
    await expect(page.locator('.member-row').first()).toBeVisible();
    await picture(page, 'space-roles');
    // A combination without a name opens to show its permissions, and an administrator can name it.
    await page
      .locator('.held-role summary', { hasText: /^Custom combination \d+ \(/ })
      .first()
      .click();
    await picture(page, 'space-roles-open');
    await page.getByRole('button', { name: 'Name this role' }).first().click();
    await expect(page.getByRole('dialog', { name: 'Name this role' })).toBeVisible();
    await picture(page, 'name-role');
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();
  });

  test('a person asked for a stronger sign-in, and a maintenance note', async ({ browser }) => {
    const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
    const page = await context.newPage();
    await login(page, READER.email, PASSWORD);
    await page.route('**/api/v1/me', async (route) => {
      const res = await route.fetch();
      const me = (await res.json()) as { sign_in: { weak: boolean; prompt: boolean } };
      me.sign_in.prompt = true;
      await route.fulfill({ response: res, json: me });
    });
    await page.route('**/api/v1/appearance', async (route) => {
      const res = await route.fetch();
      const look = (await res.json()) as { maintenance: { on: boolean; message: string } };
      look.maintenance = { on: true, message: "Back at two o'clock." };
      await route.fulfill({ response: res, json: look });
    });
    await page.goto('/people');
    await expect(page.getByTestId('maintenance-note')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Later' })).toBeVisible();
    await picture(page, 'site-notes');
    await context.close();
  });

  test('a newer version is running, in light, and dark on a phone', async ({ browser }) => {
    for (const scheme of ['light', 'dark'] as const) {
      const page = await newReaderPage(browser, scheme);
      if (scheme === 'dark') await page.setViewportSize({ width: 390, height: 844 });
      let newer = false;
      await page.route('**/api/v1/appearance', async (route) => {
        const res = await route.fetch();
        const look = (await res.json()) as { build: string };
        if (newer) look.build = `${look.build}-newer`;
        await route.fulfill({ response: res, json: look });
      });
      await page.goto('/people');
      await expect(page.locator('main')).toBeVisible();
      newer = true;
      await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
      await expect(page.getByRole('status', { name: 'New version' })).toBeVisible();
      await picture(page, `new-version-${scheme}`);
      await page.context().close();
    }
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
    await expect(page.locator('#main').getByRole('link', { name: 'Engineering Handbook' })).toBeVisible();
    // "now" or "2 minutes ago" depends on how long the run took, and its width moves the table.
    await fixText(page.locator('#main time'), 'now');
    await picture(page, 'directory', [page.locator('time')]);
    await page.context().close();
  });

  test('RecentView: recent pages', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/recent');
    await expect(page.getByRole('link', { name: 'Your first week' })).toBeVisible();
    // The width of a relative time moves the space name beside it.
    await fixText(page.locator('#main time'), 'now');
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
    // Pages changed in the same moment can swap places, so the names are masked as well.
    await picture(page, 'overview', [page.locator('.page-list')]);
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

  test('the filters of the search box', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto('/');
    await page.getByRole('combobox', { name: 'Search' }).click();
    const filters = page.getByRole('group', { name: 'Filters' });
    await filters.getByRole('button', { name: 'Type' }).click();
    await filters.getByRole('button', { name: 'Blog posts' }).click();
    await filters.getByRole('button', { name: 'Spaces' }).click();
    await filters.getByRole('button', { name: 'Engineering Handbook' }).click();
    await expect(filters.getByRole('button', { name: 'Engineering Handbook' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    await picture(page, 'quick-search-filters');
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
      await page.goto('/');
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
      // Pages from earlier tests are watched by a background job, which a slow machine finishes later,
      // so the picture shows only what this test watches.
      await page.getByPlaceholder('Filter by name').fill('Team agreements');
      await expect(page.getByRole('button', { name: 'Stop watching' })).toHaveCount(1);
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

    test.describe('insight, rules and webhooks', () => {
      let reportedPage = '';
      let ruleId = '';
      let spaceKey = '';

      test.beforeAll(async ({ browser }) => {
        const setup = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
        await login(setup, ADMIN.email, ADMIN.password);
        const on = await setup.request.put('/api/v1/admin/analytics', {
          data: { enabled: true, mode: 'names', retention_days: 365 },
        });
        expect(on.ok(), await on.text()).toBeTruthy();
        const space = (await (await setup.request.get(`/api/v1/spaces/${spaceId}`)).json()) as {
          key: string;
        };
        spaceKey = space.key;
        reportedPage = await publishedPage(setup, spaceId, 'Coffee rota', 'x');
        const reader = await newReaderPage(browser, 'light');
        const viewed = await reader.request.post(`/api/v1/pages/${reportedPage}/views`, { data: {} });
        expect(viewed.ok(), await viewed.text()).toBeTruthy();
        await reader.context().close();
        const made = await setup.request.post(`/api/v1/spaces/${spaceId}/rules`, {
          data: {
            name: 'Label new pages',
            enabled: true,
            definition: {
              trigger: { kind: 'page.published' },
              actions: [{ type: 'add_label', label: 'new' }],
            },
          },
        });
        expect(made.status(), await made.text()).toBe(201);
        ruleId = ((await made.json()) as { id: string }).id;
        await setup.context().close();
      });

      test('PageInsightsView: what a page did', async ({ browser }) => {
        const page = await newReaderPage(browser, 'light');
        await page.goto(`/pages/${reportedPage}/insights`);
        await expect(page.getByRole('heading', { name: 'Page insights' })).toBeVisible();
        await expect(page.getByRole('heading', { name: 'Readers', exact: true })).toBeVisible();
        await picture(page, 'page-insights', [page.locator('.chart'), page.locator('table')]);
        await checkAccessibility(page);
        await page.context().close();
      });

      test('SpaceInsightsView: what a space did', async ({ browser }) => {
        const page = await newReaderPage(browser, 'light');
        await page.goto(`/spaces/${spaceKey}/insights`);
        await expect(page.getByRole('heading', { name: 'Most viewed' })).toBeVisible();
        await expect(page.getByRole('columnheader', { name: 'Views' }).first()).toBeVisible();
        await picture(page, 'space-insights', [page.locator('table')]);
        await checkAccessibility(page);
        await page.context().close();
      });

      test('RulesView: the rules of a space', async ({ browser }) => {
        const page = await newReaderPage(browser, 'light');
        await page.goto(`/spaces/${spaceKey}/settings/automation`);
        await expect(page.getByRole('link', { name: 'Label new pages' })).toBeVisible();
        await picture(page, 'rules', [page.locator('table')]);
        await checkAccessibility(page);
        await page.context().close();
      });

      test('RuleView: one rule', async ({ browser }) => {
        const page = await newReaderPage(browser, 'light');
        await page.goto(`/rules/${ruleId}`);
        await expect(page.getByRole('heading', { name: 'When', exact: true })).toBeVisible();
        await picture(page, 'rule', [page.locator('time')]);
        await checkAccessibility(page);
        await page.context().close();
      });

      test('WebhooksView: the webhooks of a space', async ({ browser }) => {
        const page = await newReaderPage(browser, 'light');
        await page.goto(`/spaces/${spaceKey}/settings/webhooks`);
        await expect(page.getByRole('heading', { name: 'New webhook' })).toBeVisible();
        await picture(page, 'webhooks');
        await checkAccessibility(page);
        await page.context().close();
      });

      test('administration: analytics and outgoing calls', async ({ browser }) => {
        const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
        const page = await context.newPage();
        await login(page, ADMIN.email, ADMIN.password);
        await page.goto('/admin/analytics');
        await expect(page.getByLabel('What is kept')).toBeVisible();
        await picture(page, 'admin-analytics');
        await checkAccessibility(page);
        await page.goto('/admin/outgoing');
        await expect(page.getByLabel('Host')).toBeVisible();
        await picture(page, 'admin-outgoing');
        await checkAccessibility(page);
        await page.goto('/admin/insights');
        await expect(page.getByRole('heading', { name: 'What people search for' })).toBeVisible();
        await picture(page, 'admin-insights');
        await checkAccessibility(page);
        const off = await page.request.put('/api/v1/admin/analytics', {
          data: { enabled: false, mode: 'counts', retention_days: 365 },
        });
        expect(off.ok(), await off.text()).toBeTruthy();
        await context.close();
      });
    });
  });

  test('ExportView: a finished export, and the dialog', async ({ browser }) => {
    const page = await newReaderPage(browser, 'light');
    await page.goto(`/pages/${pageId}`);
    await page.getByRole('button', { name: 'More actions' }).click();
    await page.getByRole('menuitem', { name: 'Export' }).click();
    await expect(page.getByRole('dialog', { name: 'Export' })).toBeVisible();
    await picture(page, 'export-dialog');
    const run = await page.request.post('/api/v1/exports', {
      data: { space_id: spaceId, format: 'text', page_ids: [pageId] },
    });
    const { id } = (await run.json()) as { id: string };
    await page.goto(`/exports/${id}`);
    await expect(page.getByRole('link', { name: 'Download' })).toBeVisible();
    // The notification of this very export can come in at any moment.
    await picture(page, 'export-view', [page.locator('.bell-number')]);
    await page.context().close();
  });
});

// Last in this file, so the long names below show in no other picture.
test.describe('the settings area at the widths of a laptop', () => {
  test.describe.configure({ mode: 'serial' });
  // An address with no hyphen, as many are, cannot wrap, so it makes the table of people wide.
  const LONG_EMAIL = 'johannavandersteenhovendegroot@facilitiesandprocurementexampleholding.com';
  const LAPTOP = { width: 1150, height: 900 };
  // The clock moves the width of a login time, and with it the columns.
  const LAST_LOGIN = '9 Nov 2026, 12:00';
  // A wide screen as well: there a centred area leaves an empty band beside the sidebar.
  const WIDTHS = [1024, 1150, 1280, 1600];
  const SCREENS = [
    '/admin/users',
    '/admin/spaces',
    '/admin/groups',
    '/admin/audit',
    '/admin/system',
    '/spaces/HANDBOOK/settings?section=people',
  ];

  test.beforeAll(async ({ browser }) => {
    const { api, close } = await adminRequest(browser);
    const made = [
      await api.post('/api/v1/admin/users', {
        data: {
          email: LONG_EMAIL,
          display_name: 'Johanna van der Steenhoven-de Groot',
          type: 'member',
          password: PASSWORD,
          global_permissions: null,
        },
      }),
      await api.post('/api/v1/spaces', {
        data: {
          key: 'BENELUX',
          name: 'Customer onboarding and contract administration for the Benelux region',
        },
      }),
      await api.post('/api/v1/admin/groups', {
        data: { name: 'Facilities, procurement and workplace services' },
      }),
    ];
    for (const res of made) expect(res.status(), await res.text()).toBe(201);
    await close();
  });

  async function newAdminPage(browser: Browser, colorScheme: 'light' | 'dark'): Promise<Page> {
    const context = await browser.newContext({ baseURL: 'http://localhost:8080', colorScheme });
    const page = await context.newPage();
    await login(page, ADMIN.email, ADMIN.password);
    return page;
  }

  test('every screen starts beside the sidebar and stays inside the window', async ({ browser }) => {
    const page = await newAdminPage(browser, 'light');
    for (const width of WIDTHS) {
      await page.setViewportSize({ width, height: LAPTOP.height });
      for (const address of SCREENS) {
        await page.goto(address);
        await expect(page.locator('.settings-area > h1')).toBeVisible();
        const place = await page.evaluate(() => {
          const main = document.querySelector('main.content') as HTMLElement;
          const area = (document.querySelector('.settings-area') as HTMLElement).getBoundingClientRect();
          const sidebar = (document.querySelector('.sidebar-frame') as HTMLElement).getBoundingClientRect();
          // What sits in a box that scrolls may be wider than the room: that box is the point.
          const loose = [...document.querySelectorAll('.settings-area *')].filter(
            (el) => !el.closest('.table-box') && el.getBoundingClientRect().width > 0,
          );
          const right = Math.max(...loose.map((el) => el.getBoundingClientRect().right));
          return {
            band: Math.round(
              area.left - sidebar.right - parseFloat(getComputedStyle(main).paddingInlineStart),
            ),
            past: Math.round(right - main.getBoundingClientRect().right),
            pageScrolls: document.documentElement.scrollWidth > document.documentElement.clientWidth,
          };
        });
        expect(place, `${address} at ${width} px`).toEqual({
          band: 0,
          past: expect.any(Number),
          pageScrolls: false,
        });
        expect(place.past, `${address} at ${width} px`).toBeLessThanOrEqual(0);
      }
    }
    await page.context().close();
  });

  for (const colorScheme of ['light', 'dark'] as const) {
    test(`AdminView: the people of an instance at 1150 px, ${colorScheme}`, async ({ browser }) => {
      const page = await newAdminPage(browser, colorScheme);
      await page.setViewportSize(LAPTOP);
      await page.goto('/admin/users');
      // The address wraps, so every column and every filter is in view.
      await expect(page.getByRole('cell', { name: LONG_EMAIL })).toBeInViewport({ ratio: 1 });
      await expect(page.getByRole('columnheader', { name: 'Last login' })).toBeInViewport({ ratio: 1 });
      await expect(page.getByRole('combobox', { name: 'Source', exact: true })).toBeInViewport({ ratio: 1 });
      await fixText(page.locator('tbody td:nth-child(7)'), LAST_LOGIN);
      await picture(page, `admin-users-1150-${colorScheme}`, [page.locator('tbody td:nth-child(7)')]);
      await checkAccessibility(page);
      await page.context().close();
    });
  }

  test('AdminView: a table too wide for a narrow screen scrolls in its own box', async ({ browser }) => {
    const page = await newAdminPage(browser, 'light');
    await page.setViewportSize(NARROW_SCREEN);
    await page.goto('/admin/users');
    await expect(page.getByRole('cell', { name: LONG_EMAIL })).toBeAttached();
    const box = page.getByRole('region', { name: 'Table, scrolls sideways' });
    await expect(box).toBeVisible();
    await box.scrollIntoViewIfNeeded();
    await fixText(page.locator('tbody td:nth-child(7)'), LAST_LOGIN);
    await picture(page, 'admin-users-narrow', [page.locator('tbody td:nth-child(7)')]);
    await checkAccessibility(page);
    // The keyboard reaches the box and scrolls it; the page itself stays put.
    await box.focus();
    await expect(box).toBeFocused();
    await page.keyboard.press('ArrowRight');
    await expect.poll(() => box.evaluate((el) => el.scrollLeft)).toBeGreaterThan(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(false);
    await page.context().close();
  });

  test('AdminView: the spaces of an instance at 1150 px', async ({ browser }) => {
    const page = await newAdminPage(browser, 'light');
    await page.setViewportSize(LAPTOP);
    await page.goto('/admin/spaces');
    await expect(page.getByRole('link', { name: /Benelux region/ })).toBeVisible();
    await fixText(page.locator('tbody td:nth-child(6)'), 'now');
    await picture(page, 'admin-spaces-1150', [page.locator('tbody td:nth-child(6)')]);
    await checkAccessibility(page);
    await page.context().close();
  });
});
