// SPDX-License-Identifier: Apache-2.0
// Insight, rules, webhooks and presence, in a real browser (spec 015).
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  EDITOR,
  expect,
  grant,
  login,
  newSpace,
  PASSWORD,
  publishedPage,
  test,
  unique,
  VIEWER,
} from './helpers';

async function asPerson(browser: Browser, email: string): Promise<Page> {
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const page = await context.newPage();
  await login(page, email, PASSWORD);
  return page;
}

test.describe('rules', () => {
  test('a rule is built by keyboard, tested, and runs', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Rules');
    const target = await publishedPage(page, space.id, 'Plan', 'x');
    await page.goto(`/spaces/${space.key}/settings/automation`);
    await page.getByRole('button', { name: 'New rule' }).click();
    await page.getByRole('button', { name: 'Start from empty' }).click();
    await expect(page.getByRole('heading', { name: 'When', exact: true })).toBeVisible();

    await page.getByLabel('Name').fill('Tag commented pages');
    await page.getByLabel('Trigger').selectOption('page.commented');
    await page.getByRole('textbox', { name: 'Label' }).focus();
    await page.keyboard.type('discussed');
    await page.getByRole('button', { name: 'Save' }).focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Tag commented pages' })).toBeVisible();

    await page.getByLabel('Page id').fill(target);
    await page.getByRole('button', { name: 'Test this rule' }).click();
    await expect(page.getByText('This rule would run.')).toBeVisible();
    const labels = await page.request.get(`/api/v1/pages/${target}`);
    expect(((await labels.json()) as { labels: string[] }).labels).toEqual([]);

    await page.getByLabel('This rule is on').check();
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByLabel('This rule is on')).toBeChecked();
    const commented = await page.request.post(`/api/v1/pages/${target}/comments`, {
      data: {
        body: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello' }] }] },
      },
    });
    expect(commented.ok(), await commented.text()).toBeTruthy();
    await expect
      .poll(
        async () => {
          const res = await page.request.get(`/api/v1/pages/${target}`);
          return ((await res.json()) as { labels: string[] }).labels;
        },
        { timeout: 20_000 },
      )
      .toEqual(['discussed']);
    await page.reload();
    await expect(
      page
        .getByText('Done', { exact: true })
        .or(page.getByText(/: Done/))
        .first(),
    ).toBeVisible();
    await checkAccessibility(page);
  });

  test('a rule with an open problem says what is wrong, and stays off', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Broken');
    await page.goto(`/spaces/${space.key}/settings/automation`);
    await page.getByRole('button', { name: 'New rule' }).click();
    await page.getByRole('button', { name: 'Start from empty' }).click();
    await expect(page.getByText('Give the label.').first()).toBeVisible();
    await page.getByLabel('This rule is on').check();
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText('Fix what the form says before turning this rule on.')).toBeVisible();
  });
});

test.describe('webhooks', () => {
  test('an address on a host nobody allowed is refused', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Hooks');
    await page.goto(`/spaces/${space.key}/settings/webhooks`);
    await page.getByLabel('Address').fill('https://hooks.example.com/in');
    await page.getByLabel('page.labelled').check();
    await page.getByRole('button', { name: 'Add webhook' }).click();
    await expect(page.getByText(/must allow this host first/)).toBeVisible();
    await page.goto('/admin/outgoing');
    await page.getByLabel('Host').fill('hooks.example.com');
    await page.getByRole('button', { name: 'Allow this host' }).click();
    await expect(page.getByRole('button', { name: 'Remove hooks.example.com' })).toBeVisible();
    await page.goto(`/spaces/${space.key}/settings/webhooks`);
    await page.getByLabel('Address').fill('https://hooks.example.com/in');
    await page.getByLabel('page.labelled').check();
    await page.getByRole('button', { name: 'Add webhook' }).click();
    await expect(page.getByText('Copy it now. It shows only this once.')).toBeVisible();
    await page.getByRole('button', { name: 'Close' }).first().click();
    await page.goto('/admin/outgoing');
    await page.getByRole('button', { name: 'Remove hooks.example.com' }).click();
    await expect(page.getByText('The list is empty, so nothing leaves this instance.')).toBeVisible();
  });
});

test.describe('insight', () => {
  test('a table sorts, and says how', async ({ browser, page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const on = await page.request.put('/api/v1/admin/analytics', {
      data: { enabled: true, mode: 'counts', retention_days: 365 },
    });
    expect(on.ok(), await on.text()).toBeTruthy();
    const space = await newSpace(page, 'Insight');
    const email = `${unique('reader')}@example.com`;
    const id = await createUser(browser, email);
    await grant(page, space.id, { type: 'user', id }, VIEWER);
    const first = await publishedPage(page, space.id, 'Alpha', 'x');
    await publishedPage(page, space.id, 'Beta', 'x');
    const reader = await asPerson(browser, email);
    await reader.goto(`/pages/${first}`);
    await expect(reader.getByRole('heading', { name: 'Alpha' })).toBeVisible();
    await expect
      .poll(async () => {
        const res = await page.request.get(`/api/v1/pages/${first}/insights`);
        return ((await res.json()) as { views: number }).views;
      })
      .toBe(1);
    await reader.context().close();

    await page.goto(`/pages/${first}`);
    await expect(page.getByRole('link', { name: '1 view' })).toBeVisible();
    await page.goto(`/spaces/${space.key}/insights`);
    const header = page.getByRole('columnheader', { name: 'Page' });
    await expect(header).toHaveAttribute('aria-sort', 'none');
    await header.getByRole('button').click();
    await expect(header).toHaveAttribute('aria-sort', 'ascending');
    await header.getByRole('button').click();
    await expect(header).toHaveAttribute('aria-sort', 'descending');
    await expect(page.getByRole('row').nth(1)).toContainText('Insight');
    const off = await page.request.put('/api/v1/admin/analytics', {
      data: { enabled: false, mode: 'counts', retention_days: 365 },
    });
    expect(off.ok(), await off.text()).toBeTruthy();
  });

  test('a token with read cannot change a page', async ({ page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Tokens');
    const target = await publishedPage(page, space.id, 'Plan', 'x');
    await page.goto('/me/tokens');
    await page.getByLabel('Name').fill('script');
    await page.getByRole('button', { name: 'New token' }).click();
    const secret = (await page.locator('code').first().textContent()) ?? '';
    expect(secret).toContain('dfl_');
    const headers = { Authorization: `Bearer ${secret}` };
    const anonymous = await page.context().browser()?.newContext({ baseURL: 'http://localhost:8080' });
    const refused = await anonymous?.request.patch(`/api/v1/pages/${target}`, {
      headers,
      data: { title: 'x' },
    });
    expect(refused?.status()).toBe(403);
    await anonymous?.close();
  });
});

test.describe('presence', () => {
  test('people on a page see each other, and the editor warns', async ({ browser, page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Together');
    const email = `${unique('colleague')}@example.com`;
    const id = await createUser(browser, email);
    await grant(page, space.id, { type: 'user', id }, EDITOR);
    const target = await publishedPage(page, space.id, 'Shared', 'x');
    const colleague = await asPerson(browser, email);
    await colleague.goto(`/pages/${target}/edit`);
    await expect(colleague.locator('.prose.editing')).toBeVisible();

    await page.goto(`/pages/${target}`);
    await expect(page.locator('.presence')).toBeVisible();
    await page.goto(`/pages/${target}/edit`);
    await expect(page.locator('.prose.editing')).toBeVisible();
    await expect(page.getByText(/Also editing:/)).toBeVisible({ timeout: 30_000 });
    await colleague.context().close();
  });
});
