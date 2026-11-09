// SPDX-License-Identifier: Apache-2.0
// Spec 043: the Access screen of a space gives roles, not ticks. A combination without a
// name shows as it is and can be named; a change is previewed, and losing the space asks.
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
  const row = as.getByRole('listitem').filter({ hasText: writerName });
  // Space admin is a choice, but only a space administrator may make one.
  await row.getByLabel(`Role of ${writerName}`).selectOption({ label: 'Space admin' });
  await expect(as.getByRole('alert')).toContainText(
    'Only a space administrator can change an administrator.',
  );
  await expect(row.getByLabel(`Role of ${writerName}`).locator('option:checked')).toHaveText('Editor');

  // Taking the space away asks first, and cancelling changes nothing.
  await row.getByRole('button', { name: 'Remove access' }).click();
  const ask = as.getByRole('dialog', { name: 'Someone loses access' });
  await expect(ask).toContainText('Loses: ');
  await ask.getByRole('button', { name: 'Cancel' }).click();
  await expect(row).toBeVisible();
  const held = await page.request.get(roles);
  expect(await held.text()).toContain(writer);

  // A narrower role is saved at once, and says what it took.
  await row.getByLabel(`Role of ${writerName}`).selectOption({ label: 'Viewer' });
  await expect(as.getByRole('status').filter({ hasText: 'Loses: ' })).toBeVisible();
  await expect(row.getByLabel(`Role of ${writerName}`).locator('option:checked')).toHaveText('Viewer');
  await as.context().close();
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
  const row = page.getByRole('listitem').filter({ hasText: name });
  const choice = row.getByLabel(`Role of ${name}`);
  await expect(choice.locator('option:checked')).toHaveText(/^Custom: /);
  await expect(row).toContainText('This combination of permissions matches no named role.');
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
  await expect(choice.locator('option:checked')).toHaveText(roleName);
  await expect(row).not.toContainText('matches no named role');

  // Naming changed no permission.
  expect(await permissionsOf(page, space.id, id)).toEqual(before);
});
