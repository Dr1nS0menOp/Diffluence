// SPDX-License-Identifier: Apache-2.0
// The frontend uses only the public API, like any other client (blueprint 01).

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

async function send(method: string, path: string, body?: unknown, raw?: Blob | File): Promise<Response> {
  const init: RequestInit = { method, credentials: 'same-origin', headers: {} };
  if (raw !== undefined) {
    init.body = raw;
    init.headers = { 'Content-Type': 'application/octet-stream' };
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers = { 'Content-Type': 'application/json' };
  }
  const res = await fetch('/api/v1' + path, init);
  if (!res.ok) {
    let code = 'error';
    let message = res.statusText;
    try {
      const e = (await res.json()) as { error?: { code: string; message: string } };
      if (e.error) ({ code, message } = e.error);
    } catch {
      // not JSON: keep the status text
    }
    throw new ApiError(res.status, code, message);
  }
  return res;
}

async function json<T>(method: string, path: string, body?: unknown, raw?: Blob | File): Promise<T> {
  const res = await send(method, path, body, raw);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  get: <T>(path: string) => json<T>('GET', path),
  post: <T>(path: string, body?: unknown) => json<T>('POST', path, body ?? {}),
  put: <T>(path: string, body?: unknown) => json<T>('PUT', path, body ?? {}),
  patch: <T>(path: string, body: unknown) => json<T>('PATCH', path, body),
  del: <T = void>(path: string) => json<T>('DELETE', path),
  upload: <T>(path: string, file: Blob | File) => json<T>('POST', path, undefined, file),
  replace: <T>(path: string, file: Blob | File) => json<T>('PUT', path, undefined, file),
};

export type Json = Record<string, unknown>;

export interface User {
  id: string;
  email: string;
  display_name: string;
  type: string;
}

export interface Me {
  user: User;
  global_permissions: string[];
}

export interface Space {
  id: string;
  key: string;
  name: string;
  description: string;
  icon: string;
  status: string;
  home_page_id: string | null;
  can?: Record<string, boolean>;
}

export interface TreeNode {
  id: string;
  parent_id: string | null;
  position: number;
  title: string;
  status: string;
}

export interface Page {
  id: string;
  space_id: string;
  parent_id: string | null;
  title: string;
  status: string;
  current_version: number;
  created_by: string;
  updated_at: string;
}

export interface PageDetail extends Page {
  document: Json;
  breadcrumbs: { id: string; title: string }[];
  created_by_name: string;
  updated_by_name: string;
  has_draft: boolean;
  favourite: boolean;
  restricted: boolean;
  can: Record<string, boolean>;
}

export interface Draft {
  title: string;
  document: Json;
  base_version: number;
  updated_at: string;
}

export interface Version {
  number: number;
  title: string;
  author_name: string;
  summary: string;
  restored_from: number | null;
  created_at: string;
  document?: Json;
}

export interface DiffBlock {
  op: 'equal' | 'added' | 'removed' | 'changed';
  type: string;
  before?: string;
  after?: string;
  words?: { op: 'equal' | 'added' | 'removed'; text: string }[];
}

export interface Attachment {
  id: string;
  filename: string;
  version: number;
  size: number;
  content_type: string;
  inline: boolean;
  created_by: string;
  updated_at: string;
}

export interface Subject {
  type: string;
  id: string;
}

export interface Grant {
  subject: Subject;
  name: string;
  permissions: string[];
}

export interface NamedSubject extends Subject {
  name: string;
}

export interface Restrictions {
  view: NamedSubject[];
  edit: NamedSubject[];
  inherited: { page_id: string; title: string }[];
}

export interface DirectoryEntry {
  type: string;
  id: string;
  name: string;
  detail: string;
}

export interface TrashItem {
  id: string;
  object_type: string;
  title: string;
  deleted_at: string;
  purge_after: string;
}

export interface SearchResult {
  page_id: string;
  space_key: string;
  space_name: string;
  title: string;
  snippet: string;
  updated_at: string;
}

export interface ListedPage {
  id: string;
  title: string;
  space_key: string;
  space_name: string;
}

export interface Job {
  id: string;
  status: string;
  result: { pages?: number; files?: number; warnings?: string[]; error?: string };
}

export interface UserDetail {
  id: string;
  type: string;
  status: string;
  email: string;
  display_name: string;
  source: string;
  has_password: boolean;
  totp_enabled: boolean;
  passkeys: number;
  global_permissions: string[];
}

export interface Group {
  id: string;
  name: string;
  description: string;
  source: string;
  members: number;
  global_permissions: string[];
}

export interface AuditEvent {
  seq: number;
  occurred_at: string;
  actor_type: string;
  actor_id?: string;
  action: string;
  object_type?: string;
  object_id?: string;
  result: string;
  ip?: string;
}

export const NIL_ID = '00000000-0000-0000-0000-000000000000';
export const GLOBAL_PERMISSIONS = [
  'use_product',
  'create_space',
  'view_profiles',
  'administrator',
  'system_administrator',
];
