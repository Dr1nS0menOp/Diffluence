// SPDX-License-Identifier: Apache-2.0
// Wave F in the browser: people, accounts, access and the console (spec 013).
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  expect,
  grant,
  login,
  newSpace,
  PASSWORD,
  publishedPage,
  test,
  totpCode,
  unique,
  VIEWER,
} from './helpers';

const BASE = { baseURL: 'http://localhost:8080' };
const MANAGER = [
  'space.view',
  'space.export',
  'page.create',
  'page.edit',
  'page.archive',
  'page.delete_own',
  'page.delete_any',
  'comment.add',
  'comment.delete_own',
  'comment.delete_any',
  'attachment.add',
  'attachment.delete_own',
  'attachment.delete_any',
  'restriction.manage',
  'space.manage_access',
];

/** A second browser, logged in as someone else. */
async function browserAs(browser: Browser, email: string, password = PASSWORD): Promise<Page> {
  const page = await (await browser.newContext(BASE)).newPage();
  await login(page, email, password);
  return page;
}

/** A setting of the whole instance, put back afterwards. Tests here run one at a time. */
async function withSection<T extends object>(
  page: Page,
  section: string,
  change: Partial<T>,
  run: () => Promise<void>,
) {
  const path = `/api/v1/admin/settings/${section}`;
  const before = (await (await page.request.get(path)).json()) as T;
  const set = await page.request.put(path, { data: { ...before, ...change } });
  expect(set.ok(), await set.text()).toBeTruthy();
  try {
    await run();
  } finally {
    await page.request.put(path, { data: before });
  }
}

test('a guest only finds the people they share a space with', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const guestMail = `${unique('guest')}@example.com`;
  const sharedMail = `${unique('shared')}@example.com`;
  const otherMail = `${unique('other')}@example.com`;
  const guest = await createUser(browser, guestMail, 'guest');
  const shared = await createUser(browser, sharedMail);
  await createUser(browser, otherMail);
  const space = await newSpace(page, 'Guest people');
  await grant(page, space.id, { type: 'user', id: guest }, VIEWER);
  await grant(page, space.id, { type: 'user', id: shared }, VIEWER);

  const as = await browserAs(browser, guestMail);
  await as.goto('/people');
  await expect(as.getByRole('link', { name: sharedMail.split('@')[0] ?? '' })).toBeVisible();
  await expect(as.getByRole('link', { name: otherMail.split('@')[0] ?? '' })).toHaveCount(0);
  await expect(as.getByText('Guest', { exact: true }).first()).toBeVisible();
  await checkAccessibility(as);
  await as.context().close();
});

test('a person hides their address, and someone else no longer sees it', async ({ page, browser }) => {
  const email = `${unique('hidden')}@example.com`;
  const id = await createUser(browser, email);
  await login(page, email);
  await page.goto('/me');
  await checkAccessibility(page);
  await page.getByRole('textbox', { name: 'Job title' }).fill('Writer');
  await page.getByLabel('Who sees Email').selectOption('me');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();

  const other = await browserAs(browser, ADMIN.email, ADMIN.password);
  await other.goto(`/people/${id}`);
  await expect(other.getByText('Writer')).toBeVisible();
  await expect(other.getByText(email)).toHaveCount(0);
  await checkAccessibility(other);
  await other.context().close();
});

test('a picture is cropped by keyboard, and shows on the profile', async ({ page, browser }) => {
  const email = `${unique('picture')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await page.goto('/me');
  const png = await page.screenshot();
  await page.getByLabel('Upload a picture').setInputFiles({
    name: 'me.png',
    mimeType: 'image/png',
    buffer: png,
  });
  const dialog = page.getByRole('dialog', { name: 'Crop your picture' });
  const canvas = dialog.getByRole('application');
  await expect(canvas).toBeVisible();
  await checkAccessibility(page);
  await canvas.focus();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('+');
  await dialog.getByRole('button', { name: 'Zoom out' }).click();
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('.person-head img')).toBeVisible();
  await page.getByRole('button', { name: 'Remove' }).click();
  await expect(page.locator('.person-head img')).toHaveCount(0);
});

test('a forgotten password answers the same for every address', async ({ page }) => {
  await page.goto('/forgot');
  await checkAccessibility(page);
  await page.getByLabel('Email').fill(ADMIN.email);
  await page.getByRole('button', { name: 'Send a link' }).click();
  await expect(page.getByRole('status')).toHaveText('If this address has an account, a mail is on its way.');
  await page.goto('/forgot');
  await page.getByLabel('Email').fill('nobody@example.com');
  await page.getByRole('button', { name: 'Send a link' }).click();
  await expect(page.getByRole('status')).toHaveText('If this address has an account, a mail is on its way.');
});

test('a link that is no longer valid says so', async ({ page }) => {
  await page.goto('/reset?token=used');
  await page.getByLabel('New password').fill('a long and fine new passphrase');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('alert')).toContainText('no longer valid');
});

test('an invited person chooses a name and password, and can log in', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/invitations');
  await checkAccessibility(page);
  const email = `${unique('invited')}@example.com`;
  await page.getByLabel('Email').fill(email);
  await page.getByRole('button', { name: 'Invite' }).click();
  const status = await page.getByRole('status').filter({ hasText: 'Send this link' }).textContent();
  await expect(page.getByRole('row', { name: new RegExp(email) })).toBeVisible();
  const link = /\/invitation\?token=\S+/.exec(status ?? '')?.[0] ?? '';

  const fresh = await (await browser.newContext(BASE)).newPage();
  await fresh.goto(link);
  await checkAccessibility(fresh);
  await fresh.getByLabel('Display name').fill('Invited Person');
  await fresh.getByLabel(/^Password/).fill('short');
  await fresh.getByRole('button', { name: 'Create' }).click();
  await expect(fresh.getByRole('alert')).toContainText('15');
  await fresh.getByLabel(/^Password/).fill(PASSWORD);
  await fresh.getByRole('button', { name: 'Create' }).click();
  await expect(fresh.getByRole('status')).toContainText('Your account is ready');
  await login(fresh, email);
  await fresh.context().close();
});

test('a new person is asked for a stronger sign-in, can put it off, and it stays in the account', async ({
  browser,
}) => {
  const email = `${unique('weak')}@example.com`;
  await createUser(browser, email);
  const page = await (await browser.newContext(BASE)).newPage();
  await page.goto('/login');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(PASSWORD);
  await page.getByRole('button', { name: 'Log in', exact: true }).click();
  const note = page.getByRole('region', { name: 'Stronger sign-in' });
  await expect(note).toBeVisible();
  await checkAccessibility(page);
  await note.getByRole('button', { name: 'Later' }).click();
  await expect(note).toHaveCount(0);
  await page.goto('/me/security');
  await expect(page.getByText('protected by a password only')).toBeVisible();
  await page.context().close();
});

test('a required second factor leaves only the account open', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('factor')}@example.com`;
  await createUser(browser, email);
  await withSection(page, 'access', { second_factor: 'members' }, async () => {
    const as = await (await browser.newContext(BASE)).newPage();
    await as.goto('/login');
    await as.getByLabel('Email').fill(email);
    await as.getByLabel('Password').fill(PASSWORD);
    await as.getByRole('button', { name: 'Log in', exact: true }).click();
    await expect(as.getByRole('alert')).toContainText('asks for a second factor');
    await expect(as.getByRole('link', { name: 'People' })).toHaveCount(0);
    expect((await as.request.get('/api/v1/spaces')).status()).toBe(403);
    await checkAccessibility(as);
    await as.context().close();
  });
});

test('a warning comes before an idle session ends, and staying keeps it open', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await withSection(page, 'access', { idle_minutes: 5 }, async () => {
    await page.clock.install();
    await page.goto('/people');
    await expect(page.getByRole('heading', { name: 'People' })).toBeVisible();
    await page.clock.fastForward('03:05');
    const dialog = page.getByRole('dialog', { name: 'Still there?' });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('timer')).toContainText('logged out in');
    await checkAccessibility(page);
    await dialog.getByRole('button', { name: 'Stay logged in' }).click();
    await page.clock.runFor(2000);
    await expect(dialog).toHaveCount(0);
  });
});

test('a sensitive action asks for proof, and goes on after it', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  let asked = false;
  await page.route('**/api/v1/admin/maintenance', async (route) => {
    if (asked) return route.fallback();
    asked = true;
    return route.fulfill({
      status: 403,
      json: { error: { code: 'reauth_required', message: 'Confirm who you are to do this.' } },
    });
  });
  await page.goto('/admin/care');
  await page.getByRole('button', { name: 'Turn on' }).click();
  const dialog = page.getByRole('dialog', { name: 'Confirm it is you' });
  await expect(dialog).toBeVisible();
  await checkAccessibility(page);
  await dialog.getByLabel('Password').fill('wrong password');
  await dialog.getByRole('button', { name: 'Confirm' }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await dialog.getByLabel('Password').fill(ADMIN.password);
  await dialog.getByRole('button', { name: 'Confirm' }).click();
  await expect(page.getByRole('button', { name: 'Turn off' })).toBeVisible();

  // Maintenance mode is on now: a write is refused, and the screen says why.
  await page.reload();
  await expect(page.getByTestId('maintenance-note')).toBeVisible();
  const refused = await page.request.post('/api/v1/spaces', { data: { key: unique('M'), name: 'No' } });
  expect(refused.status()).toBe(503);
  await page.getByRole('button', { name: 'Turn off' }).click();
  await expect(page.getByRole('button', { name: 'Turn on' })).toBeVisible();
});

test('a token is shown once, and can be taken back', async ({ page, browser }) => {
  const email = `${unique('token')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await page.goto('/me/tokens');
  await checkAccessibility(page);
  await page.getByLabel('Name').fill('My script');
  await page.getByRole('button', { name: 'New token' }).click();
  const dialog = page.getByRole('dialog', { name: 'New token' });
  await expect(dialog.getByTestId('token-secret')).toContainText('dfl_');
  await dialog.getByRole('button', { name: 'Close' }).click();
  await expect(page.getByRole('row', { name: /My script/ })).toBeVisible();
  await page.getByRole('button', { name: 'Revoke' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Revoke' }).click();
  await expect(page.getByRole('row', { name: /My script/ })).toHaveCount(0);
});

test('a person downloads their data and asks for removal', async ({ page, browser }) => {
  const email = `${unique('data')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await page.goto('/me/data');
  await checkAccessibility(page);
  const download = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Download my data' }).click();
  expect((await download).suggestedFilename()).toBe('my-diffluence-data.json');
  await page.getByRole('button', { name: 'Ask for removal' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Ask for removal' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'has been sent' })).toBeVisible();
});

test('dates follow the time zone and format a person chose', async ({ page, browser }) => {
  const email = `${unique('dates')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await page.goto('/me/preferences');
  await checkAccessibility(page);
  await page.getByLabel('Date format').selectOption('iso');
  await page.getByLabel('Time zone').selectOption('Asia/Tokyo');
  await page.goto('/me/security');
  const stamp = page
    .getByRole('table')
    .first()
    .getByText(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
  await expect(stamp.first()).toBeVisible();
});

test('a personal space starts private, and is handed over when its owner leaves', async ({
  page,
  browser,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('owner')}@example.com`;
  const id = await createUser(browser, email);
  const heirMail = `${unique('heir')}@example.com`;
  await createUser(browser, heirMail);

  const owner = await browserAs(browser, email);
  await owner.getByRole('button', { name: 'Account' }).click();
  await owner.getByRole('menuitem', { name: 'Create personal space' }).click();
  await expect(owner).toHaveURL(/\/pages\//);
  await owner.getByRole('button', { name: 'Account' }).click();
  await expect(owner.getByRole('menuitem', { name: 'My space' })).toBeVisible();
  await owner.context().close();

  const disabled = await page.request.post(`/api/v1/admin/users/${id}/disable`);
  expect(disabled.ok(), await disabled.text()).toBeTruthy();
  await page.goto('/admin/summary');
  await checkAccessibility(page);
  const row = page.getByRole('listitem').filter({ hasText: email.split('@')[0] ?? '' });
  await row.getByRole('button', { name: 'Hand over' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByPlaceholder('Find a person').fill(heirMail.split('@')[0] ?? '');
  await dialog.getByRole('option', { name: new RegExp(heirMail) }).click();
  await expect(row).toHaveCount(0);
});

test('a manager runs access, and cannot reach administer or the settings', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('manager')}@example.com`;
  const id = await createUser(browser, email);
  const viewerMail = `${unique('viewer')}@example.com`;
  const viewer = await createUser(browser, viewerMail);
  const space = await newSpace(page, 'Managed space');
  await grant(page, space.id, { type: 'user', id }, MANAGER);
  await grant(page, space.id, { type: 'user', id: viewer }, VIEWER);

  const as = await browserAs(browser, email);
  await as.goto(`/spaces/${space.key}/access`);
  await checkAccessibility(as);
  const row = as.getByRole('row', { name: new RegExp(viewerMail.split('@')[0] ?? '') });
  await row.getByLabel('Role').selectOption('editor');
  await expect(as.getByRole('status').filter({ hasText: 'Gains' })).toBeVisible();

  // Taking someone's access asks first.
  await row.getByRole('button', { name: 'Remove access' }).click();
  await as
    .getByRole('dialog', { name: 'Someone loses access' })
    .getByRole('button', { name: 'Save' })
    .click();
  await expect(row).toHaveCount(0);

  const reach = await as.request.put(`/api/v1/spaces/${space.id}/grants`, {
    data: { subject: { type: 'user', id }, permissions: [...MANAGER, 'space.administer'] },
  });
  expect(reach.status()).toBe(409);
  expect(await reach.text()).toContain('needs_administer');
  await expect(as.getByRole('link', { name: 'Space settings' })).toHaveCount(0);
  await as.context().close();
});

test('everyone who can view a space sees who has access, and the export is safe', async ({
  page,
  browser,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('reader')}@example.com`;
  const id = await createUser(browser, email);
  const space = await newSpace(page, 'Open list');
  await grant(page, space.id, { type: 'user', id }, VIEWER);
  await page.goto(`/spaces/${space.key}/access`);
  await page.getByRole('tab', { name: 'People' }).click();
  await checkAccessibility(page);
  await expect(page.getByRole('row', { name: new RegExp(email.split('@')[0] ?? '') })).toContainText(
    'Directly',
  );
  const csv = await page.request.get(`/api/v1/spaces/${space.id}/access.csv`);
  expect(csv.status()).toBe(200);

  const as = await browserAs(browser, email);
  await as.goto(`/spaces/${space.key}/people`);
  await checkAccessibility(as);
  await expect(as.locator('.member-list')).toContainText(email.split('@')[0] ?? '');
  await as.context().close();
});

test('a group owner manages members outside the console', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const ownerMail = `${unique('gowner')}@example.com`;
  const owner = await createUser(browser, ownerMail);
  const memberMail = `${unique('gmember')}@example.com`;
  await createUser(browser, memberMail);
  const made = await page.request.post('/api/v1/admin/groups', { data: { name: unique('Team') } });
  const group = (await made.json()) as { id: string };
  await page.request.put(`/api/v1/admin/groups/${group.id}/owners`, { data: { owners: [owner] } });

  const as = await browserAs(browser, ownerMail);
  await as.goto('/me');
  await as.getByRole('listitem').filter({ hasText: 'Owner' }).getByRole('link').click();
  await checkAccessibility(as);
  await as
    .locator('main')
    .getByRole('combobox')
    .fill(memberMail.split('@')[0] ?? '');
  await as.getByRole('option', { name: new RegExp(memberMail) }).click();
  await expect(as.getByRole('listitem').filter({ hasText: memberMail.split('@')[0] ?? '' })).toBeVisible();
  await as.context().close();
});

test('the console opens on a summary, and finds a setting by name', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/summary');
  await expect(page.getByRole('term').filter({ hasText: 'Pages' })).toBeVisible();
  await page.getByPlaceholder('Find a setting').fill('idle');
  await page.getByRole('button', { name: /idle minutes/ }).click();
  await expect(page).toHaveURL(/\/admin\/rules$/);
  await expect(page.getByLabel(/idle minutes/)).toBeVisible();
  await checkAccessibility(page);
  await page.goto('/admin/permissions');
  await expect(page.getByRole('heading', { name: 'Who holds what' })).toBeVisible();
  await checkAccessibility(page);
  await page.goto('/admin/care');
  await page.getByRole('button', { name: 'Look for them' }).click();
  await expect(page.getByText('No unused files.')).toBeVisible();
  await checkAccessibility(page);
});

test('users filter through the address, and a bulk action says how many first', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('bulk')}@example.com`;
  await createUser(browser, email, 'guest');
  await page.goto('/admin/users?type=guest');
  await expect(page.getByLabel('Type')).toHaveValue('guest');
  await page.getByRole('searchbox', { name: 'Search' }).fill(email);
  const name = email.split('@')[0] ?? '';
  await page.getByRole('checkbox', { name: `Select ${name}` }).check();
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Disable' }).first().click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('1 account');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('row', { name: new RegExp(name) })).toContainText('Disabled');
});

test('anonymising needs a reason, and leaves a neutral name on the work', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('erased')}@example.com`;
  const id = await createUser(browser, email);
  await page.goto('/admin/users');
  const name = email.split('@')[0] ?? '';
  await page.getByRole('searchbox', { name: 'Search' }).fill(email);
  await page.getByRole('button', { name, exact: true }).click();
  await page.getByRole('button', { name: 'Anonymise' }).click();
  const dialog = page.getByRole('dialog', { name: 'Anonymise' });
  await expect(dialog).toContainText('audit rows');
  await checkAccessibility(page);
  await dialog.getByLabel('Reason').fill('Request by mail');
  await dialog.getByRole('button', { name: 'Anonymise' }).click();
  await expect(dialog).toHaveCount(0);
  const after = await page.request.get(`/api/v1/admin/users/${id}`);
  expect(await after.text()).not.toContain(name);
});

test('a failing accent is refused by name, and a good pair is used before login', async ({
  page,
  browser,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/look');
  await checkAccessibility(page);
  await page.getByLabel('For the light theme').fill('#ffff00');
  await page.getByLabel('For the dark theme').fill('#7fb0f5');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('alert')).toContainText('contrast');

  await page.getByLabel('For the light theme').fill('#7a1fa2');
  await page.getByLabel('Name', { exact: true }).fill('Our Wiki');
  await page.getByLabel('Welcome text on the login page').fill('Welcome, colleague.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();

  try {
    const fresh = await (await browser.newContext(BASE)).newPage();
    await fresh.goto('/login');
    await expect(fresh.getByText('Our Wiki')).toBeVisible();
    await expect(fresh.getByText('Welcome, colleague.')).toBeVisible();
    const accent = await fresh.evaluate(() =>
      document.documentElement.style.getPropertyValue('--color-accent'),
    );
    expect(accent).toContain('#7a1fa2');
    await fresh.context().close();
  } finally {
    await page.getByRole('button', { name: 'Back to the defaults' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Back to the defaults' }).click();
    await expect(page.getByLabel('Name', { exact: true })).toHaveValue('');
  }
});

test('an SVG logo is refused', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/look');
  await page.getByLabel('Upload a logo').setInputFiles({
    name: 'logo.svg',
    mimeType: 'image/svg+xml',
    buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>'),
  });
  await expect(page.getByRole('alert')).toBeVisible();
});

test('a first visit explains the basics, once', async ({ page, browser }) => {
  const email = `${unique('new')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await page.goto('/');
  await page.getByRole('button', { name: 'Show me around' }).click();
  const tour = page.getByRole('dialog');
  await checkAccessibility(page);
  await tour.getByRole('button', { name: 'Next' }).click();
  await tour.getByRole('button', { name: 'Next' }).click();
  await tour.getByRole('button', { name: 'Done' }).click();
  await expect(page.getByRole('button', { name: 'Show me around' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Got it' }).click();
  await expect(page.getByRole('heading', { name: 'New here?' })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'New here?' })).toHaveCount(0);
});

test('a new space starts with a text and the access chosen', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/spaces');
  await page.getByRole('button', { name: 'New space' }).click();
  const dialog = page.getByRole('dialog');
  const name = unique('Open ');
  await dialog.getByLabel('Name').fill(name);
  await dialog.getByLabel('Who gets access').selectOption('read_only_kb');
  await expect(dialog).toContainText('Every member can read here');
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Create' }).click();
  await expect(page.getByText('Welcome to this space.')).toBeVisible();

  const email = `${unique('member')}@example.com`;
  await createUser(browser, email);
  const as = await browserAs(browser, email);
  await as.goto('/spaces');
  await expect(as.locator('#main').getByRole('link', { name })).toBeVisible();
  await as.context().close();
});

test('a space administrator reads the audit of their space', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Audited space');
  await publishedPage(page, space.id, 'Logged page', 'text');
  await page.goto(`/spaces/${space.key}/audit`);
  await expect(page.getByRole('heading', { name: 'Audit of this space' })).toBeVisible();
  await expect(page.getByRole('table')).toContainText('page.');
  await checkAccessibility(page);
});

test('turning off a factor offers to end the other sessions', async ({ page, browser }) => {
  const email = `${unique('offer')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  const other = await browserAs(browser, email);
  const setup = await page.request.post('/api/v1/me/totp/begin', { data: { password: PASSWORD } });
  const { secret } = (await setup.json()) as { secret: string };
  const confirm = await page.request.post('/api/v1/me/totp/confirm', { data: { code: totpCode(secret) } });
  expect(confirm.ok(), await confirm.text()).toBeTruthy();

  await page.goto('/me/security');
  const section = page.getByRole('heading', { name: 'Authenticator app' }).locator('..');
  await section.getByLabel('Current password').fill(PASSWORD);
  await section.getByRole('button', { name: 'Remove' }).click();
  await section.getByRole('button', { name: 'Log out everywhere else' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'logged out everywhere else' })).toBeVisible();
  await checkAccessibility(page);
  expect((await other.request.get('/api/v1/me')).status()).toBe(401);
  await other.context().close();
});

test('a SCIM token shows when it ends, and rotating gives a successor', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/scim');
  const name = unique('rotate');
  await page.getByLabel('Token name').fill(name);
  await page.getByRole('button', { name: 'New token' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();
  const row = page.getByRole('row', { name: new RegExp(name) });
  await row.getByRole('button', { name: 'Rotate' }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.locator('code')).toContainText('dfl_scim_');
  await expect(dialog).toContainText('7 days');
  await dialog.getByRole('button', { name: 'Close' }).click();
  await expect(page.getByRole('row', { name: new RegExp(name) })).toHaveCount(2);
  await checkAccessibility(page);
});

test('a page out of reach shows in the audit log by id, with a censored title', async ({ page, browser }) => {
  const email = `${unique('hidden')}@example.com`;
  await createUser(browser, email, 'member', ['use_product', 'create_space']);
  const owner = await browserAs(browser, email);
  const space = await newSpace(owner, 'Hidden from the console');
  const hidden = await publishedPage(owner, space.id, 'Merger with Initech', 'text');
  await owner.context().close();

  await login(page, ADMIN.email, ADMIN.password);
  await page.goto(`/admin/audit?object_id=${hidden}`);
  await expect(page.getByRole('table')).toContainText('(Censored title)');
  await expect(page.getByRole('table')).not.toContainText('Initech');
  await checkAccessibility(page);
  await page.getByRole('table').getByRole('button').first().click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('definition').first()).toHaveText('(Censored title)');
  await expect(dialog).toContainText('Hidden from the console');
  await expect(dialog).not.toContainText('Initech');
  await checkAccessibility(page);
});

test('someone with the role asks for an invitation, an administrator approves, and a member sees no option', async ({
  page,
  browser,
}) => {
  const inviterEmail = `${unique('inviter')}@example.com`;
  const memberEmail = `${unique('member')}@example.com`;
  const outsider = `${unique('outsider')}@example.org`;
  await createUser(browser, inviterEmail, 'member', ['use_product', 'create_space', 'invite_people']);
  const memberId = await createUser(browser, memberEmail);
  const inviter = await browserAs(browser, inviterEmail);
  const space = await newSpace(inviter, 'Shared with outsiders');
  await grant(inviter, space.id, { type: 'user', id: memberId }, VIEWER);
  const pageId = await publishedPage(inviter, space.id, 'Merger with Initech', 'text');

  const member = await browserAs(browser, memberEmail);
  await member.goto(`/pages/${pageId}?share=1`);
  const theirDialog = member.getByRole('dialog', { name: 'Share this page' });
  await expect(theirDialog.getByRole('heading', { name: 'Who can see this page' })).toBeVisible();
  await expect(theirDialog.getByRole('button', { name: 'Ask an administrator' })).toHaveCount(0);
  await member.context().close();

  await inviter.goto(`/pages/${pageId}?share=1`);
  const dialog = inviter.getByRole('dialog', { name: 'Share this page' });
  await dialog.getByLabel('Email').fill(outsider);
  await dialog.getByLabel('Note for the administrator').fill('Our auditor');
  await dialog.getByRole('button', { name: 'Ask an administrator' }).click();
  await expect(dialog.getByRole('status')).toContainText('waits for an administrator');
  await checkAccessibility(inviter);

  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/invitations');
  const request = page.getByRole('listitem').filter({ hasText: outsider });
  await expect(request).toContainText('Shared with outsiders');
  await expect(request).not.toContainText('Initech');
  await expect(request.getByRole('button', { name: 'Approve' })).toBeDisabled();
  await checkAccessibility(page);
  await request.getByLabel(/^Reason/).fill('Known to us');
  await request.getByRole('button', { name: 'Approve' }).click();
  await expect(page.getByRole('row', { name: new RegExp(outsider) })).toBeVisible();
  await expect(page.getByRole('listitem').filter({ hasText: outsider })).toHaveCount(0);

  await inviter.goto('/notifications');
  await expect(inviter.getByText(`approved your invitation for ${outsider}`)).toBeVisible();
  await expect(inviter.getByText('Known to us')).toBeVisible();
  await inviter.context().close();
});

test('the account stays in reach in a full top bar', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'A space with a long name for the filter');
  await page.goto(`/pages/${space.home_page_id}`);
  await expect(page.getByRole('button', { name: 'Create', exact: true })).toBeVisible();
  const account = await page.getByRole('button', { name: 'Account' }).boundingBox();
  const width = page.viewportSize()?.width ?? 0;
  expect((account?.x ?? width) + (account?.width ?? 0)).toBeLessThanOrEqual(width);
  await expect(page.getByRole('combobox', { name: 'Search' })).toBeInViewport({ ratio: 1 });
});
