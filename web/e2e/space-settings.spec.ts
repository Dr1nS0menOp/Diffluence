// SPDX-License-Identifier: Apache-2.0
// The settings of a space as blueprint 03 arranges them (spec 083): the head, four pinned entries
// and five groups that fold, the jump field, the screens of their own beside the sidebar, every
// old address, Details as one form, and who sees which entry.
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
  test,
  unique,
  VIEWER,
} from './helpers';

const BASE = { baseURL: 'http://localhost:8080' };
const PHONE = { width: 360, height: 740 };

/** A fresh administrator with a space of their own, so what they fold never reaches another test. */
async function freshOwner(page: Page, browser: Browser, name: string) {
  const email = `${unique('owner')}@example.com`;
  await createUser(browser, email, 'member', ['use_product', 'create_space', 'administrator']);
  await login(page, email, PASSWORD);
  return newSpace(page, name);
}

/** A person who holds only the given permissions in a space, in a browser of their own. */
async function holderOf(browser: Browser, admin: Page, spaceId: string, permissions: string[]) {
  const email = `${unique('holder')}@example.com`;
  const id = await createUser(browser, email);
  await grant(admin, spaceId, { type: 'user', id }, ['space.view', ...permissions]);
  const page = await (await browser.newContext(BASE)).newPage();
  await login(page, email);
  return page;
}

const nav = (page: Page) => page.getByRole('navigation', { name: 'Space settings' });

test('the head leads back, then four entries, a divider and five groups in order', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'Arranged space');
  await page.goto(`/spaces/${space.key}/settings`);
  await expect(page.getByRole('heading', { name: 'Details', level: 1 })).toBeVisible();
  await expect(page.locator('.space-head-name')).toHaveText('Arranged space');
  await expect(nav(page).locator('.side-links').first().locator('.side-link-text')).toHaveText([
    'Automation',
    'People',
    'Content',
    'Details',
  ]);
  await expect(nav(page).locator('hr.settings-divider')).toHaveCount(1);
  await expect(page.locator('.settings-group > button')).toHaveText([
    'General',
    'Access',
    'Pages and files',
    'Look',
    'Connections',
  ]);
  await expect(nav(page).getByRole('link', { name: 'Details' })).toHaveAttribute('aria-current', 'page');
  await expect(nav(page).getByRole('link', { name: /Automation/ })).toContainText('Experimental');
  await checkAccessibility(page);
  await page.getByRole('link', { name: 'Back to the space' }).click();
  await expect(page).toHaveURL(new RegExp(`/pages/${space.home_page_id}$`));
});

test('a folded group stays shut across a reload and another entry, and never folds the administration', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'Folding space');
  await page.goto(`/spaces/${space.key}/settings`);
  const group = page.getByRole('button', { name: 'Pages and files' });
  await group.click();
  await expect(group).toHaveAttribute('aria-expanded', 'false');
  await expect(nav(page).getByRole('link', { name: 'Trash' })).toHaveCount(0);
  await nav(page).getByRole('link', { name: 'Features' }).click();
  await page.reload();
  await expect(page.getByRole('button', { name: 'Pages and files' })).toHaveAttribute(
    'aria-expanded',
    'false',
  );
  // A jump opens a shut group while the field holds text.
  await page.getByRole('searchbox', { name: 'Jump to a setting' }).fill('trash');
  await expect(nav(page).getByRole('link', { name: 'Trash' })).toBeVisible();
  await page.goto('/admin/summary');
  await expect(page.getByRole('button', { name: 'Look' })).toHaveAttribute('aria-expanded', 'true');
  await expect(page.getByRole('button', { name: 'Data' })).toHaveAttribute('aria-expanded', 'true');
});

test('the jump field finds an entry by its name and by an old name, and says when nothing matches', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'Jumping space');
  await page.goto(`/spaces/${space.key}/settings?section=features`);
  const jump = page.getByRole('searchbox', { name: 'Jump to a setting' });
  // Space key was a section of its own; it lives in Details now (D11).
  await jump.fill('space key');
  await expect(nav(page).getByRole('link')).toHaveText(['Details']);
  await jump.press('Enter');
  await expect(page).toHaveURL(/section=details$/);
  await expect(page.getByRole('heading', { name: 'Details', level: 1 })).toBeVisible();
  await jump.fill('layout');
  await jump.press('Enter');
  await expect(page.getByRole('heading', { name: 'Layout', level: 1 })).toBeVisible();
  await jump.fill('nothing like this');
  await expect(nav(page).getByText('No setting has that name.')).toBeVisible();
  await checkAccessibility(page);
  await jump.press('Escape');
  await expect(jump).toHaveValue('');
});

test('an entry deep in the sidebar opened by address shows there, and the list keeps its place', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'Scrolled space');
  await page.setViewportSize({ width: 1280, height: 520 });
  await page.goto(`/spaces/${space.key}/settings/webhooks`);
  const webhooks = nav(page).getByRole('link', { name: 'Webhooks' });
  await expect(webhooks).toHaveAttribute('aria-current', 'page');
  await expect(webhooks).toBeInViewport();
  // The list under the head is what scrolls (SPC-136); moving between sections keeps its place.
  await page.goto(`/spaces/${space.key}/settings?section=pdf`);
  await expect(page.getByRole('heading', { name: 'PDF design', level: 1 })).toBeVisible();
  const sidebar = page.locator('nav.settings-nav');
  const before = await sidebar.evaluate((element) => element.scrollTop);
  expect(before).toBeGreaterThan(0);
  await nav(page).getByRole('link', { name: 'Layout' }).click();
  await expect(page.getByRole('heading', { name: 'Layout', level: 1 })).toBeVisible();
  expect(await sidebar.evaluate((element) => element.scrollTop)).toBe(before);
  // The head stays in place while the list scrolls under it.
  await expect(page.getByRole('searchbox', { name: 'Jump to a setting' })).toBeInViewport();
});
test('Content, Trash and Automation open beside the settings sidebar, their entry marked', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'Screens space');
  for (const [entry, address] of [
    ['Content', 'content'],
    ['Trash', 'trash'],
    ['Automation', 'settings/automation'],
    ['Permissions', 'access'],
    ['Audit of the space', 'audit'],
  ] as const) {
    await page.goto(`/spaces/${space.key}/${address}`);
    await expect(nav(page).getByRole('link', { name: new RegExp(`^${entry}`) })).toHaveAttribute(
      'aria-current',
      'page',
    );
  }
  await checkAccessibility(page);
});

test('with automation off, its entry is absent and its address says the part is off', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'No automation');
  await page.route('**/api/v1/system/capabilities', async (route) => {
    const response = await route.fetch();
    const capabilities = (await response.json()) as Record<string, boolean>;
    await route.fulfill({ response, json: { ...capabilities, automation: false } });
  });
  await page.goto(`/spaces/${space.key}/settings`);
  await expect(nav(page).getByRole('link', { name: 'People' })).toBeVisible();
  await expect(nav(page).getByRole('link', { name: /Automation/ })).toHaveCount(0);
  await page.goto(`/spaces/${space.key}/settings/automation`);
  await expect(page.getByText(/experimental part, and it is off/)).toBeVisible();
});

test('every old address opens its new place, and Back does not return to the alias', async ({
  page,
  browser,
}) => {
  const space = await freshOwner(page, browser, 'Old addresses');
  const base = `/spaces/${space.key}/settings`;
  for (const [old, now, title] of [
    ['key', 'details', 'Details'],
    ['categories', 'details', 'Details'],
    ['roles', 'people', 'People'],
    ['people', 'people', 'People'],
    ['rules', 'statuses', 'Statuses and approvals'],
    ['danger', 'archive', 'Archive the space'],
    ['placeholders', 'placeholders', 'Pages still to write'],
    ['links', 'links', 'Details'],
  ] as const) {
    await page.goto(`${base}?section=${old}`);
    await expect(page.getByRole('heading', { name: title, level: 1 })).toBeVisible();
    if (old !== now) await expect(page).toHaveURL(new RegExp(`section=${now}$`));
  }
  await page.goto(`/spaces/${space.key}/overview`);
  await page.goto(`${base}?section=rules`);
  await expect(page).toHaveURL(/section=statuses$/);
  await page.goBack();
  await expect(page).toHaveURL(/\/overview$/);
});

test('Details saves as one form, and Reset changes puts back what is stored', async ({ page, browser }) => {
  const space = await freshOwner(page, browser, 'Detailed space');
  await page.goto(`/spaces/${space.key}/settings?section=details`);
  const save = page.getByRole('button', { name: 'Save', exact: true });
  const reset = page.getByRole('button', { name: 'Reset changes' });
  await expect(save).toBeDisabled();
  await expect(reset).toBeDisabled();
  const name = page.getByLabel('Name', { exact: true });
  await name.fill('Renamed space');
  await expect(save).toBeEnabled();
  await reset.click();
  await expect(name).toHaveValue('Detailed space');
  await expect(save).toBeDisabled();

  // Name and key in one Save: both change, and the form opens at the new key.
  await name.fill('Renamed space');
  const key = unique('K');
  await page.getByRole('textbox', { name: /^Space key/ }).fill(key);
  await checkAccessibility(page);
  await save.click();
  await expect(page).toHaveURL(new RegExp(`/spaces/${key}/settings\\?section=details$`));
  await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Renamed space');
});

test('a refused part says which, keeps what was typed, and saves the parts before it', async ({
  page,
  browser,
}) => {
  const taken = await freshOwner(page, browser, 'Key holder');
  const space = await newSpace(page, 'Owned space');
  await page.goto(`/spaces/${space.key}/settings?section=details`);
  // The last owner cannot be removed: a space keeps at least one (SPC-094).
  await expect(page.getByRole('button', { name: /^Remove .* as an owner$/ })).toHaveCount(0);
  await page.getByLabel('Name', { exact: true }).fill('Still owned');
  await page.getByRole('textbox', { name: /^Space key/ }).fill(taken.key);
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByRole('alert').first()).toContainText('Space key was not saved');
  await expect(page.getByRole('alert').first()).toContainText('Saved: Name and description.');
  await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Still owned');
  await expect(page.getByRole('textbox', { name: /^Space key/ })).toHaveValue(taken.key);
  const stored = (await (await page.request.get(`/api/v1/spaces/${space.id}`)).json()) as { name: string };
  expect(stored.name).toBe('Still owned');
});

test('archiving asks first and then offers Restore; deleting asks for the key', async ({ page, browser }) => {
  const space = await freshOwner(page, browser, 'Archived space');
  await page.goto(`/spaces/${space.key}/settings?section=archive`);
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Archive the space' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Archive the space' }).click();
  await expect(page.getByRole('button', { name: 'Restore the space' })).toBeVisible();
  await page.getByRole('button', { name: 'Restore the space' }).click();
  await expect(page.getByRole('button', { name: 'Archive the space' })).toBeVisible();
  await page.goto(`/spaces/${space.key}/settings?section=delete`);
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Delete the space' }).click();
  await expect(page.getByRole('dialog', { name: 'Delete space' })).toBeVisible();
});

test.describe('who sees which entry (written first)', () => {
  const OLD_AND_NEW = [
    'details',
    'key',
    'features',
    'links',
    'shortcuts',
    'categories',
    'people',
    'roles',
    'statuses',
    'labels',
    'workflows',
    'placeholders',
    'rules',
    'import',
    'export',
    'pdf',
    'danger',
    'archive',
    'delete',
    'picture',
    'layout',
    'files',
    'archived',
  ];

  test('a reader gets "does not exist" at every address, and the screens of the space as before', async ({
    page,
    browser,
  }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Read only space');
    const reader = await holderOf(browser, page, space.id, VIEWER);
    for (const id of OLD_AND_NEW) {
      await reader.goto(`/spaces/${space.key}/settings?section=${id}`);
      await expect(reader.getByRole('alert').filter({ hasText: /not exist|cannot see/ })).toBeVisible();
    }
    await reader.goto(`/spaces/${space.key}/files`);
    await expect(reader.getByRole('heading', { name: 'Files', level: 1 })).toBeVisible();
    await expect(reader.locator('aside.settings-sidebar')).toHaveCount(0);
    await reader.getByRole('button', { name: 'More for this space' }).click();
    await expect(reader.getByRole('menuitem', { name: 'Space settings' })).toHaveCount(0);
    await reader.context().close();
  });

  test('a holder of only the look sees Details with the home page, Layout, Picture and Shortcuts', async ({
    page,
    browser,
  }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Looked after space');
    const looker = await holderOf(browser, page, space.id, ['space.look']);
    await looker.goto(`/spaces/${space.key}/settings`);
    await expect(looker.getByRole('heading', { name: 'Details', level: 1 })).toBeVisible();
    await expect(nav(looker).getByRole('link')).toHaveText([
      'Details',
      'Picture of the space',
      'Shortcuts',
      'Layout',
    ]);
    await expect(looker.getByLabel('Home page')).toBeVisible();
    await expect(looker.getByLabel('Name', { exact: true })).toHaveCount(0);
    await expect(looker.getByLabel('Description')).toHaveCount(0);
    await checkAccessibility(looker);
    // The server keeps refusing what it refused before.
    const refused = await looker.request.patch(`/api/v1/spaces/${space.id}`, { data: { name: 'x' } });
    expect(refused.status()).toBe(403);
    await looker.context().close();
  });

  test('a holder of only the audit sees only Audit of the space', async ({ page, browser }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Audited by one');
    const auditor = await holderOf(browser, page, space.id, ['space.audit']);
    await auditor.goto(`/spaces/${space.key}/audit`);
    await expect(auditor.getByRole('heading', { name: 'Audit of the space', level: 1 })).toBeVisible();
    await expect(nav(auditor).getByRole('link')).toHaveText(['Audit of the space']);
    // Without Details, the settings land on the first entry the person may open (D18).
    await auditor.goto(`/spaces/${space.key}/settings?section=details`);
    await expect(auditor).toHaveURL(new RegExp(`/spaces/${space.key}/audit$`));
    const refused = await auditor.request.get(`/api/v1/spaces/${space.id}/trash`);
    expect(refused.status()).toBe(403);
    await auditor.context().close();
  });
});

test('the sidebar reads right to left and fits a phone', async ({ page, browser }) => {
  const space = await freshOwner(page, browser, 'Narrow space');
  await page.setViewportSize(PHONE);
  await page.goto(`/spaces/${space.key}/settings?section=layout`);
  await page.evaluate(() => document.documentElement.setAttribute('dir', 'rtl'));
  await expect(page.getByRole('heading', { name: 'Layout', level: 1 })).toBeVisible();
  const scrolls = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(scrolls).toBe(false);
  await checkAccessibility(page);
});
