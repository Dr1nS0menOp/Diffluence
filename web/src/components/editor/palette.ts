// SPDX-License-Identifier: Apache-2.0
// The names a document may use. The server keeps the same lists in
// internal/document/attrs.go, and refuses anything else (spec 008).

import emoji from './emoji.json';

export const TEXT_COLORS = [
  'grey',
  'slate',
  'red',
  'orange',
  'amber',
  'green',
  'teal',
  'blue',
  'indigo',
  'purple',
  'pink',
  'brown',
] as const;

export const MARK_COLORS = ['grey', 'yellow', 'green', 'blue', 'purple', 'red'] as const;

export const PANEL_KINDS = ['info', 'note', 'success', 'warning', 'error'] as const;

// A neutral panel offers these, found by their first word in the list of emoji.
const PANEL_EMOJI_WORDS = ['idea', 'pin', 'megaphone', 'star', 'lock', 'test', 'construction', 'party'];

export const PANEL_EMOJI = PANEL_EMOJI_WORDS.flatMap(
  (word) => emoji.find((entry) => entry.words.startsWith(word))?.glyph ?? [],
);

export const FIRST_PANEL_EMOJI = PANEL_EMOJI[0] ?? '';

export const IMAGE_SIZES = ['small', 'medium', 'large'] as const;

export const IMAGE_BORDERS = ['thin', 'medium'] as const;

// The same list as the server's `children`. A test keeps both the same as the schema.
export const KNOWN_NODE_TYPES = [
  'doc',
  'paragraph',
  'heading',
  'bulletList',
  'orderedList',
  'listItem',
  'taskList',
  'taskItem',
  'blockquote',
  'codeBlock',
  'horizontalRule',
  'hardBreak',
  'image',
  'inlineImage',
  'file',
  'panel',
  'spotlight',
  'unknownBlock',
  'expand',
  'columns',
  'column',
  'decision',
  'status',
  'date',
  'templateField',
  'instruction',
  'mention',
  'table',
  'tableRow',
  'tableCell',
  'tableHeader',
  'text',
  'dynamic',
  'pageFacts',
  'excerpt',
  'anchor',
  'chart',
  'sheet',
  'media',
  'pageLink',
  'pageCard',
  'placeholderLink',
];

export type MarkColor = (typeof MARK_COLORS)[number];
export type PanelKind = (typeof PANEL_KINDS)[number];

export const PANEL_TINT: Record<PanelKind, MarkColor> = {
  info: 'blue',
  note: 'purple',
  success: 'green',
  warning: 'yellow',
  error: 'red',
};
