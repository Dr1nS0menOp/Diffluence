// SPDX-License-Identifier: Apache-2.0
// Specs 043 and 049: the Access screen of a space gives roles, not ticks, and someone may
// hold several. A combination without a name has a short name, opens to show what it gives
// and can be named; a change is previewed, and losing the space asks.
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createNamedUser,
  EDITOR,
  expect,
  grant,
  login,
  newSpace,
  PASSWORD,
  test,
  unique,
} from './helpers';

async function builtInRole(page: Page, key: string): Promise<string> {
  const res = await page.request.get('/api/v1/roles?scope=space');
  const { roles } = (await res.json()) as { roles: { id: string; built_in?: string }[] };
  const role = roles.find((r) => r.built_in === key);
  expect(role, `the built-in role ${key}`).toBeDefined();
  return role?.id ?? '';
}

/** What a person holds in a space, sorted. */
async function permissionsOf(page: Page, spaceId: string, userId: string): Promise<string[]> {
  const res = await page.request.get(`/api/v1/spaces/${spaceId}/grants`);
  const { grants } = (await res.json()) as { grants: { subject: { id: string }; permissions: string[] }[] };
  const held = grants.find((g) => g.subject.id === userId)?.permissions ?? [];
  expect(held.length, 'the person holds permissions').toBeGreaterThan(0);
  return [...held].sort();
}

async function pageAs(browser: Browser, email: string): Promise<Page> {
  const page = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
  await login(page, email, PASSWORD);
  return page;
}

test('a manager cannot make an administrator, and taking access away asks first', async ({
  page,
  browser,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Managed by roles');
  const managerName = unique('manager');
  const managerMail = `${managerName}@example.com`;
  const manager = await createNamedUser(browser, managerMail, managerName);
  const writerName = unique('writer');
  const writer = await createNamedUser(browser, `${writerName}@example.com`, writerName);
  const roles = `/api/v1/spaces/${space.id}/role-assignments`;
  for (const [id, key] of [
    [manager, 'manager'],
    [writer, 'editor'],
  ] as const) {
    const res = await page.request.put(roles, {
      data: { subject: { type: 'user', id }, roles: [await builtInRole(page, key)] },
    });
    expect(res.status(), await res.text()).toBe(204);
  }

  const as = await pageAs(browser, managerMail);
  await as.goto(`/spaces/${space.key}/access`);
  await checkAccessibility(as);
  const row = as.locator('.member-row').filter({ hasText: writerName });
  const held = row.getByRole('list', { name: `Roles of ${writerName}` });
  // Space admin is a choice, but only a space administrator may give it.
  await row.getByLabel(`Add a role for ${writerName}`).selectOption({ label: 'Space admin' });
  await expect(as.getByRole('alert')).toContainText(
    'Only a space administrator can change an administrator.',
  );
  await expect(held.locator('summary')).toHaveText(['Editor']);

  // Taking the space away asks first, and cancelling changes nothing.
  await row.getByRole('button', { name: 'Remove access' }).click();
  const ask = as.getByRole('dialog', { name: 'Someone loses access' });
  await expect(ask).toContainText('Loses: ');
  await ask.getByRole('button', { name: 'Cancel' }).click();
  await expect(row).toBeVisible();
  expect(await (await page.request.get(roles)).text()).toContain(writer);

  // Spec 049: a second role is added beside the first, and one is taken away at a time.
  await row.getByLabel(`Add a role for ${writerName}`).selectOption({ label: 'Viewer' });
  await expect(held.locator('summary')).toHaveText(['Editor', 'Viewer']);
  await row.getByRole('button', { name: `Take Editor away from ${writerName}` }).click();
  await expect(as.getByRole('status').filter({ hasText: 'Loses: ' })).toBeVisible();
  await expect(held.locator('summary')).toHaveText(['Viewer']);
  await as.context().close();
});

test('someone holds several roles, and each opens to show what it gives', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Several roles');
  const name = unique('helper');
  const id = await createNamedUser(browser, `${name}@example.com`, name);
  await page.goto(`/spaces/${space.key}/access`);
  await page.getByPlaceholder('Give a role to').fill(name);
  await page.getByRole('option', { name: new RegExp(`${name}@example.com`) }).click();
  await page.getByLabel(`Role to give ${name}`).selectOption({ label: 'Viewer' });
  await page.getByRole('button', { name: 'Give access' }).click();
  const row = page.locator('.member-row').filter({ hasText: name });
  await expect(row.locator('summary')).toHaveText(['Viewer']);

  // Giving access again adds a role and keeps the one held.
  await page.getByPlaceholder('Give a role to').fill(name);
  await page.getByRole('option', { name: new RegExp(`${name}@example.com`) }).click();
  await page.getByLabel(`Role to give ${name}`).selectOption({ label: 'Editor' });
  await page.getByRole('button', { name: 'Give access' }).click();
  // The roles a person holds are listed by name.
  await expect(row.locator('summary')).toHaveText(['Editor', 'Viewer']);
  const assigned = await (await page.request.get(`/api/v1/spaces/${space.id}/role-assignments`)).json();
  const mine = (
    assigned as { assignments: { subject: { id: string }; roles: unknown[] }[] }
  ).assignments.find((a) => a.subject.id === id);
  expect(mine?.roles.length).toBe(2);

  await row.locator('summary', { hasText: 'Viewer' }).click();
  await expect(row.locator('details[open]')).toContainText('View the space');
  await checkAccessibility(page);
});

test('a combination without a name shows as it is, and an administrator names it', async ({
  page,
  browser,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Unnamed combinations');
  const name = unique('author');
  const id = await createNamedUser(browser, `${name}@example.com`, name);
  // The older route for single permissions makes a role named after them.
  await grant(page, space.id, { type: 'user', id }, EDITOR);
  const before = await permissionsOf(page, space.id, id);
  await page.goto(`/spaces/${space.key}/access`);
  const row = page.locator('.member-row').filter({ hasText: name });
  // Spec 049: a short name, never cut off, that says how many permissions it holds.
  const role = row.locator('summary');
  await expect(role).toHaveText([/^Custom combination \d+ \(\d+ permissions\)$/]);
  await role.click();
  await expect(row).toContainText('This combination of permissions matches no named role.');
  await expect(row.locator('details[open]')).toContainText('Edit pages');
  await checkAccessibility(page);

  await row.getByRole('button', { name: 'Name this role' }).click();
  const dialog = page.getByRole('dialog', { name: 'Name this role' });
  // A name another role has is refused, and nothing changes.
  await dialog.getByLabel('Name', { exact: true }).fill('Viewer');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(dialog.getByRole('alert')).toContainText('Another role already has this name.');
  const roleName = unique('Authors');
  await dialog.getByLabel('Name', { exact: true }).fill(roleName);
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(dialog).toBeHidden();
  await expect(role).toHaveText([roleName]);
  await expect(row).not.toContainText('matches no named role');

  // Naming changed no permission.
  expect(await permissionsOf(page, space.id, id)).toEqual(before);
});
