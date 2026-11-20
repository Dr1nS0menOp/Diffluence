// SPDX-License-Identifier: Apache-2.0
// The administration as blueprint 03 arranges it (spec 081): the sidebar in two tiers, the head
// of a section, the forms with Save and Reset changes, and the guards of spec 076 at their new
// address.
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  expect,
  login,
  PASSWORD,
  test,
  totpCode,
  unique,
} from './helpers';

const BASE = { baseURL: 'http://localhost:8080' };
const SHORT_SCREEN = { width: 1280, height: 520 };
const PHONE = { width: 360, height: 740 };

/** A fresh administrator, so what they fold never reaches another test. */
async function freshAdministrator(
  page: Page,
  browser: Browser,
  permissions = ['use_product', 'administrator'],
) {
  const email = `${unique('admin')}@example.com`;
  await createUser(browser, email, 'member', permissions);
  await login(page, email, PASSWORD);
}

/** A system administrator with an authenticator app, in a browser of their own (spec 076). */
async function systemAdministrator(browser: Browser) {
  const email = `${unique('keeper')}@example.com`;
  await createUser(browser, email, 'member', ['use_product', 'administrator', 'system_administrator']);
  const context = await browser.newContext(BASE);
  const res = await context.request.post('/api/v1/auth/login', { data: { email, password: PASSWORD } });
  expect(res.ok(), await res.text()).toBeTruthy();
  const setup = await context.request.post('/api/v1/me/totp/begin', { data: { password: PASSWORD } });
  const { secret } = (await setup.json()) as { secret: string };
  const confirm = await context.request.post('/api/v1/me/totp/confirm', { data: { code: totpCode(secret) } });
  expect(confirm.ok(), await confirm.text()).toBeTruthy();
  const { recovery_codes: codes } = (await confirm.json()) as { recovery_codes: string[] };
  return { page: await context.newPage(), codes, close: () => context.close() };
}

const sidebar = (page: Page) => page.getByRole('navigation', { name: 'Administration' });
const aside = (page: Page) => page.locator('aside.settings-sidebar');

test('the sidebar has a way back, three tool sections and five groups that fold, in order', async ({
  page,
  browser,
}) => {
  await freshAdministrator(page, browser);
  await page.goto('/admin/summary');
  await expect(page.getByRole('heading', { name: 'Summary', level: 1 })).toBeVisible();
  await expect(page.locator('.settings-tools-title')).toHaveText(['Overview', 'Tools', 'People and access']);
  await expect(sidebar(page).getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
  await expect(page.locator('.settings-group > button')).toHaveText([
    'Instance',
    'Security',
    'Permissions',
    'Look',
    'Data',
  ]);
  await expect(sidebar(page).getByRole('link', { name: 'Summary' })).toHaveAttribute('aria-current', 'page');
  // Every entry says its whole name on hover and focus, however narrow the sidebar.
  await expect(sidebar(page).getByRole('link', { name: 'Login and second factor' })).toHaveAttribute(
    'title',
    'Login and second factor',
  );
  // While its experimental part is on, an entry carries the word in a lozenge (ADM-067).
  await expect(sidebar(page).getByRole('link', { name: /Import a space/ })).toContainText('Experimental');
  await checkAccessibility(page);
  await page.getByRole('link', { name: 'Back to the start screen' }).click();
  await expect(page).toHaveURL(/\/$/);
});

test('a folded group stays shut across a reload and another section, and opens for a jump', async ({
  page,
  browser,
}) => {
  await freshAdministrator(page, browser);
  await page.goto('/admin/summary');
  const security = page.getByRole('button', { name: 'Security', exact: true });
  const saved = page.waitForResponse(
    (r) => r.url().includes('/me/preferences') && r.request().method() === 'PUT',
  );
  await security.click();
  await saved;
  await expect(security).toHaveAttribute('aria-expanded', 'false');
  await expect(sidebar(page).getByRole('link', { name: 'Sessions' })).toHaveCount(0);
  // Folding changes nothing in the large area.
  await expect(page.getByRole('heading', { name: 'Summary', level: 1 })).toBeVisible();
  await page.reload();
  await expect(security).toHaveAttribute('aria-expanded', 'false');
  await sidebar(page).getByRole('link', { name: 'Look', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Look', level: 1 })).toBeVisible();
  await expect(security).toHaveAttribute('aria-expanded', 'false');

  // The jump field finds a section by its name, also inside a shut group, and shows it open.
  await page.getByRole('searchbox', { name: 'Find a setting' }).fill('second factor');
  await page.getByRole('button', { name: 'Login and second factor' }).click();
  await expect(page).toHaveURL(/\/admin\/login$/);
  await expect(security).toHaveAttribute('aria-expanded', 'true');
  await expect(sidebar(page).getByRole('link', { name: 'Login and second factor' })).toHaveAttribute(
    'aria-current',
    'page',
  );
  // Pressing the heading shuts it again, and the shut state was never forgotten.
  await security.click();
  await expect(security).toHaveAttribute('aria-expanded', 'false');

  // It finds a setting inside a section too, at the section that holds it now.
  await page.getByRole('searchbox', { name: 'Find a setting' }).fill('font');
  await page.getByRole('button', { name: 'Font', exact: true }).click();
  await expect(page).toHaveURL(/\/admin\/look$/);
  await expect(page.getByRole('heading', { name: 'Font', level: 2 })).toBeVisible();
  await checkAccessibility(page);
});

test('the sidebar keeps its place between sections, and shows a section opened by address', async ({
  page,
  browser,
}) => {
  await freshAdministrator(page, browser);
  await page.setViewportSize(SHORT_SCREEN);
  // Opened by address, an entry deep in Data is brought into view, and the header row stays.
  await page.goto('/admin/webhooks');
  const webhooks = sidebar(page).getByRole('link', { name: 'Webhooks' });
  await expect(webhooks).toHaveAttribute('aria-current', 'page');
  await expect(webhooks).toBeInViewport();
  await expect(page.locator('.settings-head')).toBeInViewport();
  await expect(page.locator('.settings-head')).toHaveClass(/scrolled/);

  // Moving by the sidebar never moves it.
  const before = await aside(page).evaluate((element) => element.scrollTop);
  expect(before).toBeGreaterThan(0);
  await sidebar(page).getByRole('link', { name: 'Backups' }).click();
  await expect(page.getByRole('heading', { name: 'Backups', level: 1 })).toBeVisible();
  await sidebar(page).getByRole('link', { name: 'Care' }).click();
  await expect(page.getByRole('heading', { name: 'Care', level: 1 })).toBeVisible();
  expect(await aside(page).evaluate((element) => element.scrollTop)).toBe(before);

  // At the top, nothing lies under the header row, so it has no line.
  await aside(page).evaluate((element) => element.scrollTo({ top: 0 }));
  await expect(page.locator('.settings-head')).not.toHaveClass(/scrolled/);
  await checkAccessibility(page);
});

test('an experimental section that is off leaves the sidebar, and its address says so', async ({
  page,
  browser,
}) => {
  await freshAdministrator(page, browser);
  await page.route('**/api/v1/system/capabilities', async (route) => {
    const response = await route.fetch();
    const capabilities = (await response.json()) as Record<string, boolean>;
    await route.fulfill({ response, json: { ...capabilities, automation: false, bundles: false } });
  });
  await page.goto('/admin/summary');
  await expect(sidebar(page).getByRole('link', { name: 'Spaces', exact: true })).toBeVisible();
  await expect(sidebar(page).getByRole('link', { name: /Automation/ })).toHaveCount(0);
  await expect(sidebar(page).getByRole('link', { name: /Import a space/ })).toHaveCount(0);
  await page.getByRole('searchbox', { name: 'Find a setting' }).fill('import a space');
  await expect(page.getByRole('button', { name: 'Import a space' })).toHaveCount(0);
  await page.goto('/admin/automation');
  await expect(page.getByRole('heading', { name: 'Automation', level: 1 })).toBeVisible();
  await expect(page.getByText('This is an experimental part, and it is off')).toBeVisible();
  await checkAccessibility(page);
});

test('each moved part shows at its new address and no longer at its old one', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const moved: [string, string, string][] = [
    // [old address, new address, a heading of the part]
    ['users', 'service-accounts', 'Service accounts'],
    ['rules', 'login', 'Access'],
    ['permissions', 'instance-permissions', 'Who holds what'],
    ['look', 'announcements', 'Banner'],
    ['settings', 'look', 'Font'],
    ['spaces', 'spaces-trash', 'Spaces in the trash'],
    ['export', 'import-space', 'Bundles'],
    ['permissions', 'organising', 'Space categories'],
    ['organising', 'templates', 'Space templates'],
    ['rules', 'outgoing', 'Outside addresses allowed in link cards and embeds'],
  ];
  for (const [before, after, heading] of moved) {
    await page.goto(`/admin/${after}`);
    await expect(page.getByRole('heading', { name: heading, exact: true }).first(), after).toBeVisible();
    await page.goto(`/admin/${before}`);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    await expect(page.getByRole('heading', { name: heading, exact: true }), before).toHaveCount(0);
  }
  await page.goto('/admin/space-defaults');
  await expect(page.getByLabel('Default permission set for new spaces')).toBeVisible();
  await page.goto('/admin/organising');
  await expect(page.getByLabel('Default permission set for new spaces')).toHaveCount(0);
  // The section once called Settings is General, at its old address (D2).
  await page.goto('/admin/settings');
  await expect(page.getByRole('heading', { name: 'General', level: 1 })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Language, time and help' })).toHaveCount(0);
  await page.goto('/admin/rules');
  await expect(page.getByRole('heading', { name: 'Language, time and help' })).toBeVisible();
});

test('Save and Reset changes wait for a change, put the stored values back and keep what failed', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/settings');
  const unit = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Trash and files' }) });
  const save = unit.getByRole('button', { name: 'Save', exact: true });
  const reset = unit.getByRole('button', { name: 'Reset changes' });
  const upload = unit.getByLabel('Maximum upload size (MiB)');
  await expect(save).toBeDisabled();
  await expect(reset).toBeDisabled();
  await expect(save).toHaveAccessibleDescription('Nothing has changed yet.');
  const stored = await upload.inputValue();
  await upload.fill(String(Number(stored) + 1));
  await expect(save).toBeEnabled();
  await expect(reset).toBeEnabled();
  await reset.click();
  await expect(upload).toHaveValue(stored);
  await expect(save).toBeDisabled();

  // A failed save keeps what was typed.
  await page.route('**/api/v1/admin/settings', (route) =>
    route.request().method() === 'PUT'
      ? route.fulfill({
          status: 500,
          json: { error: { code: 'internal', message: 'The database is away.' } },
        })
      : route.fallback(),
  );
  await upload.fill('77');
  await save.click();
  await expect(unit.getByRole('alert')).toBeVisible();
  await expect(upload).toHaveValue('77');
  await expect(save).toBeEnabled();
  await checkAccessibility(page);
});

test('weakening the login rule at its new address keeps the guard of spec 076', async ({ page, browser }) => {
  // An administrator without system administration is refused, and asked for nothing.
  await freshAdministrator(page, browser);
  await page.goto('/admin/login');
  await page.getByLabel('Second factor required for').selectOption('none');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('needs a system administrator');
  await expect(page.getByLabel('Code from your authenticator app, or a recovery code')).toHaveCount(0);
  await checkAccessibility(page);

  // A system administrator is asked for a code, and the change holds after it.
  const keeper = await systemAdministrator(browser);
  const before = (await (await keeper.page.request.get('/api/v1/admin/settings/access')).json()) as Record<
    string,
    unknown
  >;
  try {
    await keeper.page.goto('/admin/login');
    await keeper.page.getByLabel('Second factor required for').selectOption('none');
    await keeper.page.getByRole('button', { name: 'Save', exact: true }).click();
    const code = keeper.page.getByLabel('Code from your authenticator app, or a recovery code');
    await expect(code).toBeVisible();
    await checkAccessibility(keeper.page);
    // Reset changes takes the confirmation away with the change.
    await keeper.page.getByRole('button', { name: 'Reset changes' }).click();
    await expect(code).toHaveCount(0);
    await keeper.page.getByLabel('Second factor required for').selectOption('none');
    await keeper.page.getByRole('button', { name: 'Save', exact: true }).click();
    await code.fill(keeper.codes[0] ?? '');
    await keeper.page.getByRole('button', { name: 'Confirm and save' }).click();
    await expect(keeper.page.getByRole('status').filter({ hasText: 'Saved.' })).toBeVisible();
    const stored = (await (await keeper.page.request.get('/api/v1/admin/settings/access')).json()) as {
      second_factor: string;
    };
    expect(stored.second_factor).toBe('none');
  } finally {
    // A stricter rule needs no confirmation, so the old one goes back as a plain change.
    const back = await keeper.page.request.put('/api/v1/admin/settings/access', { data: before });
    expect(back.ok(), await back.text()).toBeTruthy();
    await keeper.close();
  }
});

test('the administration fits a phone and reads right to left', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.setViewportSize(PHONE);
  for (const address of ['/admin/users', '/admin/login', '/admin/spaces-trash']) {
    await page.goto(address);
    await page.evaluate(() => document.documentElement.setAttribute('dir', 'rtl'));
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth), address).toBe(
      false,
    );
    await checkAccessibility(page);
  }
});
