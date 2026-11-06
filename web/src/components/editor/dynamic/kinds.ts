// SPDX-License-Identifier: Apache-2.0
// Every kind of dynamic block and its settings form. The server has the same list
// (internal/document/dynamic.go) and is the one that decides.

export type SettingType = 'text' | 'list' | 'number' | 'switch' | 'page' | 'pages' | 'sort' | 'style';

export interface SettingField {
  key: string;
  type: SettingType;
}

const field = (key: string, type: SettingType): SettingField => ({ key, type });

const FILTER: SettingField[] = [
  field('labels', 'list'),
  field('exclude_labels', 'list'),
  field('title', 'text'),
  field('below', 'page'),
  field('this_space', 'switch'),
  field('sort', 'sort'),
  field('descending', 'switch'),
  field('max', 'number'),
  field('hide_empty', 'switch'),
];

export const DYNAMIC_KINDS: Record<string, SettingField[]> = {
  toc: [field('from', 'number'), field('to', 'number'), field('numbered', 'switch')],
  back_to_top: [],
  search_box: [field('placeholder', 'text'), field('this_space', 'switch'), field('below', 'page')],
  link_cards: [field('pages', 'pages')],
  child_pages: [
    field('page', 'page'),
    field('max', 'number'),
    field('levels', 'number'),
    field('sort', 'sort'),
    field('descending', 'switch'),
    field('hide_empty', 'switch'),
  ],
  page_tree: [field('page', 'page')],
  space_index: [],
  spaces_list: [field('favourites', 'switch'), field('style', 'style')],
  breadcrumbs: [field('page', 'page')],
  recent_changes: [field('max', 'number'), field('this_space', 'switch'), field('hide_empty', 'switch')],
  my_favourites: [field('max', 'number')],
  my_drafts: [field('max', 'number')],
  my_tasks: [field('max', 'number')],
  filter_list: FILTER,
  report_table: FILTER,
  labels_list: [field('this_space', 'switch'), field('max', 'number')],
  facts_report: [...FILTER, field('facts_id', 'text'), field('columns', 'list'), field('sort_key', 'text')],
  decision_report: [field('labels', 'list'), field('this_space', 'switch'), field('max', 'number')],
  task_report: [
    field('labels', 'list'),
    field('this_space', 'switch'),
    field('mine', 'switch'),
    field('done', 'switch'),
    field('max', 'number'),
  ],
  show_excerpt: [field('page', 'page'), field('name', 'text'), field('frame', 'switch')],
  embedded_page: [field('page', 'page')],
  contributors: [field('page', 'page'), field('max', 'number')],
  history_here: [field('page', 'page'), field('max', 'number')],
  attachments_here: [field('page', 'page'), field('pattern', 'text'), field('max', 'number')],
  blog_posts: [field('max', 'number'), field('this_space', 'switch'), field('labels', 'list')],
};

/** These answer in the browser, from the document itself. */
export const LOCAL_KINDS = new Set(['toc', 'back_to_top', 'search_box']);

/** A setting for a list is one line with commas, the way a person writes it. */
export function listFromText(text: string): string[] {
  return text
    .split(',')
    .map((part) => part.trim())
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
