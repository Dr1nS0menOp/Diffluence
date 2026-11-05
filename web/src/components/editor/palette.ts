// SPDX-License-Identifier: Apache-2.0
// The names a document may use. The server keeps the same lists in
// internal/document/attrs.go, and refuses anything else (spec 008).

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

export const IMAGE_SIZES = ['small', 'medium', 'large'] as const;

export type MarkColor = (typeof MARK_COLORS)[number];
export type PanelKind = (typeof PANEL_KINDS)[number];

export const PANEL_TINT: Record<PanelKind, MarkColor> = {
  info: 'blue',
  note: 'purple',
  success: 'green',
  warning: 'yellow',
  error: 'red',
};
