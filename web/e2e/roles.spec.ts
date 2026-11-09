// SPDX-License-Identifier: Apache-2.0
// Spec 036: an administrator makes a role on the roles screen, a space gives it, and a
// change reaches people only after a preview.
import { expect } from '@playwright/test';
import { ADMIN, createNamedUser, login, newSpace, PASSWORD, test, unique } from './helpers';

test('a role is made, given in a space, and changed after a preview', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const roleName = unique('Reviewer');
  const person = unique('reviewer');
  const email = `${person}@example.com`;
  await createNamedUser(browser, email, person);
  const space = await newSpace(page, 'Roles space');

  // Make the role.
  await page.goto('/admin/permissions');
  await expect(page.getByRole('heading', { name: 'Roles for a space' })).toBeVisible();
  await page.getByRole('button', { name: 'New role' }).first().click();
  const make = page.getByRole('dialog');
  await make.getByLabel('Name').fill(roleName);
  await make.getByRole('checkbox', { name: 'View the space' }).check();
  await make.getByRole('checkbox', { name: 'Add comments' }).check();
  await make.getByRole('button', { name: 'Save' }).click();
  await expect(make).toBeHidden();
  const row = page.getByRole('row').filter({ hasText: roleName });
  await expect(row).toContainText('0 holders');

  // Give it to a person in the space, from the space's settings.
  await page.goto(`/spaces/${space.key}/settings?section=roles`);
  await expect(page.getByRole('heading', { name: 'People and roles', level: 1 })).toBeVisible();
  await page.getByPlaceholder('Give a role to').fill(person);
  await page.getByRole('option', { name: new RegExp(email) }).click();
  const give = page.getByRole('dialog', { name: `Roles of ${person}` });
  await give.getByRole('checkbox', { name: roleName }).check();
  await give.getByRole('button', { name: 'Save' }).click();
  await expect(give).toBeHidden();
  await expect(page.getByRole('listitem').filter({ hasText: person })).toContainText(roleName);

  // The person can now open the space.
  const theirs = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const them = await theirs.newPage();
  await login(them, email, PASSWORD);
  expect((await them.request.get(`/api/v1/spaces/${space.id}`)).status()).toBe(200);

  // In use, the role cannot be deleted, and a change shows who it reaches first.
  await page.goto('/admin/permissions');
  await expect(row).toContainText('1 holder');
  await expect(row.getByRole('button', { name: 'Delete' })).toHaveCount(0);
  await row.getByRole('button', { name: 'Edit' }).click();
  const change = page.getByRole('dialog');
  // The editor says who holds the role, and where.
  await expect(change.getByRole('region', { name: 'Who has this role' })).toContainText(person);
  await change.getByRole('checkbox', { name: 'Create pages' }).check();
  await change.getByRole('button', { name: 'Save' }).click();
  await expect(change.getByRole('status')).toContainText('This role is in use');
  await expect(change.getByRole('status')).toContainText('Create pages');
  await change.getByRole('button', { name: 'Change it for all of them' }).click();
  await expect(change).toBeHidden();
  await expect(row).toContainText('Create pages');

  // The change reached the person at once.
  const created = await them.request.post('/api/v1/pages', {
    data: { space_id: space.id, title: unique('By role') },
  });
  expect(created.status(), await created.text()).toBe(201);
  await theirs.close();
});
