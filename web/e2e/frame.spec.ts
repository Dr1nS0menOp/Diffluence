// SPDX-License-Identifier: Apache-2.0
// Spec 078: the frame. The top bar, the global part of the sidebar and its panels, customising
// one's own sidebar, the block of a space, and the start screen.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  PASSWORD,
  checkAccessibility,
  createUser,
  expect,
  login,
  newSpace,
  publishedPage,
  test,
  unique,
} from './helpers';

const PHONE = { width: 360, height: 740 };

/** A member of their own, so what one test chooses for its sidebar touches no other test. */
async function freshMember(page: Page, perms?: string[]) {
  const email = `${unique('frame')}@example.com`;
  await createUser(page.context().browser()!, email, 'member', perms);
  await login(page, email, PASSWORD);
  return email;
}

const places = (page: Page) => page.getByRole('navigation', { name: 'Your places' });

test('the top bar holds the sidebar button and the name, then search and Create, then the round buttons', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, unique('Bar'));
  await page.goto(`/pages/${space.home_page_id}`);
  await expect(page.getByRole('banner')).toHaveCount(1);
  await expect(page.getByRole('main')).toHaveCount(1);
  await expect(places(page)).toBeVisible();

  const names = await page
    .getByRole('banner')
    .locator('a, button, input')
    .evaluateAll((all) =>
      all
        .filter((el) => (el as HTMLElement).offsetParent !== null)
        .map((el) => el.getAttribute('aria-label') || el.textContent?.trim() || ''),
    );
  const order = ['Hide sidebar', 'Diffluence', 'Search', 'Create', 'Help', 'Administration', 'Account'];
  const places_ = order.map((name) => names.findIndex((n) => n.startsWith(name)));
  expect(
    places_.every((at) => at >= 0),
    names.join(' | '),
  ).toBeTruthy();
  expect([...places_].sort((a, b) => a - b)).toEqual(places_);
  // Home, Spaces, Recent and Favourites are no longer in the bar (NAV-091).
  for (const gone of ['Home', 'Spaces', 'Recent', 'Favourites', 'People']) {
    await expect(page.getByRole('banner').getByRole('link', { name: gone, exact: true })).toHaveCount(0);
    await expect(page.getByRole('banner').getByRole('button', { name: gone, exact: true })).toHaveCount(0);
  }
  // Administration has left the account menu for the settings button (NAV-095).
  await page.getByRole('button', { name: 'Account' }).click();
  await expect(page.getByRole('menuitem', { name: 'Administration' })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await page.getByRole('link', { name: 'Administration' }).click();
  await expect(page).toHaveURL(/\/admin\/summary$/);
});

test('someone who may not use the administration has no settings button', async ({ page }) => {
  await freshMember(page);
  await expect(page.getByRole('banner').getByRole('link', { name: 'Administration' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Account' }).click();
  await expect(page.getByRole('menuitem', { name: 'Administration' })).toHaveCount(0);
});

test('the sidebar button and [ hide and show the sidebar, and the choice outlives a reload', async ({
  page,
}) => {
  await freshMember(page);
  await expect(places(page)).toBeVisible();
  await page.getByRole('button', { name: 'Hide sidebar' }).click();
  await expect(places(page)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Show sidebar' })).toBeFocused();
  await page.reload();
  await expect(page.getByRole('button', { name: 'Show sidebar' })).toBeVisible();
  await expect(places(page)).toHaveCount(0);
  await page.locator('body').press('[');
  await expect(places(page)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Hide sidebar' })).toBeVisible();
  // A screen without a sidebar that hides has no button that would do nothing (D4).
  await page.goto('/search?q=x');
  await expect(page.getByRole('main')).toBeVisible();
  await expect(page.getByRole('button', { name: /sidebar$/ })).toHaveCount(0);
});

for (const entry of ['Recent', 'Favourites', 'Spaces'] as const) {
  test(`the ${entry} panel opens with the focus in its filter and shuts back onto its entry`, async ({
    page,
  }) => {
    await freshMember(page);
    const opener = places(page).getByRole('button', { name: entry, exact: true });
    await expect(opener).toHaveAttribute('aria-expanded', 'false');
    await opener.click();
    const panel = page.getByRole('dialog', { name: entry });
    await expect(panel).toBeVisible();
    await expect(opener).toHaveAttribute('aria-expanded', 'true');
    await expect(panel.getByRole('searchbox', { name: 'Filter' })).toBeFocused();
    await checkAccessibility(page);

    await page.keyboard.press('Escape');
    await expect(panel).toHaveCount(0);
    await expect(opener).toBeFocused();
    await expect(opener).toHaveAttribute('aria-expanded', 'false');

    await page.keyboard.press('Enter');
    await expect(panel).toBeVisible();
    await panel.getByRole('button', { name: 'Close' }).click();
    await expect(panel).toHaveCount(0);
    await expect(opener).toBeFocused();

    // Only one panel is open at a time, and a click outside shuts it.
    await opener.click();
    await page.getByRole('main').click({ position: { x: 600, y: 400 } });
    await expect(panel).toHaveCount(0);
  });
}

test('the Spaces panel lists the current space and favourites, stars them and goes where a row says', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const hereName = unique('Here');
  const likedName = unique('Liked');
  const here = await newSpace(page, hereName);
  const liked = await newSpace(page, likedName);
  expect((await page.request.put(`/api/v1/spaces/${liked.id}/favourite`)).ok()).toBeTruthy();
  await page.goto(`/pages/${here.home_page_id}`);

  await places(page).getByRole('button', { name: 'Spaces', exact: true }).click();
  const panel = page.getByRole('dialog', { name: 'Spaces' });
  const current = panel.getByRole('region', { name: 'This space' });
  await expect(current.getByRole('link', { name: hereName })).toBeVisible();
  const favourites = panel.getByRole('region', { name: 'Favourites' });
  await expect(favourites.getByRole('link', { name: likedName })).toBeVisible();
  await expect(panel.getByRole('link', { name: 'All spaces' })).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Create a space' })).toBeVisible();

  await current.getByRole('button', { name: `Add ${hereName} to favourites` }).click();
  await expect(favourites.getByRole('link', { name: hereName })).toBeVisible();
  await favourites.getByRole('button', { name: `Remove ${hereName} from favourites` }).click();
  await expect(favourites.getByRole('link', { name: hereName })).toHaveCount(0);

  await panel.getByRole('searchbox', { name: 'Filter' }).fill(likedName.slice(0, 6));
  await expect(current).toHaveCount(0);
  await favourites.getByRole('link', { name: likedName }).click();
  await expect(panel).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/pages/${liked.home_page_id}$`));
});

test('an archived space still shows as the current space in the Spaces panel', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const name = unique('Old shelf');
  const space = await newSpace(page, name);
  expect((await page.request.post(`/api/v1/spaces/${space.id}/archive`, { data: {} })).status()).toBe(204);
  await page.goto(`/pages/${space.home_page_id}`);
  await places(page).getByRole('button', { name: 'Spaces', exact: true }).click();
  const current = page.getByRole('dialog', { name: 'Spaces' }).getByRole('region', { name: 'This space' });
  await expect(current.getByRole('link', { name, exact: true })).toBeVisible();
});

test('the Recent panel holds visited pages and drafts, and opens its full screen', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, unique('Lately'));
  const title = unique('Seen lately');
  const seen = await publishedPage(page, space.id, title, 'x');
  await page.goto(`/pages/${seen}`);
  await expect(page.getByRole('heading', { name: title, level: 1 })).toBeVisible();
  await places(page).getByRole('button', { name: 'Recent', exact: true }).click();
  const panel = page.getByRole('dialog', { name: 'Recent' });
  await expect(
    panel.getByRole('region', { name: 'Visited' }).getByRole('link', { name: title }),
  ).toBeVisible();
  await expect(panel.getByRole('region', { name: 'Drafts' })).toBeVisible();
  await panel.getByRole('searchbox', { name: 'Filter' }).fill('nothing like this');
  await expect(panel.getByRole('link', { name: title })).toHaveCount(0);
  // The panel lies over the main area, so its foot takes the click and not what lies below it.
  await panel.getByRole('link', { name: 'All recent pages' }).click({ timeout: 5000 });
  await expect(page).toHaveURL(/\/recent$/);
  await expect(panel).toHaveCount(0);
});

test('Theme in the account menu is a sub-menu that works from the keyboard', async ({ page }) => {
  await freshMember(page);
  const account = page.getByRole('button', { name: 'Account' });
  await account.click();
  // The first entry has the focus when the menu opens (NAV-096).
  await expect(page.getByRole('menuitem', { name: 'Profile' })).toBeFocused();
  await checkAccessibility(page);
  const theme = page.getByRole('menuitem', { name: 'Theme' });
  await theme.focus();
  await page.keyboard.press('ArrowRight');
  const choices = page.getByRole('menu', { name: 'Theme' });
  await expect(choices.getByRole('menuitemradio', { name: 'Light' })).toBeFocused();
  await expect(choices.getByRole('menuitemradio', { name: 'Same as my system' })).toHaveAttribute(
    'aria-checked',
    'true',
  );
  await page.keyboard.press('ArrowLeft');
  await expect(choices).toHaveCount(0);
  await expect(theme).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowDown');
  await expect(choices.getByRole('menuitemradio', { name: 'Dark' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect
    .poll(async () => (await (await page.request.get('/api/v1/me/preferences')).json()).theme)
    .toBe('dark');
  await account.click();
  await page.getByRole('menuitem', { name: 'Theme' }).click();
  await expect(page.getByRole('menuitemradio', { name: 'Dark' })).toHaveAttribute('aria-checked', 'true');
});

test('customising the sidebar hides an entry under More and moves one by keyboard; Cancel keeps all', async ({
  page,
}) => {
  await freshMember(page);
  const nav = places(page);
  // Watching starts under More (NAV-142).
  await expect(nav.getByRole('link', { name: 'Watching' })).toHaveCount(0);
  await nav.getByRole('button', { name: 'More', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: 'Watching' })).toBeVisible();
  await page.getByRole('menuitem', { name: 'Customise sidebar' }).click();

  const dialog = page.getByRole('dialog', { name: 'Customise sidebar' });
  await expect(dialog.getByRole('checkbox', { name: 'Home' })).toBeDisabled();
  await expect(dialog.getByRole('checkbox', { name: 'Home' })).toBeChecked();
  await checkAccessibility(page);
  await dialog.getByRole('checkbox', { name: 'Tasks' }).uncheck();
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(nav.getByRole('link', { name: 'Tasks' })).toBeVisible();

  await nav.getByRole('button', { name: 'More', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Customise sidebar' }).click();
  await dialog.getByRole('checkbox', { name: 'Tasks' }).uncheck();
  await dialog.getByRole('checkbox', { name: 'Watching' }).check();
  // People moves above Tasks and then above Drafts, by keyboard (NAV-140).
  const people = dialog.getByRole('button', { name: 'More for People' });
  for (let step = 0; step < 2; step++) {
    await people.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('menuitem', { name: 'Move up' })).toBeFocused();
    await page.keyboard.press('Enter');
  }
  await dialog.getByRole('button', { name: 'Save changes' }).click();
  await expect(dialog).toHaveCount(0);

  await expect(nav.locator('.side-link')).toHaveText([
    'Home',
    'Recent',
    'Favourites',
    'Spaces',
    'People',
    'Drafts',
    'Watching',
    'More',
  ]);
  await nav.getByRole('button', { name: 'More', exact: true }).click();
  await expect(page.getByRole('menuitem', { name: 'Tasks' })).toBeVisible();
  // The choice belongs to the account, so it follows the person to another device.
  const stored = await (await page.request.get('/api/v1/me/preferences')).json();
  expect(stored.sidebar_hidden).toEqual(['tasks']);
});

test('the block of a space: its header row leads home, its menu holds the tools, and Back to top', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const name = unique('Block');
  const space = await newSpace(page, name);
  for (let i = 0; i < 40; i++) await publishedPage(page, space.id, `Row ${String(i).padStart(2, '0')}`, 'x');
  await page.goto(`/pages/${space.home_page_id}`);

  const head = page.locator('#space-head-link');
  await expect(head).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('region', { name: 'Content' })).toBeVisible();
  await expect(page.getByLabel('Filter pages')).toBeVisible();
  // The strip of spaces and the fixed Home row have left the sidebar (NAV-135, SPC-106).
  await expect(page.getByRole('navigation', { name: 'Your spaces' })).toHaveCount(0);
  await expect(page.getByRole('navigation', { name: 'Space links' })).toHaveCount(0);

  await page.getByRole('button', { name: 'More for this space' }).click();
  const tools = page.getByRole('group', { name: 'Space tools' });
  for (const tool of ['Overview', 'People', 'Files', 'Archived pages']) {
    await expect(tools.getByRole('menuitem', { name: tool })).toBeVisible();
  }
  await expect(page.getByRole('menuitem', { name: 'Space settings' })).toBeVisible();
  await expect(page.getByRole('menuitem', { name: 'Archive space' })).toBeVisible();
  await expect(page.getByRole('menuitem', { name: 'Delete space' })).toBeVisible();
  await checkAccessibility(page);
  await page.getByRole('menuitem', { name: 'Archive space' }).click();
  await page.getByRole('dialog', { name: 'Archive space' }).getByRole('button', { name: 'Cancel' }).click();
  await expect(page.getByText('This space is archived and read-only.')).toHaveCount(0);

  const sidebar = page.locator('aside.sidebar');
  await expect(page.getByRole('button', { name: 'Back to top' })).toHaveCount(0);
  await sidebar.evaluate((el) => el.scrollTo({ top: el.scrollHeight }));
  const back = page.getByRole('button', { name: 'Back to top' });
  await expect(back).toBeVisible();
  await back.click();
  await expect(head).toBeFocused();
  await expect.poll(() => sidebar.evaluate((el) => el.scrollTop)).toBe(0);
  await expect(back).toHaveCount(0);

  await page.getByRole('link', { name: 'Row 00' }).click();
  await expect(head).not.toHaveAttribute('aria-current', /./);
  await head.click();
  await expect(page).toHaveURL(new RegExp(`/pages/${space.home_page_id}$`));
});

test('the add button of Content and the order and view menu replace the row of tree tools', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, unique('Tools'));
  await publishedPage(page, space.id, 'Banana', 'x');
  await publishedPage(page, space.id, 'Apple', 'x');
  await page.goto(`/pages/${space.home_page_id}`);
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await expect(tree.locator('a[data-row]')).toHaveText(['Banana', 'Apple']);

  await page.getByRole('button', { name: 'Order and view' }).click();
  await page.getByRole('menuitemradio', { name: 'A to Z' }).click();
  await expect(tree.locator('a[data-row]')).toHaveText(['Apple', 'Banana']);
  await page.getByRole('button', { name: 'Order and view' }).click();
  await expect(page.getByRole('menuitemradio', { name: 'A to Z' })).toHaveAttribute('aria-checked', 'true');
  await page.getByRole('menuitemradio', { name: 'Tree' }).click();

  await page.getByRole('button', { name: 'Create in this space' }).click();
  await page.getByRole('menuitem', { name: 'New page' }).click();
  await expect(page.getByRole('dialog', { name: 'New page' })).toBeVisible();
});

test('the start screen has no greeting and no grid of spaces, but the cards of what you touched', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, unique('Start'));
  const title = unique('Touched');
  const id = await publishedPage(page, space.id, title, 'x');
  await page.goto(`/pages/${id}`);
  await expect(page.getByRole('heading', { name: title, level: 1 })).toBeVisible();
  await page.goto('/');
  const cards = page.getByRole('region', { name: 'Pick up where you left off' });
  await expect(cards.getByRole('link', { name: new RegExp(title) })).toBeVisible();
  await expect(page.getByRole('heading', { name: /Welcome back/ })).toHaveCount(0);
  await expect(page.getByRole('complementary', { name: 'Your pages and tasks' })).toHaveCount(0);
  const pills = page.getByRole('group', { name: 'What the feed shows' });
  await expect(pills.getByRole('button', { name: 'For you' })).toHaveAttribute('aria-pressed', 'true');
  await pills.getByRole('button', { name: 'Recent activity' }).click();
  await expect(pills.getByRole('button', { name: 'Recent activity' })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(pills.getByRole('button', { name: 'For you' })).toHaveAttribute('aria-pressed', 'false');
  await checkAccessibility(page);
});

test('on a phone, and right to left, the sidebar and a panel stay inside the window', async ({ browser }) => {
  const context = await browser.newContext({ viewport: PHONE });
  const page = await context.newPage();
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, unique('Phone'));
  await page.goto(`/pages/${space.home_page_id}`);
  // A narrow screen starts without the sidebar (NAV-072).
  await expect(places(page)).toHaveCount(0);
  await page.getByRole('button', { name: 'Show sidebar' }).click();
  await places(page).getByRole('button', { name: 'Spaces', exact: true }).click();
  const panel = page.getByRole('dialog', { name: 'Spaces' });
  await expect(panel).toBeVisible();
  const box = await panel.boundingBox();
  expect(box && box.x >= 0 && box.x + box.width <= PHONE.width).toBeTruthy();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  await checkAccessibility(page);
  await page.keyboard.press('Escape');

  await page.setViewportSize({ width: 1280, height: 800 });
  await page.evaluate(() => document.documentElement.setAttribute('dir', 'rtl'));
  await places(page).getByRole('button', { name: 'Spaces', exact: true }).click();
  const sidebar = await page.locator('aside.sidebar').boundingBox();
  const mirrored = await panel.boundingBox();
  // The sidebar stands on the right, and the panel opens beside it toward the middle.
  expect(sidebar && mirrored && mirrored.x + mirrored.width <= sidebar.x + 1).toBeTruthy();
  await context.close();
});
