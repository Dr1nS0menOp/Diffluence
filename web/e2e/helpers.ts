// SPDX-License-Identifier: Apache-2.0
import { createHmac } from 'node:crypto';
import { expect, test as base, type APIRequestContext, type Browser, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

export const ADMIN = { email: 'admin@example.com', password: 'correct horse battery staple' };
export const PASSWORD = 'correct horse battery staple';

/** Every test fails on a Content Security Policy violation or a script error. */
export const test = base.extend<{ expectViolations: boolean; watch: void }>({
  expectViolations: [false, { option: true }],
  watch: [
    async ({ page, expectViolations }, use) => {
      const problems: string[] = [];
      page.on('pageerror', (e) => problems.push('script error: ' + e.message));
      page.on('console', (m) => {
        const text = m.text();
        if (m.type() === 'error' && /Content.Security.Policy|Trusted/i.test(text)) problems.push(text);
      });
      await use();
      if (!expectViolations) expect(problems, 'browser problems').toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };

export async function login(page: Page, email: string, password = PASSWORD) {
  await page.goto('/login');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Log in', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();
}

let counter = 0;
export const unique = (prefix: string) => `${prefix}${Date.now().toString(36)}${counter++}`;

async function adminRequest(
  browser: Browser,
): Promise<{ api: APIRequestContext; close: () => Promise<void> }> {
  const context = await browser.newContext({ baseURL: process.env.BASE_URL ?? 'http://localhost:8080' });
  const res = await context.request.post('/api/v1/auth/login', { data: ADMIN });
  expect(res.ok()).toBeTruthy();
  return { api: context.request, close: () => context.close() };
}

/** Creates a user through the admin API and returns the id. */
export async function createUser(browser: Browser, email: string, type = 'member', perms?: string[]) {
  const { api, close } = await adminRequest(browser);
  const res = await api.post('/api/v1/admin/users', {
    data: {
      email,
      display_name: email.split('@')[0],
      type,
      password: PASSWORD,
      global_permissions: perms ?? null,
    },
  });
  expect(res.status(), await res.text()).toBe(201);
  const user = (await res.json()) as { id: string };
  await close();
  return user.id;
}

export async function newSpace(page: Page, name: string) {
  const key = unique('S');
  const res = await page.request.post('/api/v1/spaces', { data: { key, name } });
  expect(res.status(), await res.text()).toBe(201);
  return (await res.json()) as { id: string; key: string; home_page_id: string };
}

export function paragraph(text: string) {
  return { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text }] }] };
}

export async function publishedPage(
  page: Page,
  spaceId: string,
  title: string,
  text: string,
  parent?: string,
) {
  const created = await page.request.post('/api/v1/pages', {
    data: { space_id: spaceId, parent_id: parent ?? null, title },
  });
  expect(created.status(), await created.text()).toBe(201);
  const p = (await created.json()) as { id: string };
  const done = await page.request.post(`/api/v1/pages/${p.id}/publish`, {
    data: { title, document: paragraph(text), base_version: 0 },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  return p.id;
}

export async function grant(
  page: Page,
  spaceId: string,
  subject: { type: string; id: string },
  perms: string[],
) {
  const res = await page.request.put(`/api/v1/spaces/${spaceId}/grants`, {
    data: { subject, permissions: perms },
  });
  expect(res.status(), await res.text()).toBe(204);
}

export const VIEWER = ['space.view', 'space.export'];
export const EDITOR = ['space.view', 'page.create', 'page.edit', 'page.delete_own', 'attachment.add'];

/** Accessibility: WCAG 2.2 AA is the goal (blueprint 01). */
export async function checkAccessibility(page: Page) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze();
  const summary = results.violations.map(
    (v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).join(', ')}`,
  );
  expect(summary).toEqual([]);
}

/** RFC 6238, written out so the test does not share code with the server. */
export function totpCode(secret: string, at = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = '';
  for (const c of secret.replace(/=+$/, '')) bits += alphabet.indexOf(c).toString(2).padStart(5, '0');
  const bytes = bits.match(/.{8}/g)?.map((b) => parseInt(b, 2)) ?? [];
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 1000 / 30)));
  const h = createHmac('sha1', Buffer.from(bytes)).update(counter).digest();
  const o = (h[h.length - 1] ?? 0) & 15;
  const n = (h.readUInt32BE(o) & 0x7fffffff) % 1_000_000;
  return n.toString().padStart(6, '0');
}
