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

interface Call {
  method: string;
  path: string;
  body?: unknown;
  raw?: Blob | File;
  /** A request no person asked for: it does not keep a session alive (spec 013, D8). */
  background?: boolean;
  watch?: UploadWatch;
}

/** Follows an upload while it runs. The share goes from 0 to 1. */
export interface UploadWatch {
  onProgress?: (share: number) => void;
  signal?: AbortSignal;
}

/** True for an upload the person stopped, which is not a failure. */
export const isCancelled = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';

const BACKGROUND_HEADER = 'X-Background';
const NO_CONTENT = 204;

// Module state, on purpose: `api` is used outside React, and what answers a request
// for proof is a dialog inside it. `ReauthProvider` sets this once.
let confirmIdentity: (() => Promise<boolean>) | null = null;
let lastRequestAt = Date.now();

/** Lets a dialog answer `reauth_required`, so the request can be sent again. */
export function answerReauthWith(confirm: (() => Promise<boolean>) | null): void {
  confirmIdentity = confirm;
}

/** When a person last did something the server saw. */
export const lastRequestTime = () => lastRequestAt;

/** Fetch cannot tell how far an upload is. This gives the same Response, so all else stays the same. */
function sendWatched(call: Call, watch: UploadWatch): Promise<Response> {
  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open(call.method, '/api/v1' + call.path);
    request.setRequestHeader('Content-Type', 'application/octet-stream');
    request.upload.onprogress = (e) => {
      if (e.lengthComputable) watch.onProgress?.(e.loaded / e.total);
    };
    request.onload = () => {
      const body = request.status === NO_CONTENT ? null : request.responseText;
      resolve(new Response(body, { status: request.status, statusText: request.statusText }));
    };
    request.onerror = () => reject(new TypeError('The upload did not reach the server.'));
    request.onabort = () => reject(new DOMException('The upload was stopped.', 'AbortError'));
    watch.signal?.addEventListener('abort', () => request.abort(), { once: true });
    request.send(call.raw);
  });
}

function sendOnce(call: Call): Promise<Response> {
  if (call.watch) {
    lastRequestAt = Date.now();
    return sendWatched(call, call.watch);
  }
  const headers: Record<string, string> = {};
  const init: RequestInit = { method: call.method, credentials: 'same-origin', headers };
  if (call.raw !== undefined) {
    init.body = call.raw;
    headers['Content-Type'] = 'application/octet-stream';
  } else if (call.body !== undefined) {
    init.body = JSON.stringify(call.body);
    headers['Content-Type'] = 'application/json';
  }
  if (call.background) headers[BACKGROUND_HEADER] = '1';
  else lastRequestAt = Date.now();
  return fetch('/api/v1' + call.path, init);
}

async function errorOf(res: Response): Promise<ApiError> {
  let code = 'error';
  let message = res.statusText;
  try {
    const e = (await res.json()) as { error?: { code: string; message: string } };
    if (e.error) ({ code, message } = e.error);
  } catch {
    // not JSON: keep the status text
  }
  return new ApiError(res.status, code, message);
}

async function send(call: Call): Promise<Response> {
  const res = await sendOnce(call);
  if (res.ok) return res;
  const error = await errorOf(res);
  if (error.code !== 'reauth_required' || !confirmIdentity || !(await confirmIdentity())) throw error;
  const again = await sendOnce(call);
  if (!again.ok) throw await errorOf(again);
  return again;
}

async function json<T>(call: Call): Promise<T> {
  const res = await send(call);
  const body = await res.text();
  return (body ? JSON.parse(body) : undefined) as T;
}

export const api = {
  get: <T>(path: string) => json<T>({ method: 'GET', path }),
  /** For polling: see `Call.background`. */
  poll: <T>(path: string) => json<T>({ method: 'GET', path, background: true }),
  post: <T>(path: string, body?: unknown) => json<T>({ method: 'POST', path, body: body ?? {} }),
  put: <T>(path: string, body?: unknown) => json<T>({ method: 'PUT', path, body: body ?? {} }),
  patch: <T>(path: string, body: unknown) => json<T>({ method: 'PATCH', path, body }),
  del: <T = void>(path: string, body?: unknown) => json<T>({ method: 'DELETE', path, body }),
  upload: <T>(path: string, file: Blob | File, watch?: UploadWatch) =>
    json<T>({ method: 'POST', path, raw: file, watch }),
  replace: <T>(path: string, file: Blob | File) => json<T>({ method: 'PUT', path, raw: file }),
};

export type Json = Record<string, unknown>;

export interface User {
  id: string;
  email: string;
  display_name: string;
  type: string;
}

export interface Preferences {
  start_screen?: string;
  theme: string;
  shortcuts: boolean;
  contents: boolean;
  hover_cards: boolean;
  hide_follows: boolean;
  show_presence: boolean;
  typing_help: boolean;
  sidebar_width: number;
  /** Parts of the sidebar folded shut; the server sends null for none. */
  sidebar_shut: string[] | null;
  /** Height in rem of the links of a space, or 0 for their own height. */
  sidebar_links_height: number;
  /** Groups of the settings sidebar folded shut; the server sends null for none. */
  settings_shut: string[] | null;
  tree_view: string;
  language: string;
  time_zone: string;
  date_format: string;
  first_day: number;
  welcome_dismissed: boolean;
  tour_seen: boolean;
}

export interface Me {
  drawing_origin?: string;
  office_preview?: boolean;
  user: User;
  global_permissions: string[];
  preferences: Preferences;
  /** The key of the person's own space, or empty. */
  personal_space: string;
  needs_second_factor: boolean;
  sign_in: { weak: boolean; prompt: boolean };
  /** Zero means sessions do not end from idleness. */
  idle_minutes: number;
}

export interface Person {
  id: string;
  name: string;
  type: string;
  active: boolean;
  job_title: string;
  department: string;
  has_picture: boolean;
}

export interface Profile extends Person {
  email: string;
  location: string;
  about: string;
  time_zone: string;
  visibility?: Record<string, string>;
  locked_fields?: string[];
  is_self: boolean;
}

export interface Token {
  id: string;
  name: string;
  can_write: boolean;
  scopes: string[];
  expires_at: string;
  created_at: string;
  last_used_at: string | null;
}

export interface LoginEntry {
  method: string;
  ip: string;
  user_agent: string;
  at: string;
}

export interface MemberGroup {
  id: string;
  name: string;
  owner: boolean;
}

export interface Holder {
  subject: Subject;
  name: string;
  role: string;
  source: string;
  via: string;
  /** A person who is a guest of the instance. */
  guest: boolean;
}

export interface GrantPreview {
  gains: string[];
  loses: string[];
  people: number;
  loses_view: boolean;
}

export interface Category {
  id: string;
  name: string;
}

export interface SystemInfo {
  version: string;
}

export interface Space {
  id: string;
  key: string;
  name: string;
  description: string;
  icon: string;
  status: string;
  home_page_id: string | null;
  kind?: string;
  owner_id?: string | null;
  favourite?: boolean;
  can?: Record<string, boolean>;
  has_icon?: boolean;
  title_align?: string;
  review_months?: number;
  approval_required?: boolean;
  features?: Features;
}

export interface DirectorySpace extends Space {
  page_count: number;
  last_changed: string;
  administrators: string[];
}

export interface Shortcut {
  id?: string;
  label: string;
  target: string;
}

export interface SpaceOverview {
  page_count: number;
  administrators: string[];
  owners: { id: string; name: string }[];
  recent: ListedPage[];
  posts: BlogPost[];
}

export interface BlogPost {
  id: string;
  title: string;
  emoji: string;
  space_id: string;
  space_key: string;
  space_name: string;
  author_name: string;
  published_at: string;
  excerpt: string;
}

export interface Stage {
  text: string;
  color: string;
}

export interface LabelCount {
  label: string;
  count: number;
}

export interface Verified {
  by: string;
  by_name: string;
  at: string;
  until: string;
  version: number;
  changed_after: boolean;
  expired: boolean;
}

export interface ApprovalMark {
  round_id: string;
  status: string;
  approved_version: number;
  is_current: boolean;
  asked: number;
  answered: number;
  must_answer: boolean;
}

export interface ApprovalRound {
  id: string;
  asked_by: string;
  asked_by_name: string;
  message: string;
  due_on: string;
  title: string;
  base_version: number;
  approved_version: number;
  status: string;
  created_at: string;
  answers: { user_id: string; name: string; answer: string; comment: string }[];
  document?: Json;
}

export interface TemplateField {
  name: string;
  kind: string;
  options?: string[];
  required: boolean;
}

export interface TemplateSummary {
  id: string;
  space_id: string | null;
  name: string;
  description: string;
  category: string;
  labels: string[];
  title_pattern: string;
  steps: { title: string; fields: string[] }[];
  default_parent: string | null;
  hidden: boolean;
  promoted: boolean;
  starter: boolean;
  favourite: boolean;
  version: number;
  fields?: TemplateField[];
  document?: Json;
}

export interface ContentRow {
  id: string;
  parent_id: string | null;
  title: string;
  owner_id: string;
  owner_name: string;
  owner_disabled: boolean;
  stage_text: string;
  stage_color: string;
  labels: string[];
  created_at: string;
  updated_at: string;
  children: number;
}

export interface BulkFailure {
  id: string;
  reason: string;
}

export interface ArchivedPage {
  id: string;
  parent_id: string | null;
  title: string;
  archived_at: string;
  archived_by_name: string;
}

export interface CopyImpact {
  restrictions_lost: string[];
  needs_manage: boolean;
  /** False when the target space has no room for the files of the page. */
  fits: boolean;
}

export interface MoveImpact {
  pages: number;
  hidden: boolean;
  restrictions_lost: string[];
  restrictions_gained: string[];
  needs_manage: boolean;
  other_space: boolean;
  /** False when the target space has no room for the files of the branch. */
  fits: boolean;
}

export interface TreeNode {
  id: string;
  parent_id: string | null;
  position: number;
  title: string;
  status: string;
  emoji: string;
  restricted: boolean;
  has_draft: boolean;
  updated_at: string;
  updated_by_name: string;
  visited_at: string | null;
  kind: string;
  link_target: string;
  owner_name: string;
  stage_text: string;
  stage_color: string;
  /** Rows this person gets below this one. Only the parts of a tree say this. */
  has_children?: boolean;
}

/** A part of a tree, and the levels that go on past it. */
export interface TreeParts {
  pages: TreeNode[];
  more: { parent_id: string | null; offset: number }[];
}

/** What a filter or a flat view found, with the rows above the matches. */
export interface TreeFound {
  pages: TreeNode[];
  matches: string[];
  capped: boolean;
}

export interface PageLook {
  language: string;
  emoji: string;
  full_width: boolean;
  title_align: string;
  hide_children: boolean;
  header_attachment_id: string | null;
  header_position: number;
}

export interface Page extends PageLook {
  comments_closed: boolean;
  id: string;
  space_id: string;
  parent_id: string | null;
  title: string;
  status: string;
  current_version: number;
  created_by: string;
  updated_at: string;
  kind: string;
  owner_id: string;
  stage_text: string;
  stage_color: string;
  review_on: string;
  archived_at: string | null;
  published_at: string | null;
  link_target: string;
}

/** A page without its text: enough for a menu or a dialog. */
export interface PageSummary extends Page {
  favourite: boolean;
  can: Record<string, boolean>;
}

/** What is on in a space, by name. The server always sends every feature. */
export interface Features {
  comments: boolean;
  reactions: boolean;
  access_requests: boolean;
  [name: string]: boolean;
}

export interface PageDetail extends PageSummary {
  features: Features;
  document: Json;
  breadcrumbs: { id: string; title: string; emoji: string }[];
  created_by_name: string;
  updated_by_name: string;
  has_draft: boolean;
  restricted: boolean;
  owner_name: string;
  owner_disabled: boolean;
  labels: string[];
  verified: Verified | null;
  seen_version: number;
  scheduled: { at: string; silent: boolean } | null;
  approval: ApprovalMark | null;
  control: ControlInfo | null;
  /** The person reads this page only because they are asked to review it (DOC-203). */
  review_only: boolean;
}

/** What a state means to Diffluence, whatever a space calls it (spec 064). */
export type WorkflowStateKind = 'draft' | 'in_review' | 'approved' | 'rejected' | 'obsolete';

/** Which step the system takes when a review passes or asks for changes (spec 064, DC2). */
export type WorkflowOutcome = 'approved' | 'changes';

export interface WorkflowTransition {
  id: string;
  name: string;
  description: string;
  from: string;
  to: string;
  requires_comment: boolean;
  outcome?: WorkflowOutcome;
}

/** How many reviewers must approve: all, a number of them, or any one. */
export type ReviewRule = 'all' | 'quorum' | 'any';

export interface WorkflowReviewer {
  type: 'user' | 'group' | 'role';
  id: string;
}

export interface WorkflowReview {
  reviewers: WorkflowReviewer[];
  rule: ReviewRule;
  quorum?: number;
  authors_may_review: boolean;
  authors_may_add: boolean;
}

export interface WorkflowState {
  id: string;
  name: string;
  description: string;
  kind: WorkflowStateKind;
  start: boolean;
  timed: boolean;
  review?: WorkflowReview;
}

export interface WorkflowDefinition {
  states: WorkflowState[];
  transitions: WorkflowTransition[];
}

export interface Workflow {
  id: string;
  space_id: string;
  name: string;
  description: string;
  version: number;
  definition: WorkflowDefinition;
  archived: boolean;
  created_at: string;
  /** When its newest version was saved. */
  saved_at: string;
  /** Names of the people, groups and roles its reviews name, for those who may change it. */
  names?: Record<string, string>;
  pages: number;
  space_workflow: boolean;
  template_id: string | null;
  template_version: number | null;
  template_latest: number | null;
}

/** A rule a workflow breaks, with the state or transition it is about. */
export interface WorkflowProblem {
  rule: string;
  subject?: string;
  message: string;
}

/** One difference between two versions of a workflow. */
export interface WorkflowChange {
  kind: string;
  id: string;
  from?: string;
  to?: string;
}

export interface WorkflowVersion {
  version: number;
  definition: WorkflowDefinition;
  saved_by: string;
  saved_at: string;
  changes: WorkflowChange[];
}

/** A workflow Diffluence ships, which a space copies to start (DOC-131). */
export interface WorkflowExample {
  key: string;
  name: string;
  description: string;
  definition: WorkflowDefinition;
}

export interface WorkflowTemplateCopy {
  workflow_id: string;
  workflow_name: string;
  space_id: string;
  space_name: string;
  version: number;
}

/** An instance template of a workflow (DOC-130). */
export interface WorkflowTemplate {
  id: string;
  name: string;
  description: string;
  version: number;
  definition: WorkflowDefinition;
  saved_at: string;
  /** Names of the people, groups and roles it names, for administrators. */
  names?: Record<string, string>;
  copies?: WorkflowTemplateCopy[];
}

export interface ReviewerRow {
  user_id: string;
  name: string;
  source: 'user' | 'group' | 'role' | 'added';
  source_name: string;
  answer: '' | 'approve' | 'changes';
  comment: string;
  answered_at: string | null;
  removed: boolean;
  removed_reason: string;
}

export interface ReviewComment {
  id: string;
  user_id: string;
  name: string;
  body: string;
  created_at: string;
}

export interface ReviewText {
  version: number;
  title: string;
  document: Json;
}

/** The round of a controlled page, for its authors and reviewers (DOC-250, DOC-921). */
export interface ReviewPanelData {
  round_id: string | null;
  status: string;
  ended_reason: string;
  started_at: string | null;
  state_id: string;
  state_name: string;
  in_review: boolean;
  rule: ReviewRule | '';
  needed: number;
  approved: number;
  asked: number;
  independent: boolean;
  my_answer: '' | 'approve' | 'changes';
  reviewers: ReviewerRow[];
  comments: ReviewComment[];
  base: ReviewText | null;
  head: ReviewText | null;
  /** The change from the approved version, when the server compares for the panel. */
  changes?: DiffBlock[];
  can: { answer: boolean; add: boolean; remove: boolean; comment: boolean };
}

/** The workflow of a controlled page, as the band near its title shows it. */
export interface ControlInfo {
  workflow_id: string;
  workflow_name: string;
  workflow_version: number;
  state_id: string;
  state_name: string;
  state_kind: WorkflowStateKind;
  approved_version: number | null;
  approved_at: string | null;
  approved_by: string;
  head_version: number;
  shows_working: boolean;
  governed_by: 'space' | 'page' | 'branch';
  governed_from: string | null;
  governed_from_title: string;
  set_by: string;
  transitions: WorkflowTransition[];
  /** The person is asked to review the page now. */
  reviewing: boolean;
  /** The page waits for the answers of reviewers. */
  in_review: boolean;
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
  imported?: boolean;
  created_at: string;
  document?: Json;
  removed?: boolean;
}

export interface WordOp {
  op: 'equal' | 'added' | 'removed';
  text: string;
}

export interface DiffBlock {
  op: 'equal' | 'added' | 'removed' | 'changed';
  type: string;
  before?: string;
  after?: string;
  words?: WordOp[];
  node?: Json;
  previous?: Json;
}

export interface Attachment {
  id: string;
  page_id: string;
  filename: string;
  version: number;
  size: number;
  content_type: string;
  inline: boolean;
  office: boolean;
  created_by: string;
  updated_at: string;
}

/** A file as the list of a page shows it. */
export interface FileRow extends Attachment {
  comment: string;
  added_by: string;
  created_at: string;
  labels: string[];
  in_use: boolean;
  clearable: boolean;
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
  deleted_by_name: string;
  hidden: boolean;
}

export interface SearchResult {
  page_id: string;
  space_key: string;
  space_name: string;
  title: string;
  snippet: string;
  updated_at: string;
  emoji?: string;
  kind?: string;
  stage_text?: string;
  stage_color?: string;
  archived?: boolean;
  breadcrumbs?: { id: string; title: string }[];
  comment_id?: string;
}

export interface SearchAnswer {
  results: SearchResult[];
  total: number;
  capped: boolean;
  suggestion: string;
  /** The search gave up: no results then means "not known", never "nothing there". */
  timed_out: boolean;
}

export interface ListedPage {
  id: string;
  title: string;
  emoji: string;
  space_key: string;
  space_name: string;
  at: string;
  unpublished: boolean;
}

export interface PageRef {
  id: string;
  title: string;
  emoji: string;
  space_key: string;
  space_name: string;
}

export interface PageFacts {
  created_at: string;
  created_by_name: string;
  updated_at: string;
  updated_by_name: string;
  contributors: { id: string; name: string; versions: number }[];
  incoming: PageRef[];
  outgoing: PageRef[];
  children: PageRef[];
}

export interface CommentAnchor {
  kind: 'text' | 'image';
  exact?: string;
  prefix?: string;
  suffix?: string;
  offset?: number;
  version?: number;
  attachment_id?: string;
}

export interface Reaction {
  emoji: string;
  count: number;
  mine: boolean;
}

export interface Reactor {
  emoji: string;
  names: string[];
  more: number;
}

export interface Comment {
  id: string;
  page_id: string;
  parent_id: string | null;
  author_id: string;
  author_name: string;
  author_active: boolean;
  body: Json | null;
  anchor: CommentAnchor | null;
  status: 'open' | 'resolved';
  editors_only: boolean;
  removed: boolean;
  unread: boolean;
  created_at: string;
  edited_at: string | null;
  resolved_at: string | null;
  resolved_by: string;
  reactions: Reaction[];
  can: { edit: boolean; delete: boolean; resolve: boolean };
  replies: Comment[];
}

export interface CommentPage {
  threads: Comment[];
  inline: Comment[];
  total: number;
  unread: number;
  closed: boolean;
  seen_at: string;
}

export interface MentionCandidate {
  kind: 'user' | 'group';
  id: string;
  name: string;
  members?: number;
  can_view: boolean;
}

export interface TaskItem {
  id: string;
  text: string;
  done: boolean;
  due: string | null;
  page_id: string;
  page_title: string;
  space_name: string;
  assignee_name: string;
}

export interface Notification {
  id: string;
  type: string;
  reason: string;
  actors: string[];
  count: number;
  page_id: string;
  page_title: string;
  space_id: string;
  space_name: string;
  comment_id: string | null;
  read: boolean;
  at: string;
  data: {
    message?: string;
    request_id?: string;
    export_id?: string;
    filename?: string;
    email?: string;
    answer?: 'approved' | 'refused';
    reason?: string;
  };
}

export interface NotificationSettings {
  channels: Record<string, string>;
  digest_frequency: string;
  digest_day: number;
  digest_hour: number;
  time_zone: string;
  quiet_start: string;
  quiet_end: string;
  paused_until: string;
  auto_watch_create: boolean;
  auto_watch_edit: boolean;
  auto_watch_comment: boolean;
  notify_own: boolean;
  comment_order: string;
}

export interface WatchEntry {
  type: 'page' | 'branch' | 'space';
  id: string;
  title: string;
  space_name: string;
  since: string;
}

export interface FeedItem {
  page_id: string;
  title: string;
  emoji: string;
  space_key: string;
  space_name: string;
  excerpt: string;
  author_name: string;
  kind: string;
  at: string;
  reactions: number;
  comments: number;
}

export interface AccessEntry {
  type: string;
  id: string;
  name: string;
}

export interface AccessLevel {
  title: string;
  kind: string;
  pages: number;
  can_manage: boolean;
}

export interface AccessRequest {
  id: string;
  page_id: string;
  requester_name: string;
  kind: string;
  status: string;
  effect: string;
  levels: AccessLevel[];
  can_allow: boolean;
  created_at: string;
}

export interface MailEntry {
  id: string;
  kind: string;
  status: string;
  attempts: number;
  last_error: string;
  created_at: string;
  sent_at: string | null;
}

export interface DeleteImpact {
  pages: number;
  incoming: PageRef[];
  incoming_total: number;
}

export interface QuickResults {
  pages: SearchResult[];
  posts?: SearchResult[];
  spaces: Space[];
  files: { id: string; filename: string; page_id: string; page_title: string; space_name: string }[];
  people: { id: string; name: string }[];
}

export interface Job {
  id: string;
  status: string;
  result: {
    pages?: number;
    files?: number;
    warnings?: string[];
    error?: string;
    done?: number;
    failed?: BulkFailure[];
  };
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
  /** Password login stopped after too many wrong passwords in a row. */
  password_locked: boolean;
  passkeys: number;
  global_permissions: string[];
  expires_at: string | null;
  last_login_at: string | null;
  /** The keys of the spaces a guest can open. Empty for members. */
  spaces?: string[];
}

/** A reset link an administrator sent: mailed, or handed back once to pass on. */
export type ResetLink = { mailed: true } | { mailed: false; link: string };

export interface Group {
  id: string;
  name: string;
  description: string;
  source: string;
  members: number;
  global_permissions: string[];
}

export interface AuditEvent {
  id: string;
  /** Zero while the event waits for the sealer (ADR-041). */
  seq: number;
  sealed: boolean;
  occurred_at: string;
  actor_type: string;
  actor_id?: string;
  action: string;
  object_type?: string;
  object_id?: string;
  result: string;
  ip?: string;
  details?: Json;
}

export interface ExportOptions {
  comments: boolean;
  versions: boolean;
  attachments: boolean;
  restricted: boolean;
  paper: string;
  orientation: string;
  contents: boolean;
}

export interface ExportRun {
  id: string;
  format: string;
  state: string;
  filename: string;
  size: number;
  report: {
    pages: number;
    skipped: number;
    restricted: number;
    drafts?: string[];
    failed?: string[];
    explanation?: string;
  };
}

export interface ExportPolicy {
  rules: { format: string; enabled: boolean; group_ids: string[] }[];
  limits: {
    max_export_mib: number;
    max_import_mib: number;
    public_sites_enabled: boolean;
    public_links_enabled: boolean;
  };
  pdf_available: boolean;
}

export interface ImportTarget {
  people: Record<string, string>;
  fingerprint: string;
  space_key: string;
}

export interface ImportRun {
  id: string;
  kind: string;
  state: string;
  filename: string;
  touched: string[] | null;
  report: {
    pages: number;
    files: number;
    comments: number;
    versions: number;
    tree: { title: string; depth: number }[];
    warnings: string[];
    people?: { id: string; name: string; email: string; matched?: string }[];
    signer?: string;
    reason?: string;
  };
}

export interface PublicLink {
  id: string;
  page_id: string;
  page_title: string;
  version: number;
  path: string;
  created_by: string;
}

export interface PublicSite {
  id: string;
  space_id: string;
  name: string;
  address: string;
  language: string;
  accent: string;
  show_names: boolean;
  search_engines: boolean;
  update_mode: string;
  live_build: number;
}

export interface SiteChoice {
  id: string;
  parent_id: string | null;
  title: string;
  chosen: boolean;
  blocked: boolean;
}

export interface SiteReport {
  added: string[];
  changed: string[];
  removed: string[];
  left_out: string[];
  dead_links: string[];
  no_alt: string[];
  metadata: string[];
  stopped?: string;
}

export interface SiteBuild {
  number: number;
  state: string;
  report: SiteReport;
}

export const NIL_ID = '00000000-0000-0000-0000-000000000000';
export const GLOBAL_PERMISSIONS = [
  'use_product',
  'create_space',
  'view_profiles',
  'create_personal_space',
  'invite_people',
  'administrator',
  'system_administrator',
];
