// SPDX-License-Identifier: Apache-2.0
import {
  ADMIN,
  checkAccessibility,
  createUser,
  EDITOR,
  expect,
  grant,
  login,
  newSpace,
  publishedPage,
  test,
  totpCode,
  unique,
  VIEWER,
} from './helpers';

test('the login page is accessible and refuses a wrong password', async ({ page }) => {
  await page.goto('/login');
  await checkAccessibility(page);
  await page.getByLabel('Email').fill(ADMIN.email);
  await page.getByLabel('Password').fill('wrong password here');
  await page.getByRole('button', { name: 'Log in', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveText('Email or password is incorrect.');
});

test('anyone logged out is sent to the login page', async ({ page }) => {
  await page.goto('/admin/users');
  await expect(page).toHaveURL(/\/login$/);
});

test('a guest cannot find a restricted page by any route', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const email = `${unique('guest')}@example.com`;
  const guestId = await createUser(browser, email, 'guest');
  const space = await newSpace(page, 'Restricted space');
  await grant(page, space.id, { type: 'user', id: guestId }, VIEWER);
  const word = unique('kumquat');
  const open = await publishedPage(page, space.id, 'Open page', `open ${word}`);
  const secret = await publishedPage(page, space.id, 'Secret page', `secret ${word}`);
  const below = await publishedPage(page, space.id, 'Below secret', `below ${word}`, secret);

  // Set the restriction through the screen, as a space admin would.
  await page.goto(`/pages/${secret}`);
  await page.getByRole('button', { name: 'Restrictions' }).click();
  const dialog = page.getByRole('dialog', { name: 'Page restrictions' });
  await dialog.getByPlaceholder('Add').first().fill('admin');
  await dialog.getByRole('button', { name: /admin@example.com/ }).click();
  await expect(dialog.getByRole('listitem').filter({ hasText: 'admin' }).first()).toBeVisible();
  await expect(dialog).toHaveCount(1);
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Close' }).click();
  await expect(page.getByText('Restricted', { exact: true })).toBeVisible();

  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const guest = await context.newPage();
  await login(guest, email);
  await guest.goto(`/pages/${open}`);
  await expect(guest.getByRole('heading', { name: 'Open page', level: 1 })).toBeVisible();
  const tree = guest.getByRole('navigation', { name: 'Pages' });
  await expect(tree.getByRole('link', { name: 'Open page' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Secret page' })).toHaveCount(0);

  for (const id of [secret, below]) {
    await guest.goto(`/pages/${id}`);
    await expect(guest.getByRole('alert')).toContainText('does not exist, or you cannot see it');
    await expect(guest.getByText('Secret page')).toHaveCount(0);
  }

  await guest.goto(`/search?q=${word}`);
  await expect(guest.getByRole('link', { name: 'Open page' })).toBeVisible();
  await expect(guest.getByRole('link', { name: 'Secret page' })).toHaveCount(0);
  await expect(guest.getByRole('link', { name: 'Below secret' })).toHaveCount(0);

  // A guest has no view of the people in this installation.
  const directory = await guest.request.get('/api/v1/directory?q=admin');
  expect(directory.status()).toBe(403);
  await context.close();

  // "Why can't they?" gives the reason.
  await page.getByRole('button', { name: 'Restrictions' }).click();
  await page
    .getByRole('dialog')
    .getByPlaceholder('Check a person')
    .fill(email.split('@')[0] ?? '');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: new RegExp(email) })
    .click();
  await expect(page.getByRole('dialog').getByRole('status')).toHaveText('Denied');
  await expect(page.getByRole('dialog').getByRole('cell', { name: 'Failed' })).toBeVisible();
});

test('a viewer sees no edit controls, and an editor does', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const viewerEmail = `${unique('viewer')}@example.com`;
  const editorEmail = `${unique('editor')}@example.com`;
  const viewerId = await createUser(browser, viewerEmail);
  const editorId = await createUser(browser, editorEmail);
  const space = await newSpace(page, 'Roles space');
  await grant(page, space.id, { type: 'user', id: viewerId }, VIEWER);
  await grant(page, space.id, { type: 'user', id: editorId }, EDITOR);
  const id = await publishedPage(page, space.id, 'Roles', 'x');

  const v = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
  await login(v, viewerEmail);
  await v.goto(`/pages/${id}`);
  await expect(v.getByRole('heading', { name: 'Roles', level: 1 })).toBeVisible();
  await expect(v.getByRole('link', { name: 'Edit', exact: true })).toHaveCount(0);
  await expect(v.getByRole('button', { name: 'New page' })).toHaveCount(0);
  await expect(v.getByRole('link', { name: 'Space settings' })).toHaveCount(0);

  const e = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
  await login(e, editorEmail);
  await e.goto(`/pages/${id}`);
  await expect(e.getByRole('link', { name: 'Edit', exact: true })).toBeVisible();
  await expect(e.getByRole('button', { name: 'New page' })).toBeVisible();
});

test('space settings: details, roles, archive', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Settings space');
  await page.goto(`/spaces/${space.key}/settings`);
  await checkAccessibility(page);
  await page.getByRole('button', { name: /Team space/ }).click();
  await expect(page.getByRole('row', { name: /All members/ })).toBeVisible();
  await page.getByRole('button', { name: 'Archive' }).click();
  await expect(page.getByText('This space is archived and read-only.')).toBeVisible();
  await page.goto(`/pages/${space.home_page_id}`);
  await expect(page.getByRole('link', { name: 'Edit', exact: true })).toHaveCount(0);
  await page.goto(`/spaces/${space.key}/settings`);
  await page.getByRole('button', { name: 'Unarchive' }).click();
  await expect(page.getByText('This space is archived and read-only.')).toHaveCount(0);
});

test('an authenticator app protects the login', async ({ page, browser }) => {
  const email = `${unique('totp')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await page.goto('/me');
  await checkAccessibility(page);

  const section = page.getByRole('heading', { name: 'Authenticator app' }).locator('..');
  await section.getByLabel('Current password').fill('correct horse battery staple');
  await section.getByRole('button', { name: 'Set up' }).click();
  const secret = (await page.getByTestId('totp-secret').textContent()) ?? '';
  await expect(section.getByRole('img')).toBeVisible();
  await section.getByLabel('Code').fill(totpCode(secret));
  await section.getByRole('button', { name: 'Confirm' }).click();
  await expect(page.getByRole('dialog', { name: 'Save your recovery codes' }).locator('code')).toHaveCount(
    10,
  );

  const fresh = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
  await fresh.goto('/login');
  await fresh.getByLabel('Email').fill(email);
  await fresh.getByLabel('Password').fill('correct horse battery staple');
  await fresh.getByRole('button', { name: 'Log in', exact: true }).click();
  const code = fresh.getByLabel(/authenticator app/);
  await code.fill('000000');
  await fresh.getByRole('button', { name: 'Continue' }).click();
  await expect(fresh.getByRole('alert')).toHaveText('That code is not correct.');
  await code.fill(totpCode(secret, Date.now() + 30_000));
  await fresh.getByRole('button', { name: 'Continue' }).click();
  await expect(fresh.getByRole('heading', { name: 'Spaces' })).toBeVisible();
});

test('a passkey logs in without a password', async ({ page, browser }) => {
  const email = `${unique('passkey')}@example.com`;
  await createUser(browser, email);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });

  await login(page, email);
  await page.goto('/me');
  await page.getByLabel('Name for this passkey').fill('Test key');
  await page.getByRole('button', { name: 'Add a passkey' }).click();
  await expect(page.getByRole('listitem').filter({ hasText: 'Test key' })).toBeVisible();

  await page.getByRole('button', { name: 'Log out' }).first().click();
  await expect(page).toHaveURL(/\/login$/);
  await page.getByRole('button', { name: 'Log in with a passkey' }).click();
  await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();
  const me = await page.request.get('/api/v1/me');
  expect(await me.text()).toContain(email);
});

test('every tab of the admin console works and is accessible', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  for (const [tab, marker] of [
    ['users', 'New user'],
    ['groups', 'Create'],
    ['spaces', 'Key'],
    ['audit', 'Verify the chain'],
    ['settings', 'Keep deleted items for'],
    ['scim', 'SCIM base URL'],
    ['system', 'Nodes'],
  ] as const) {
    await page.goto(`/admin/${tab}`);
    await expect(page.getByText(marker).first()).toBeVisible();
    await checkAccessibility(page);
  }

  await page.goto('/admin/audit');
  await page.getByRole('button', { name: 'Verify the chain' }).click();
  await expect(page.getByRole('status')).toContainText('The audit chain is intact');

  await page.goto('/admin/scim');
  await page.getByLabel('Token name').fill(unique('token'));
  await page.getByRole('button', { name: 'New token' }).click();
  await expect(page.getByRole('dialog').locator('code')).toContainText('dfl_scim_');

  await page.goto('/admin/system');
  await expect(page.getByRole('row', { name: /Ready/ })).toContainText('ready');
});

test('a normal user has no way into the admin console', async ({ page, browser }) => {
  const email = `${unique('plain')}@example.com`;
  await createUser(browser, email);
  await login(page, email);
  await expect(page.getByRole('link', { name: 'Administration' })).toHaveCount(0);
  await page.goto('/admin/users');
  await expect(page.getByRole('alert')).toBeVisible();
  const res = await page.request.get('/api/v1/admin/users');
  expect(res.status()).toBe(403);
});

test.describe('the policy itself', () => {
  test.use({ expectViolations: true });

  test('the security headers reach the browser', async ({ page }) => {
    const res = await page.goto('/login');
    const policy = res?.headers()['content-security-policy'] ?? '';
    expect(policy).toContain("script-src 'self'");
    expect(policy).toContain("require-trusted-types-for 'script'");
    // The policy is really enforced: an inline script cannot run.
    const ran = await page
      .evaluate(() => {
        const s = document.createElement('script');
        s.textContent = 'window.__ran = true';
        document.body.appendChild(s);
        return (window as unknown as { __ran?: boolean }).__ran === true;
      })
      .catch(() => false);
    expect(ran).toBe(false);
  });
});
