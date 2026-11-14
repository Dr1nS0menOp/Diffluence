// SPDX-License-Identifier: Apache-2.0
import { createHmac } from 'node:crypto';
import { readFileSync } from 'node:fs';
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
  // People with only a password are asked for more. Tests on other things put that off.
  await page.request.post('/api/v1/me/sign-in-prompt/later');
}

/**
 * Waits until the editor really has the focus. TipTap gives it back in the next
 * frame, and ProseMirror puts the cursor back 20 ms after focus. No person can type
 * that fast, but a test can: on a slow machine its keys then land somewhere else.
 * This timer starts after that one and lasts longer, so it always ends later.
 */
export async function editorSettled(page: Page) {
  await expect(page.locator('.prose.editing')).toBeFocused();
  await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => setTimeout(done, 40))));
}

let counter = 0;
export const unique = (prefix: string) => `${prefix}${Date.now().toString(36)}${counter++}`;

export async function adminRequest(
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

/**
 * A system administrator who just logged in and has recovery codes, for a change that lets more
 * through than before, such as putting a test's setting back (spec 076).
 */
export async function confirmingAdministrator(
  browser: Browser,
): Promise<{ api: APIRequestContext; codes: string[]; close: () => Promise<void> }> {
  const email = `${unique('keeper')}@example.com`;
  await createUser(browser, email, 'member', ['use_product', 'administrator', 'system_administrator']);
  const context = await browser.newContext({ baseURL: process.env.BASE_URL ?? 'http://localhost:8080' });
  const res = await context.request.post('/api/v1/auth/login', { data: { email, password: PASSWORD } });
  expect(res.ok(), await res.text()).toBeTruthy();
  const setup = await context.request.post('/api/v1/me/totp/begin', { data: { password: PASSWORD } });
  const { secret } = (await setup.json()) as { secret: string };
  const confirm = await context.request.post('/api/v1/me/totp/confirm', { data: { code: totpCode(secret) } });
  expect(confirm.ok(), await confirm.text()).toBeTruthy();
  const { recovery_codes: codes } = (await confirm.json()) as { recovery_codes: string[] };
  return { api: context.request, codes, close: () => context.close() };
}

/** A user with a real name, for screens where the name shows. */
export async function createNamedUser(browser: Browser, email: string, displayName: string) {
  const { api, close } = await adminRequest(browser);
  const res = await api.post('/api/v1/admin/users', {
    data: { email, display_name: displayName, type: 'member', password: PASSWORD, global_permissions: null },
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

const WOFF_HEADER_BYTES = 44;
const WOFF_ENTRY_BYTES = 20;
const SFNT_ENTRY_BYTES = 16;
const SFNT_HEADER_BYTES = 12;
const padded = (length: number) => (length + 3) & ~3;

/** A real web font from a font in the test image: WOFF may store its tables uncompressed. */
export function realFont(path = '/usr/share/fonts/truetype/liberation/LiberationSerif-Regular.ttf'): Buffer {
  const font = readFileSync(path);
  const tableCount = font.readUInt16BE(4);
  const tables = Array.from({ length: tableCount }, (_, i) => {
    const at = SFNT_HEADER_BYTES + i * SFNT_ENTRY_BYTES;
    return {
      tag: font.subarray(at, at + 4),
      checksum: font.readUInt32BE(at + 4),
      offset: font.readUInt32BE(at + 8),
      length: font.readUInt32BE(at + 12),
    };
  });
  const bodyStart = WOFF_HEADER_BYTES + tableCount * WOFF_ENTRY_BYTES;
  const bodySize = tables.reduce((sum, table) => sum + padded(table.length), 0);
  const woff = Buffer.alloc(bodyStart + bodySize);
  woff.write('wOFF', 0, 'latin1');
  woff.writeUInt32BE(font.readUInt32BE(0), 4);
  woff.writeUInt32BE(woff.length, 8);
  woff.writeUInt16BE(tableCount, 12);
  woff.writeUInt32BE(SFNT_HEADER_BYTES + tableCount * SFNT_ENTRY_BYTES + bodySize, 16);
  woff.writeUInt16BE(1, 20);
  let cursor = bodyStart;
  tables.forEach((table, i) => {
    const entry = WOFF_HEADER_BYTES + i * WOFF_ENTRY_BYTES;
    table.tag.copy(woff, entry);
    woff.writeUInt32BE(cursor, entry + 4);
    woff.writeUInt32BE(table.length, entry + 8);
    woff.writeUInt32BE(table.length, entry + 12);
    woff.writeUInt32BE(table.checksum, entry + 16);
    font.copy(woff, cursor, table.offset, table.offset + table.length);
    cursor += padded(table.length);
  });
  return woff;
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
