// SPDX-License-Identifier: Apache-2.0
// Working with others (spec 010): two people, and the keyboard first.
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  EDITOR,
  editorSettled,
  expect,
  grant,
  login,
  newSpace,
  paragraph,
  publishedPage,
  test,
  unique,
  VIEWER,
} from './helpers';

const WRITER = [...EDITOR, 'comment.add', 'comment.delete_own'];
const COMMENTER = [...VIEWER, 'comment.add', 'comment.delete_own'];
const PAGE_TEXT = 'The quick brown fox jumps over the lazy dog.';
const DELIVERY_MS = 20_000;
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
  'base64',
);

interface Listed {
  type: string;
  count: number;
  page_id: string;
}

interface Team {
  name: string;
  email: string;
  id: string;
  spaceId: string;
  spaceKey: string;
  pageId: string;
  other: Page;
}

async function signedIn(browser: Browser, email: string): Promise<Page> {
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const page = await context.newPage();
  await login(page, email);
  return page;
}

/** The admin on `page`, a colleague on `other`, and a page in a space of their own. */
async function team(page: Page, browser: Browser, permissions: string[] | null = WRITER): Promise<Team> {
  await login(page, ADMIN.email, ADMIN.password);
  const name = unique('colleague');
  const email = `${name}@example.com`;
  const id = await createUser(browser, email);
  const space = await newSpace(page, unique('Team '));
  if (permissions) await grant(page, space.id, { type: 'user', id }, permissions);
  const pageId = await publishedPage(page, space.id, 'Working agreement', PAGE_TEXT);
  const other = await signedIn(browser, email);
  return { name, email, id, spaceId: space.id, spaceKey: space.key, pageId, other };
}

async function notificationsOf(page: Page, type: string): Promise<Listed[]> {
  const res = await page.request.get('/api/v1/me/notifications?limit=50');
  expect(res.ok(), await res.text()).toBeTruthy();
  const { notifications } = (await res.json()) as { notifications: Listed[] };
  return notifications.filter((n) => n.type === type);
}

/**
 * Notifications leave through the outbox, so they arrive a moment later. The page counts:
 * the administrator has older ones of the same type, from other tests.
 */
async function waitForNotification(page: Page, type: string, pageId: string) {
  const arrived = async () => (await notificationsOf(page, type)).filter((n) => n.page_id === pageId).length;
  await expect.poll(arrived, { timeout: DELIVERY_MS }).toBeGreaterThan(0);
}

async function comment(page: Page, pageId: string, text: string) {
  const res = await page.request.post(`/api/v1/pages/${pageId}/comments`, {
    data: { body: paragraph(text) },
  });
  expect(res.status(), await res.text()).toBe(201);
  return ((await res.json()) as { id: string }).id;
}

async function publish(page: Page, pageId: string, text: string, version: number) {
  const res = await page.request.post(`/api/v1/pages/${pageId}/publish`, {
    data: { title: 'Working agreement', document: paragraph(text), base_version: version },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** Selects part of the first paragraph of the page, the way a mouse would. */
async function selectWords(page: Page, from: number, to: number) {
  await page.evaluate(
    ([start, end]) => {
      const text = document.querySelector('.page .prose p')?.firstChild;
      if (!text) throw new Error('no paragraph');
      const range = document.createRange();
      range.setStart(text, start ?? 0);
      range.setEnd(text, end ?? 0);
      const selection = window.getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
    },
    [from, to],
  );
}

async function openPage(page: Page, pageId: string) {
  await page.goto(`/pages/${pageId}`);
  await expect(page.getByRole('heading', { name: 'Working agreement', level: 1 })).toBeVisible();
}

test('a comment reaches the author, who answers it by keyboard from the bell', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  await openPage(other, pageId);
  await expect(other.locator('#comment-box')).toBeVisible();
  await other.keyboard.press('m');
  await expect(other.locator('#comment-box')).toBeFocused();
  await other.keyboard.type('Can we move this to Friday?');
  await other.keyboard.press('Control+Enter');
  await expect(other.getByRole('article', { name: `Comment by ${name}` })).toContainText('Friday');

  await waitForNotification(page, 'comment', pageId);
  await page.goto('/');
  await expect(page.getByRole('button', { name: /^Notifications, \d+ unread$/ })).toBeVisible();
  await page.keyboard.press('g');
  await page.keyboard.press('n');
  const panel = page.getByRole('region', { name: 'Notifications' });
  await expect(panel).toBeVisible();
  await checkAccessibility(page);
  const row = panel.getByRole('link', { name: new RegExp(`${name} commented on Working agreement`) });
  await row.focus();
  await page.keyboard.press('Enter');
  const thread = page.getByRole('article', { name: `Comment by ${name}` });
  await expect(thread).toBeInViewport();

  await thread.getByRole('button', { name: 'Reply' }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('textbox', { name: 'Write a reply' })).toBeFocused();
  await page.keyboard.type('Friday works.');
  await page.keyboard.press('Control+Enter');
  await expect(page.getByRole('article', { name: 'Comment by Admin' })).toContainText('Friday works.');
  await waitForNotification(other, 'reply', pageId);
  await other.context().close();
});

test('a notification about the page you are reading still goes to the comment', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  await comment(other, pageId, 'Look at this.');
  await waitForNotification(page, 'comment', pageId);
  await page.setViewportSize({ width: 1280, height: 420 });
  await openPage(page, pageId);
  await page.getByRole('button', { name: /^Notifications, \d+ unread$/ }).click();
  const panel = page.getByRole('region', { name: 'Notifications' });
  await panel.getByRole('link', { name: new RegExp(`${name} commented`) }).click();
  await expect(page.getByRole('article', { name: `Comment by ${name}` })).toBeInViewport();
  await other.context().close();
});

test('a mention in the editor reaches the person when the page is published', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  await page.goto(`/pages/${pageId}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.press('End');
  await page.keyboard.type(` @${name.slice(0, 12)}`);
  await expect(
    page.getByRole('listbox', { name: 'People' }).getByRole('option', { name: new RegExp(name) }),
  ).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page.locator('.prose.editing .mention')).toHaveText(`@${name}`);
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Working agreement', level: 1 })).toBeVisible();
  await waitForNotification(other, 'mention', pageId);
  await other.context().close();
});

test('a comment can hold an image, and an image in a page can get a comment', async ({ page, browser }) => {
  const { other, pageId } = await team(page, browser);
  await openPage(page, pageId);
  await page.locator('.comment-box input[type="file"]').setInputFiles({
    name: 'dot.png',
    mimeType: 'image/png',
    buffer: PNG,
  });
  await expect(page.locator('.comment-input img')).toHaveCount(1);
  await page.getByRole('button', { name: 'Post', exact: true }).click();
  await expect(page.locator('.comments .prose img')).toHaveCount(1);

  const uploaded = await page.request.post(`/api/v1/pages/${pageId}/attachments?filename=dot.png`, {
    data: PNG,
    headers: { 'Content-Type': 'application/octet-stream' },
  });
  expect(uploaded.status(), await uploaded.text()).toBe(201);
  const { id } = (await uploaded.json()) as { id: string };
  const published = await page.request.post(`/api/v1/pages/${pageId}/publish`, {
    data: {
      title: 'Working agreement',
      base_version: 1,
      document: { type: 'doc', content: [{ type: 'image', attrs: { attachment_id: id, alt: 'A dot' } }] },
    },
  });
  expect(published.ok(), await published.text()).toBeTruthy();

  await openPage(other, pageId);
  await other.getByRole('button', { name: 'dot.png' }).click();
  await expect(other.getByRole('dialog', { name: 'dot.png' })).toBeVisible();
  await other.keyboard.press('Escape');
  await other.getByRole('button', { name: 'A dot' }).click();
  await other.getByRole('dialog', { name: 'A dot' }).getByRole('button', { name: 'Comment' }).click();
  const dialog = other.getByRole('dialog', { name: 'Comment on this image' });
  await expect(dialog.getByRole('textbox', { name: 'Write a comment' })).toBeFocused();
  await other.keyboard.type('Why a dot?');
  await other.keyboard.press('Control+Enter');
  const panel = other.getByRole('complementary', { name: 'Comments' });
  await expect(panel).toContainText('Why a dot?');
  await panel.getByRole('button', { name: 'Back to all comments' }).click();
  await expect(panel.getByText('On an image')).toBeVisible();
  await other.context().close();
});

test('logging out leaves nothing of a person in the browser', async ({ page, browser }) => {
  const { pageId, email, other } = await team(page, browser);
  await other.context().close();
  const personal = () =>
    page.evaluate(() => Object.keys(localStorage).filter((key) => key.startsWith('diffluence.person.')));
  await openPage(page, pageId);
  await page.keyboard.press('/');
  await page.keyboard.type('kumquat plans');
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/search\?/);
  await openPage(page, pageId);
  await page.keyboard.press('m');
  await expect(page.locator('#comment-box')).toBeFocused();
  await page.keyboard.type('A half-written thought');
  await expect.poll(async () => (await personal()).some((key) => key.includes('.draft.'))).toBeTruthy();
  expect((await personal()).some((key) => key.endsWith('.searches'))).toBeTruthy();

  await page.getByRole('button', { name: 'Account' }).click();
  await page.getByRole('menuitem', { name: 'Log out' }).click();
  await expect(page).toHaveURL(/\/login$/);
  expect(await personal()).toEqual([]);

  await login(page, email);
  await page.keyboard.press('/');
  await expect(page.getByRole('option', { name: 'kumquat plans' })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await openPage(page, pageId);
  await expect(page.locator('#comment-box')).toHaveText('');
});

test('a comment on chosen words stays on them until it is resolved', async ({ page, browser }) => {
  const { other, pageId } = await team(page, browser);
  await openPage(other, pageId);
  await selectWords(other, 4, 15);
  await expect(other.locator('.selection-bar').getByRole('button', { name: 'Comment' })).toBeVisible();
  await other.keyboard.press('m');
  const dialog = other.getByRole('dialog', { name: 'Comment on this text' });
  await expect(dialog).toContainText('quick brown');
  await expect(dialog.getByRole('textbox', { name: 'Write a comment' })).toBeFocused();
  await other.keyboard.type('Which fox?');
  await other.keyboard.press('Control+Enter');
  await expect(dialog).toHaveCount(0);
  const panel = other.getByRole('complementary', { name: 'Comments' });
  await expect(panel).toContainText('Which fox?');

  const mark = other.locator('.comment-mark');
  await expect(mark).toHaveText('quick brown');
  await expect(mark).toHaveAccessibleName(/Which fox|Comment by/);
  await checkAccessibility(other);
  await other.keyboard.press('Escape');
  await expect(panel).toHaveCount(0);
  await mark.focus();
  await other.keyboard.press('Enter');
  await expect(panel).toContainText('Which fox?');

  // The words move: the mark follows them.
  await publish(page, pageId, `Before all that. ${PAGE_TEXT}`, 1);
  await other.reload();
  await expect(other.locator('.comment-mark')).toHaveText('quick brown');

  // The words go: the thread stays, and says so.
  await publish(page, pageId, 'Nothing of that is left.', 2);
  await other.reload();
  await expect(other.getByRole('heading', { name: 'Working agreement', level: 1 })).toBeVisible();
  await expect(other.locator('.comment-mark')).toHaveCount(0);
  await other.getByRole('button', { name: '1 comment', exact: true }).click();
  await expect(panel.getByText('No longer in the page')).toBeVisible();

  await panel.getByRole('button', { name: /Which fox/ }).click();
  await panel.getByRole('button', { name: 'Resolve', exact: true }).click();
  await panel.getByRole('button', { name: 'Back to all comments' }).click();
  await expect(panel.getByText('Nothing here.')).toBeVisible();
  await panel.getByRole('tab', { name: 'Open' }).focus();
  await other.keyboard.press('ArrowRight');
  await expect(panel.getByRole('tab', { name: 'Resolved' })).toBeFocused();
  await expect(panel).toContainText('Which fox?');
  await other.context().close();
});

test('your own comment can be changed, removed and brought back', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  await comment(other, pageId, 'Frist thought');
  await openPage(other, pageId);
  const item = other.getByRole('article', { name: `Comment by ${name}` });
  await item.getByRole('button', { name: 'Edit' }).click();
  await other.getByRole('textbox', { name: 'Edit your comment' }).focus();
  await other.keyboard.press('Control+a');
  await other.keyboard.type('First thought');
  await other.keyboard.press('Control+Enter');
  await expect(item).toContainText('First thought');
  await expect(item).toContainText('edited');

  await item.getByRole('button', { name: 'Delete' }).click();
  await other
    .getByRole('dialog', { name: 'Delete this comment?' })
    .getByRole('button', { name: 'Delete' })
    .click();
  await expect(item).toHaveCount(0);
  await other.getByRole('status').getByRole('button', { name: 'Undo' }).click();
  await expect(item).toContainText('First thought');

  // Someone else's comment offers neither.
  await comment(page, pageId, 'From the administrator');
  await openPage(other, pageId);
  const theirs = other.getByRole('article', { name: 'Comment by Admin' });
  await expect(theirs).toContainText('From the administrator');
  await expect(theirs.getByRole('button', { name: 'Edit' })).toHaveCount(0);
  await expect(theirs.getByRole('button', { name: 'Delete' })).toHaveCount(0);
  await other.context().close();
});

test('closing comments stops new ones, and a space can turn them off', async ({ page, browser }) => {
  const { other, pageId, spaceId } = await team(page, browser, COMMENTER);
  await openPage(page, pageId);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Close comments' }).click();
  await expect(page.getByText('Comments are closed on this page.')).toBeVisible();

  await openPage(other, pageId);
  await expect(other.getByText('Comments are closed on this page.')).toBeVisible();
  await expect(other.locator('#comment-box')).toHaveCount(0);
  const refused = await other.request.post(`/api/v1/pages/${pageId}/comments`, {
    data: { body: paragraph('Too late') },
  });
  expect(refused.ok()).toBeFalsy();

  const off = await page.request.put(`/api/v1/spaces/${spaceId}/features`, {
    data: { comments: false, reactions: false, access_requests: false },
  });
  expect(off.ok(), await off.text()).toBeTruthy();
  await openPage(other, pageId);
  await expect(other.getByRole('region', { name: 'Comments' })).toHaveCount(0);
  await expect(other.getByRole('group', { name: 'Reactions' })).toHaveCount(0);
  await other.context().close();
});

test('a reaction counts once per person and can be taken back', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  await openPage(other, pageId);
  const row = other.getByRole('group', { name: 'Reactions' }).first();
  await row.getByRole('button', { name: 'Add a reaction' }).click();
  await row.getByRole('button', { name: '👍' }).click();
  const counter = row.getByRole('button', { name: /React with 👍, 1 so far/ });
  await expect(counter).toHaveAttribute('aria-pressed', 'true');

  await openPage(page, pageId);
  const seen = page.getByRole('group', { name: 'Reactions' }).first();
  const count = seen.getByRole('button', { name: /React with 👍, 1 so far/ });
  await expect(count).toHaveAttribute('aria-pressed', 'false');
  await count.focus();
  await expect(page.getByRole('tooltip')).toHaveText(name);
  await count.click();
  await expect(seen.getByRole('button', { name: /React with 👍, 2 so far/ })).toBeVisible();

  await counter.click();
  await other.reload();
  await expect(other.getByRole('group', { name: 'Reactions' }).first()).toContainText('1');
  await other.context().close();
});

test('a mention tells the person, and warns about who cannot see the page', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  const outsider = unique('outsider');
  await createUser(browser, `${outsider}@example.com`);
  await openPage(page, pageId);
  await page.keyboard.press('m');
  await expect(page.locator('#comment-box')).toBeFocused();
  await page.keyboard.type(`Please look @${name.slice(0, 12)}`);
  const list = page.getByRole('listbox', { name: 'People' });
  await expect(list.getByRole('option', { name: new RegExp(name) })).toBeVisible();
  await page.keyboard.press('Enter');
  await page.keyboard.type(`and @${outsider.slice(0, 11)}`);
  await expect(list.getByRole('option', { name: /Cannot see this page/ })).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('status').filter({ hasText: 'cannot see this page' })).toBeVisible();
  await page.keyboard.press('Control+Enter');
  await expect(page.locator('.comments .mention').first()).toHaveText(`@${name}`);

  await waitForNotification(other, 'mention', pageId);
  await other.goto('/notifications');
  await expect(other.getByRole('link', { name: /mentioned you on Working agreement/ })).toBeVisible();
  await checkAccessibility(other);
  await other.context().close();
});

test('a task with a person lands on their list, and ticking it counts on the page', async ({
  page,
  browser,
}) => {
  const { other, pageId, id, name } = await team(page, browser);
  const assigned = await page.request.post(`/api/v1/pages/${pageId}/publish`, {
    data: {
      title: 'Working agreement',
      base_version: 1,
      document: {
        type: 'doc',
        content: [
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
                      { type: 'text', text: 'Book the room ' },
                      { type: 'mention', attrs: { id, kind: 'user', label: name } },
                      { type: 'text', text: ' ' },
                      { type: 'date', attrs: { value: '2020-01-02' } },
                    ],
                  },
                ],
              },
            ],
          },
        ],
      },
    },
  });
  expect(assigned.ok(), await assigned.text()).toBeTruthy();

  await waitForNotification(other, 'task_assigned', pageId);
  await other.goto('/tasks');
  await expect(other.getByRole('heading', { name: 'Overdue' })).toBeVisible();
  const tick = other.getByRole('checkbox', { name: /Done: Book the room/ });
  await checkAccessibility(other);
  await tick.focus();
  await other.keyboard.press('Space');
  await expect(other.getByText('No tasks here.')).toBeVisible();

  await openPage(other, pageId);
  await expect(other.locator('.page .prose input[type="checkbox"]')).toBeChecked();
  await other.goto('/tasks');
  await other.getByLabel('Show').selectOption('done');
  await expect(other.getByRole('checkbox', { name: /Done: Book the room/ })).toBeChecked();
  await other.context().close();
});

test('someone reading a page cannot tick a task on it', async ({ page, browser }) => {
  const { other, pageId } = await team(page, browser, VIEWER);
  const published = await page.request.post(`/api/v1/pages/${pageId}/publish`, {
    data: {
      title: 'Working agreement',
      base_version: 1,
      document: {
        type: 'doc',
        content: [
          {
            type: 'taskList',
            content: [
              {
                type: 'taskItem',
                attrs: { checked: false },
                content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Book the room' }] }],
              },
            ],
          },
        ],
      },
    },
  });
  expect(published.ok(), await published.text()).toBeTruthy();
  await openPage(other, pageId);
  await expect(other.locator('.page .prose input[type="checkbox"]')).toBeDisabled();
  await other.context().close();
});

test('a watcher hears about a change, unless it is published without notifying', async ({
  page,
  browser,
}) => {
  const { other, pageId } = await team(page, browser);
  await openPage(other, pageId);
  await other.keyboard.press('w');
  await expect(other.getByRole('status').filter({ hasText: 'You now watch this.' })).toBeVisible();

  await page.goto(`/pages/${pageId}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.press('End');
  await page.keyboard.type(' Quietly.');
  await page.getByRole('button', { name: 'Publish options' }).click();
  const options = page.getByRole('dialog', { name: 'Publish options' });
  await options.getByLabel('Notify watchers').uncheck();
  await options.getByRole('button', { name: 'Done' }).click();
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Working agreement', level: 1 })).toBeVisible();

  await publish(page, pageId, 'Out loud.', 2);
  await waitForNotification(other, 'page_changed', pageId);
  // One change arrived, and the quiet one was left out.
  expect((await notificationsOf(other, 'page_changed')).map((n) => n.count)).toEqual([1]);

  await other.goto('/me/watching');
  await expect(other.getByRole('link', { name: 'Working agreement' })).toBeVisible();
  await checkAccessibility(other);
  await other.getByRole('button', { name: 'Stop watching' }).click();
  await expect(other.getByText('You watch nothing here.')).toBeVisible();
  await other.context().close();
});

test('sharing tells people but gives nobody access', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser);
  const outsider = unique('outsider');
  await createUser(browser, `${outsider}@example.com`);
  await openPage(page, pageId);
  await page.getByRole('button', { name: 'Share' }).click();
  const dialog = page.getByRole('dialog', { name: 'Share this page' });
  await expect(dialog.getByRole('heading', { name: 'Who can see this page' })).toBeVisible();
  await checkAccessibility(page);

  await dialog.getByLabel('Who to tell').fill(name);
  await dialog.getByRole('button', { name: new RegExp(name) }).click();
  await dialog.getByLabel('Who to tell').fill(outsider);
  await dialog.getByRole('button', { name: new RegExp(outsider) }).click();
  await expect(dialog.getByText('Cannot see this page')).toBeVisible();
  await dialog.getByLabel('Message').fill('Have a look at this.');
  await dialog.getByRole('button', { name: 'Send' }).click();
  await expect(dialog.getByRole('status')).toContainText('1 person was told.');
  await expect(dialog.getByRole('status')).toContainText(`These people cannot see the page: ${outsider}.`);

  await waitForNotification(other, 'share', pageId);
  await other.goto('/notifications');
  await expect(other.getByText('Have a look at this.')).toBeVisible();
  const third = await signedIn(browser, `${outsider}@example.com`);
  await third.goto(`/pages/${pageId}`);
  await expect(third.getByRole('alert')).toContainText('does not exist, or you cannot see it');
  await other.context().close();
  await third.context().close();
});

test('asking for access only exists where the space allows it', async ({ page, browser }) => {
  const { other, pageId, spaceId } = await team(page, browser, null);
  await other.goto(`/pages/${pageId}`);
  await expect(other.getByRole('alert')).toContainText('does not exist, or you cannot see it');
  await expect(other.getByRole('button', { name: 'Ask for access' })).toHaveCount(0);

  const on = await page.request.put(`/api/v1/spaces/${spaceId}/features`, {
    data: { comments: true, reactions: true, access_requests: true },
  });
  expect(on.ok(), await on.text()).toBeTruthy();
  await other.reload();
  await other.getByRole('button', { name: 'Ask for access' }).click();
  await expect(other.getByRole('status').filter({ hasText: 'Your request was sent.' })).toBeVisible();

  await waitForNotification(page, 'access_request', pageId);
  await page.goto(`/pages/${pageId}?share=1`);
  const dialog = page.getByRole('dialog', { name: 'Share this page' });
  await expect(dialog).toContainText('Allowing makes them an editor of the whole space.');
  await dialog.getByRole('button', { name: 'Allow', exact: true }).click();
  await expect(dialog.getByRole('heading', { name: 'Requests for access' })).toHaveCount(0);

  await waitForNotification(other, 'access_answer', pageId);
  await openPage(other, pageId);
  await other.context().close();
});

test('someone who may only read can ask to edit', async ({ page, browser }) => {
  const { other, pageId } = await team(page, browser, VIEWER);
  await openPage(other, pageId);
  await other.getByRole('button', { name: 'More actions' }).click();
  await other.getByRole('menuitem', { name: 'Ask to edit' }).click();
  await expect(other.getByRole('status').filter({ hasText: 'Your request was sent.' })).toBeVisible();
  await waitForNotification(page, 'access_request', pageId);
  await other.context().close();
});

test('notification settings hold, and comments follow the chosen order', async ({ page, browser }) => {
  const { other, pageId } = await team(page, browser);
  await comment(page, pageId, 'The older one');
  await comment(page, pageId, 'The newer one');
  await other.goto('/me/notifications');
  await checkAccessibility(other);
  const comments = other.getByRole('combobox', { name: 'Comments', exact: true });
  await comments.selectOption('off');
  await other.getByLabel('Order of comments').selectOption('newest');
  await other.getByRole('button', { name: 'Save' }).click();
  await expect(other.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();
  await other.reload();
  await expect(comments).toHaveValue('off');

  await openPage(other, pageId);
  await expect(other.locator('.thread').first()).toContainText('The newer one');
  await other.context().close();
});

test('the start screen moves between its tabs and articles by keyboard', async ({ page, browser }) => {
  const { other, pageId } = await team(page, browser);
  await publishedPage(
    page,
    (await (await page.request.get(`/api/v1/pages/${pageId}`)).json()).space_id,
    'A second page',
    'Text',
  );
  await other.goto('/');
  const recent = 'Recent activity';
  const tabs = other.getByRole('tablist', { name: 'Activity' });
  await tabs.getByRole('tab', { name: 'For you' }).focus();
  await other.keyboard.press('ArrowRight');
  await expect(tabs.getByRole('tab', { name: recent })).toBeFocused();
  await expect(tabs.getByRole('tab', { name: recent })).toHaveAttribute('aria-selected', 'true');
  const feed = other.getByRole('feed', { name: recent });
  const first = feed.getByRole('article').first();
  await expect(first).toBeVisible();
  await checkAccessibility(other);
  await first.focus();
  await other.keyboard.press('PageDown');
  await expect(feed.getByRole('article').nth(1)).toBeFocused();
  await other.keyboard.press('PageUp');
  await expect(first).toBeFocused();
  await other.context().close();
});

test('the page to stop a mail needs no login, and a wrong link changes nothing', async ({ page }) => {
  await page.goto('/unsubscribe/not-a-real-token');
  await expect(page.getByRole('heading', { name: 'Stop this mail' })).toBeVisible();
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Stop this mail' }).click();
  // The same plain answer as a real one: nothing tells a stranger which tokens exist.
  await expect(page.getByRole('status')).toHaveText('Done. This mail will not come any more.');
  await expect(page).toHaveURL(/unsubscribe/);
});

test('an administrator sees that no mail server is set', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/mail');
  await expect(page.getByText('No mail server is set.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Send a test mail to yourself' })).toBeDisabled();
  await checkAccessibility(page);
});

test('a comment for editors stays with the editors', async ({ page, browser }) => {
  const { other, pageId, name } = await team(page, browser, COMMENTER);
  await page.goto(`/pages/${pageId}/edit`);
  // On a word: the column is wider than this line, so its middle is empty.
  await page.locator('.prose.editing p').dblclick({ position: { x: 60, y: 12 } });
  await page.getByRole('button', { name: 'Comment', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Comment on this text' });
  await page.getByRole('textbox', { name: 'Write a comment' }).focus();
  await page.keyboard.type('Between us editors.');
  await page.keyboard.press('Control+Enter');
  await expect(dialog).toHaveCount(0);

  await openPage(page, pageId);
  await page.getByRole('button', { name: '1 comment', exact: true }).click();
  await expect(page.getByRole('complementary', { name: 'Comments' })).toContainText('Only for editors');

  await openPage(other, pageId);
  await expect(other.getByRole('button', { name: '1 comment', exact: true })).toHaveCount(0);
  await expect(other.getByText('Between us editors.')).toHaveCount(0);
  expect(name).toBeTruthy();
  await other.context().close();
});
