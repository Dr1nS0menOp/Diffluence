// SPDX-License-Identifier: Apache-2.0
// Every kind of dynamic block and its settings form. The server has the same list
// (internal/document/dynamic.go) and is the one that decides.
import type { Json } from '../../../api';
import type { TimelineItem } from '../../chart';

export type SettingType =
  'text' | 'list' | 'lines' | 'number' | 'switch' | 'date' | 'page' | 'pages' | 'sort' | 'choice';

export interface SettingField {
  key: string;
  type: SettingType;
  options?: string[];
}

const field = (key: string, type: SettingType): SettingField => ({ key, type });
const choice = (key: string, ...options: string[]): SettingField => ({ key, type: 'choice', options });

const FILTER: SettingField[] = [
  field('labels', 'list'),
  field('exclude_labels', 'list'),
  field('title', 'text'),
  field('text', 'text'),
  choice('content_type', 'page', 'post', 'all'),
  field('below', 'page'),
  field('direct', 'switch'),
  field('this_space', 'switch'),
  field('spaces', 'list'),
  field('creator', 'text'),
  field('contributor', 'text'),
  field('mentioned', 'text'),
  field('created_after', 'date'),
  field('changed_after', 'date'),
  field('sort', 'sort'),
  field('descending', 'switch'),
  field('max', 'number'),
  field('hide_empty', 'switch'),
];

const CARDS: SettingField[] = [field('pages', 'pages'), ...FILTER];

export const DYNAMIC_KINDS: Record<string, SettingField[]> = {
  toc: [field('from', 'number'), field('to', 'number'), field('numbered', 'switch')],
  section_contents: [
    field('start', 'text'),
    field('end', 'text'),
    field('section', 'text'),
    field('from', 'number'),
    field('to', 'number'),
  ],
  back_to_top: [],
  search_box: [
    field('placeholder', 'text'),
    field('this_space', 'switch'),
    field('below', 'page'),
    choice('content_type', 'page', 'post'),
    field('labels', 'list'),
  ],
  link_cards: CARDS,
  rotating_cards: CARDS,
  child_pages: [
    field('page', 'page'),
    field('max', 'number'),
    field('levels', 'number'),
    field('sort', 'sort'),
    field('descending', 'switch'),
    field('preview', 'switch'),
    field('hide_empty', 'switch'),
  ],
  page_tree: [field('page', 'page')],
  space_index: [],
  spaces_list: [field('favourites', 'switch'), choice('style', 'list', 'cards'), field('categories', 'list')],
  breadcrumbs: [field('page', 'page')],
  recent_changes: [
    field('max', 'number'),
    field('this_space', 'switch'),
    field('spaces', 'list'),
    field('hide_empty', 'switch'),
  ],
  activity_tabs: [field('max', 'number')],
  popular_pages: [field('days', 'number'), field('this_space', 'switch'), field('max', 'number')],
  my_favourites: [field('max', 'number')],
  my_drafts: [field('max', 'number')],
  my_tasks: [field('max', 'number')],
  filter_list: [...FILTER, field('preview', 'switch')],
  report_table: [...FILTER, field('columns', 'list')],
  labels_list: [field('this_space', 'switch'), field('max', 'number')],
  popular_labels: [field('this_space', 'switch'), choice('style', 'list', 'cloud'), field('max', 'number')],
  related_labels: [field('page', 'page'), field('max', 'number')],
  recent_labels: [field('this_space', 'switch'), field('max', 'number')],
  facts_report: [...FILTER, field('facts_id', 'text'), field('columns', 'list'), field('sort_key', 'text')],
  decision_report: [
    field('labels', 'list'),
    field('this_space', 'switch'),
    field('page', 'page'),
    field('person', 'text'),
    field('max', 'number'),
  ],
  task_report: [
    field('labels', 'list'),
    field('this_space', 'switch'),
    field('mine', 'switch'),
    field('done', 'switch'),
    field('created_by', 'text'),
    field('created_after', 'date'),
    field('max', 'number'),
  ],
  show_excerpt: [
    field('page', 'page'),
    field('name', 'text'),
    field('frame', 'switch'),
    field('values', 'lines'),
  ],
  synced_block: [field('page', 'page'), field('name', 'text'), field('values', 'lines')],
  embedded_page: [field('page', 'page')],
  contributors: [field('page', 'page'), field('kinds', 'list'), field('max', 'number')],
  contribution_table: [field('page', 'page'), choice('by', 'person', 'page'), field('max', 'number')],
  person_card: [field('person', 'text')],
  people_list: [
    field('people', 'list'),
    field('group', 'text'),
    choice('style', 'list', 'grid'),
    field('max', 'number'),
  ],
  history_here: [field('page', 'page'), field('max', 'number')],
  attachments_here: [
    field('page', 'page'),
    field('pattern', 'text'),
    field('labels', 'list'),
    field('max', 'number'),
  ],
  space_files: [field('labels', 'list'), field('file_type', 'text'), field('max', 'number')],
  blog_posts: [
    field('max', 'number'),
    field('this_space', 'switch'),
    field('labels', 'list'),
    field('days', 'number'),
    field('authors', 'list'),
  ],
  create_from_template: [field('template', 'page'), field('parent', 'page'), field('label', 'text')],
  timeline: [field('items', 'lines')],
};

/** These answer in the browser, from the document or their own settings. */
export const LOCAL_KINDS = new Set([
  'toc',
  'section_contents',
  'back_to_top',
  'search_box',
  'create_from_template',
  'timeline',
]);

/** A setting for a list is one line with commas, the way a person writes it. */
export function listFromText(text: string): string[] {
  return text
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean);
}

/** A setting of lines, for values that hold commas themselves. */
export function linesFromText(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean);
}

/** A pattern from a writer uses `*`. It is never a regular expression (spec 012, D18). */
export function matchesPattern(text: string, pattern: string): boolean {
  const parts = pattern.toLowerCase().split('*');
  const target = text.toLowerCase();
  let from = 0;
  return parts.every((part, index) => {
    if (part === '') return true;
    const at = target.indexOf(part, from);
    if (at < 0 || (index === 0 && at !== 0)) return false;
    from = at + part.length;
    return true;
  });
}

interface DocumentPart {
  type?: string;
  attrs?: { name?: string; title?: string };
  content?: DocumentPart[];
}

function holdsAnchor(part: DocumentPart, name: string): boolean {
  if (part.type === 'anchor' && part.attrs?.name === name) return true;
  return (part.content ?? []).some((child) => holdsAnchor(child, name));
}

/**
 * The part of a page a section block is about: one collapsible section by its title, or
 * everything between two anchors. Without a marker that exists, it is the whole page.
 */
export function sectionOf(content: Json, settings: Record<string, unknown>): Json {
  const blocks = (content as DocumentPart).content ?? [];
  const section = typeof settings.section === 'string' ? settings.section : '';
  if (section) {
    const found = blocks.find((block) => block.type === 'expand' && block.attrs?.title === section);
    return { type: 'doc', content: found?.content ?? [] } as unknown as Json;
  }
  const start = typeof settings.start === 'string' ? settings.start : '';
  const end = typeof settings.end === 'string' ? settings.end : '';
  const from = start ? blocks.findIndex((block) => holdsAnchor(block, start)) : 0;
  const to = end ? blocks.findIndex((block) => holdsAnchor(block, end)) : -1;
  const part = blocks.slice(Math.max(from, 0), to < 0 ? undefined : to);
  return { type: 'doc', content: part } as unknown as Json;
}

function timeOf(text: string | undefined): number | undefined {
  const time = text ? Date.parse(text) : Number.NaN;
  return Number.isNaN(time) ? undefined : time;
}

/** A line is lane | label | start | end. No end makes it a marker. A bad line is left out. */
export function timelineOf(lines: unknown): TimelineItem[] {
  if (!Array.isArray(lines)) return [];
  const items: TimelineItem[] = [];
  for (const line of lines as string[]) {
    const [lane, label, startText, endText] = line.split('|').map((part) => part.trim());
    const start = timeOf(startText);
    if (!lane || !label || start === undefined) continue;
    const end = timeOf(endText);
    items.push({ lane, label, start, text: line, ...(end !== undefined && end >= start ? { end } : {}) });
  }
  return items;
}
