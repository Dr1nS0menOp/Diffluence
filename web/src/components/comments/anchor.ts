// SPDX-License-Identifier: Apache-2.0
// Keeps a comment on its words. A thread holds a quote, not a position: versions cannot
// change, and a reader cannot edit, so nothing goes into the document (spec 010, D1).
import { Extension } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view';
import type { CommentAnchor } from '../../api';

const CONTEXT_CHARS = 32;
const MAX_EXACT_CHARS = 1000;
const BLOCK_BREAK = '\n';
const LEAF = '￼';

export const refreshHighlights = new PluginKey<DecorationSet>('commentHighlights');

export interface QuoteMatch {
  from: number;
  to: number;
}

function placesOf(text: string, exact: string): number[] {
  const places: number[] = [];
  for (let at = text.indexOf(exact); at >= 0; at = text.indexOf(exact, at + 1)) places.push(at);
  return places;
}

/**
 * Finds a quote again. Only one fit counts: pointing at the wrong sentence is worse than
 * pointing at none (spec 010, D2).
 */
export function findQuote(text: string, anchor: CommentAnchor): QuoteMatch | null {
  const exact = anchor.exact ?? '';
  if (!exact) return null;
  const places = placesOf(text, exact);
  const prefix = anchor.prefix ?? '';
  const suffix = anchor.suffix ?? '';
  const fits = places.filter(
    (at) => text.slice(0, at).endsWith(prefix) && text.slice(at + exact.length).startsWith(suffix),
  );
  let chosen: number | undefined;
  if (fits.length === 1) chosen = fits[0];
  else if (fits.length > 1 && fits.includes(anchor.offset ?? -1)) chosen = anchor.offset;
  else if (fits.length === 0 && places.length === 1) chosen = places[0];
  return chosen === undefined ? null : { from: chosen, to: chosen + exact.length };
}

interface Segment {
  text: number;
  pos: number;
  length: number;
}

export interface TextIndex {
  text: string;
  segments: Segment[];
}

/** The text of a document with its positions, blocks apart by a line break. */
export function textIndex(doc: ProseNode): TextIndex {
  let text = '';
  const segments: Segment[] = [];
  doc.descendants((node, pos) => {
    if (node.isText) {
      segments.push({ text: text.length, pos, length: node.nodeSize });
      text += node.text ?? '';
    } else if (node.isLeaf && node.isInline) {
      segments.push({ text: text.length, pos, length: 1 });
      text += LEAF;
    } else if (node.isTextblock && text !== '') {
      text += BLOCK_BREAK;
    }
    return true;
  });
  return { text, segments };
}

function positionOf(index: TextIndex, offset: number): number | null {
  for (const s of index.segments) {
    if (offset >= s.text && offset <= s.text + s.length) return s.pos + offset - s.text;
  }
  return null;
}

function offsetOf(index: TextIndex, pos: number): number | null {
  for (const s of index.segments) {
    if (pos >= s.pos && pos <= s.pos + s.length) return s.text + pos - s.pos;
  }
  return null;
}

/** What a selection in the reading view points at, or null when it says nothing. */
export function anchorOfSelection(view: EditorView, version: number): CommentAnchor | null {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || !selection.anchorNode || !selection.focusNode) return null;
  if (!view.dom.contains(selection.anchorNode) || !view.dom.contains(selection.focusNode)) return null;
  const range = selection.getRangeAt(0);
  const from = view.posAtDOM(range.startContainer, range.startOffset);
  const to = view.posAtDOM(range.endContainer, range.endOffset);
  const index = textIndex(view.state.doc);
  const start = offsetOf(index, from);
  const end = offsetOf(index, to);
  if (start === null || end === null || end <= start) return null;
  const exact = index.text.slice(start, end);
  if (!exact.trim() || exact.length > MAX_EXACT_CHARS) return null;
  return {
    kind: 'text',
    exact,
    prefix: index.text.slice(Math.max(0, start - CONTEXT_CHARS), start),
    suffix: index.text.slice(end, end + CONTEXT_CHARS),
    offset: start,
    version,
  };
}

export interface Highlight {
  id: string;
  label: string;
  anchor: CommentAnchor;
}

interface HighlightOptions {
  highlights: () => Highlight[];
  onFound: (ids: string[]) => void;
}

function buildDecorations(doc: ProseNode, options: HighlightOptions): DecorationSet {
  const index = textIndex(doc);
  const marks: Decoration[] = [];
  const found: string[] = [];
  for (const h of options.highlights()) {
    const match = findQuote(index.text, h.anchor);
    const from = match && positionOf(index, match.from);
    const to = match && positionOf(index, match.to);
    if (typeof from !== 'number' || typeof to !== 'number' || to <= from) continue;
    found.push(h.id);
    marks.push(
      Decoration.inline(from, to, {
        class: 'comment-mark',
        role: 'button',
        tabindex: '0',
        'aria-label': h.label,
        'data-comment': h.id,
      }),
    );
  }
  options.onFound(found);
  return DecorationSet.create(doc, marks);
}

/** Marks the words threads point at. Worked out in the browser, never stored. */
export const CommentHighlights = Extension.create<HighlightOptions>({
  name: 'commentHighlights',
  addOptions() {
    return { highlights: () => [], onFound: () => undefined };
  },
  addProseMirrorPlugins() {
    const options = this.options;
    return [
      new Plugin<DecorationSet>({
        key: refreshHighlights,
        state: {
          init: (_, state) => buildDecorations(state.doc, options),
          apply: (tr, old) => (tr.getMeta(refreshHighlights) ? buildDecorations(tr.doc, options) : old),
        },
        props: {
          decorations(state) {
            return refreshHighlights.getState(state);
          },
        },
      }),
    ];
  },
});
