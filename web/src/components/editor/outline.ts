// SPDX-License-Identifier: Apache-2.0
// What a reader can do with the structure of a document: heading addresses, the
// contents list, reading time and slides (spec 009).
import { Extension } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import type { Json } from '../../api';
import { linkIconElement } from '../icons';

const WORDS_PER_MINUTE = 230;
const SHORT_PAGE_WORDS = 100;
const CONTENTS_MAX_LEVEL = 3;

interface JsonNode {
  type?: string;
  text?: string;
  attrs?: { level?: number };
  content?: JsonNode[];
}

export interface Heading {
  slug: string;
  level: number;
  text: string;
}

/** A readable address for a heading. Letters of any language stay. */
export function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '');
}

/** The same text twice gets -2, -3, so every heading has its own address. */
function slugMaker() {
  const seen = new Map<string, number>();
  return (text: string) => {
    const base = slugify(text) || 'section';
    const count = (seen.get(base) ?? 0) + 1;
    seen.set(base, count);
    return count === 1 ? base : `${base}-${count}`;
  };
}

function textOf(node: JsonNode): string {
  return node.text ?? (node.content ?? []).map(textOf).join('');
}

function walk(node: JsonNode, visit: (n: JsonNode) => void) {
  visit(node);
  node.content?.forEach((child) => walk(child, visit));
}

/** Every heading in document order, the same order the marks in the page use. */
export function headingsOf(document: Json): Heading[] {
  const next = slugMaker();
  const out: Heading[] = [];
  walk(document as JsonNode, (n) => {
    if (n.type !== 'heading') return;
    const text = textOf(n);
    out.push({ slug: next(text), level: n.attrs?.level ?? 1, text });
  });
  return out;
}

export const contentsOf = (document: Json) =>
  headingsOf(document).filter((h) => h.level <= CONTENTS_MAX_LEVEL);

export function countWords(document: Json): number {
  let words = 0;
  walk(document as JsonNode, (n) => {
    if (n.text) words += n.text.trim().split(/\s+/).filter(Boolean).length;
  });
  return words;
}

/** Minutes to read, or null for a page too short to bother. */
export function readingMinutes(words: number): number | null {
  if (words < SHORT_PAGE_WORDS) return null;
  return Math.max(1, Math.ceil(words / WORDS_PER_MINUTE));
}

/** One slide per heading of the highest level at the top of the document. */
export function slidesOf(document: Json): Json[] {
  const blocks = (document as JsonNode).content ?? [];
  const levels = blocks.flatMap((b) => (b.type === 'heading' ? [b.attrs?.level ?? 1] : []));
  if (levels.length === 0) return [document];
  const top = Math.min(...levels);
  const slides: JsonNode[][] = [];
  for (const block of blocks) {
    const startsSlide = block.type === 'heading' && (block.attrs?.level ?? 1) === top;
    if (startsSlide || slides.length === 0) slides.push([]);
    slides[slides.length - 1]?.push(block);
  }
  return slides.map((content) => ({ type: 'doc', content }));
}

function anchorMark(slug: string, text: string, label: string, onCopy: (slug: string) => void) {
  const link = document.createElement('button');
  link.type = 'button';
  link.className = 'heading-link';
  link.setAttribute('aria-label', `${label}: ${text}`);
  link.title = label;
  link.append(linkIconElement());
  link.addEventListener('click', () => onCopy(slug));
  return link;
}

function headingMarks(doc: ProseNode, label: string, onCopy: (slug: string) => void): DecorationSet {
  const next = slugMaker();
  const marks: Decoration[] = [];
  doc.descendants((node, pos) => {
    if (node.type.name !== 'heading') return true;
    const slug = next(node.textContent);
    marks.push(Decoration.node(pos, pos + node.nodeSize, { id: slug, tabindex: '-1' }));
    marks.push(
      Decoration.widget(pos + node.nodeSize - 1, () => anchorMark(slug, node.textContent, label, onCopy), {
        side: 1,
        ignoreSelection: true,
        key: slug,
      }),
    );
    return false;
  });
  return DecorationSet.create(doc, marks);
}

/** Gives headings an address and a link icon, for readers only. */
export const HeadingAnchors = Extension.create<{ label: string; onCopy: (slug: string) => void }>({
  name: 'headingAnchors',
  addOptions() {
    return { label: '', onCopy: () => undefined };
  },
  addProseMirrorPlugins() {
    const { label, onCopy } = this.options;
    return [
      new Plugin({
        key: new PluginKey('headingAnchors'),
        props: { decorations: (state) => headingMarks(state.doc, label, onCopy) },
      }),
    ];
  },
});

/** Scrolls to the words a thread points at, and marks them briefly. */
export function jumpToComment(id: string): void {
  const mark = document.querySelector<HTMLElement>(`[data-comment="${id}"]`);
  if (!mark) return;
  mark.scrollIntoView({ block: 'center' });
  mark.classList.add('heading-target');
  setTimeout(() => mark.classList.remove('heading-target'), 2000);
}

/** The next thread for `n` and `p`, going round. */
export function nextThread(ids: string[], current: string | null, step: 1 | -1): string | null {
  if (ids.length === 0) return null;
  const at = current === null ? -1 : ids.indexOf(current);
  if (at < 0) return step === 1 ? (ids[0] ?? null) : (ids[ids.length - 1] ?? null);
  return ids[(at + step + ids.length) % ids.length] ?? null;
}

/** Scrolls to a heading and marks it briefly. A section around it opens first. */
export function jumpToHeading(slug: string, moveFocus: boolean): boolean {
  const heading = document.getElementById(slug);
  if (!heading) return false;
  for (let at = heading.parentElement; at; at = at.parentElement) {
    if (at instanceof HTMLDetailsElement) at.open = true;
  }
  heading.scrollIntoView({ block: 'start' });
  if (moveFocus) heading.focus({ preventScroll: true });
  heading.classList.add('heading-target');
  setTimeout(() => heading.classList.remove('heading-target'), 2000);
  return true;
}
