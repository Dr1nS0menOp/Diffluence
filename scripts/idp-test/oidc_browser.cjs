// Logs in with a real browser, through the real Authentik login screens.
const { chromium } = require('/work/web/node_modules/@playwright/test');

async function login(browser, username) {
  const context = await browser.newContext();
  const page = await context.newPage();
  page.on('requestfailed', (r) => console.log('  failed:', r.url().slice(0, 100), r.failure()?.errorText));
  try {
    await page.goto('http://localhost:8080/login');
    await page.getByRole('link', { name: 'Authentik' }).click();
    await page.waitForURL(/ak-server:9000/, { timeout: 15000, waitUntil: 'commit' });
    console.log('  at authentik:', page.url().slice(0, 70));
    const user = page.locator('input[name=uidField]');
    await user.waitFor({ timeout: 30000 });
    await user.fill(username);
    await page.keyboard.press('Enter');
    const password = page.locator('input[name=password]');
    await password.waitFor({ timeout: 30000 });
    await page.waitForTimeout(2000); // the stage re-renders and clears the field once
    await password.fill('authentik-user-password-1');
    await page.getByRole('button', { name: 'Continue' }).click();
    await page.waitForURL(/localhost:8080/, { timeout: 30000, waitUntil: 'commit' });
  } catch (e) {
    console.log('  stopped at:', page.url().slice(0, 120), '|', e.message.split('\n')[0]);
    await page.screenshot({ path: `/t/${username}.png` });
    await context.close();
    return false;
  }
  await page.waitForTimeout(1500);
  const url = page.url();
  const me = await page.request.get('http://localhost:8080/api/v1/me');
  const body = me.status() === 200 ? await me.json() : null;
  const alert = await page
    .getByRole('alert')
    .first()
    .textContent({ timeout: 1000 })
    .catch(() => '');
  console.log(username, '-> ended at', url, '| session:', me.status(), body ? body.user.display_name + ' <' + body.user.email + '>' : '', alert ? '| message: ' + alert : '');
  await context.close();
  return me.status() === 200;
}

(async () => {
  const browser = await chromium.launch();
  const names = process.argv.slice(2);
  let ok = true;
  for (const n of names) ok = (await login(browser, n)) && ok;
  await browser.close();
  process.exit(ok ? 0 : 1);
})().catch((e) => {
  console.error(e.message);
  process.exit(2);
});
