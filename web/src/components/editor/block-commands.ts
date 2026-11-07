// SPDX-License-Identifier: Apache-2.0
// Moving, copying and reshaping whole blocks. Every command here is one Undo.
import type { Editor } from '@tiptap/core';
import { closeHistory } from '@tiptap/pm/history';
import { endUndoStep } from './together';
import { Fragment, type Node as ProseNode } from '@tiptap/pm/model';
import { NodeSelection, Selection, TextSelection, type Transaction } from '@tiptap/pm/state';

export const MIN_COLUMNS = 2;
export const MAX_COLUMNS = 5;

/** One step for undo, in the history of the page and on a shared document. */
function dispatchStep(editor: Editor, tr: Transaction) {
  endUndoStep(editor.state);
  editor.view.dispatch(closeHistory(tr));
}

/** Puts the cursor after everything, in a new paragraph when the last block is not one. */
export function continueAtEnd(editor: Editor) {
  const { doc } = editor.state;
  const chain = editor.chain();
  if (doc.lastChild?.type.name !== 'paragraph') {
    chain.insertContentAt(doc.content.size, { type: 'paragraph' });
  }
  chain.focus('end').run();
}

const LIST_ITEMS = { bulletList: 'listItem', orderedList: 'listItem', taskList: 'taskItem' } as const;

export type ListKind = keyof typeof LIST_ITEMS;

const isList = (node: ProseNode) => node.type.name in LIST_ITEMS;

function listOfKind(list: ProseNode, kind: ListKind): ProseNode {
  const { nodes } = list.type.schema;
  const items: ProseNode[] = [];
  list.forEach((item) => {
    const content: ProseNode[] = [];
    item.forEach((child) => content.push(isList(child) ? listOfKind(child, kind) : child));
    items.push(nodes[LIST_ITEMS[kind]]!.create(null, content));
  });
  return nodes[kind]!.create(null, items);
}

/** Turns the whole list at the cursor into another kind. Text and nesting stay (EDT-052). */
export function setListKind(editor: Editor, kind: ListKind): boolean {
  const { selection, tr } = editor.state;
  const { $from } = selection;
  for (let depth = 1; depth <= $from.depth; depth += 1) {
    const node = $from.node(depth);
    if (!isList(node)) continue;
    if (node.type.name === kind) return false;
    const start = $from.before(depth);
    tr.replaceWith(start, start + node.nodeSize, listOfKind(node, kind));
    tr.setSelection(Selection.near(tr.doc.resolve(selection.from)));
    dispatchStep(editor, tr);
    return true;
  }
  return false;
}

/** The list tools: another kind of list changes, the same kind goes back to text. */
export function toggleList(editor: Editor, kind: ListKind) {
  if (setListKind(editor, kind)) {
    editor.commands.focus();
    return;
  }
  const chain = editor.chain().focus();
  if (kind === 'bulletList') chain.toggleBulletList().run();
  else if (kind === 'orderedList') chain.toggleOrderedList().run();
  else chain.toggleTaskList().run();
}

/** The top-level block at the cursor, as its position in the document. */
export function blockAtCursor(editor: Editor): number | null {
  const { $from } = editor.state.selection;
  if ($from.depth === 0) return editor.state.selection.from;
  return $from.before(1);
}

/** A run of top-level blocks, as positions before the first and after the last. */
export interface BlockRange {
  from: number;
  to: number;
}

/** The blocks an action on this block means: the whole selection when it holds this block too. */
export function blockRange(editor: Editor, pos: number): BlockRange | null {
  const { doc, selection } = editor.state;
  const node = doc.nodeAt(pos);
  if (!node) return null;
  const single = { from: pos, to: pos + node.nodeSize };
  const { $from, $to } = selection;
  if ($from.depth === 0 || $to.depth === 0) return single;
  const from = $from.before(1);
  const to = $to.after(1);
  const spansSeveral = $from.index(0) !== $to.index(0);
  return spansSeveral && pos >= from && pos < to ? { from, to } : single;
}

/** Selects everything from one top-level block to another, whichever comes first. */
export function selectBlocks(editor: Editor, first: number, second: number) {
  const { doc, tr } = editor.state;
  const start = Math.min(first, second);
  const last = doc.nodeAt(Math.max(first, second));
  if (!last) return;
  const end = Math.max(first, second) + last.nodeSize;
  const anchor = Selection.near(doc.resolve(start + 1), 1).from;
  const head = Selection.near(doc.resolve(end - 1), -1).to;
  editor.view.dispatch(tr.setSelection(TextSelection.create(doc, anchor, head)));
}

export function moveBlock(editor: Editor, pos: number, direction: 'up' | 'down') {
  const { doc, tr, selection } = editor.state;
  const range = blockRange(editor, pos);
  if (!range) return;
  const before = doc.resolve(range.from).index(0);
  const after = doc.resolve(range.to).index(0);
  const target = direction === 'up' ? before - 1 : after;
  if (target < 0 || target >= doc.childCount) return;
  const neighbour = doc.child(target);
  const content = doc.slice(range.from, range.to).content;
  const shift = direction === 'up' ? -neighbour.nodeSize : neighbour.nodeSize;
  tr.delete(range.from, range.to).insert(range.from + shift, content);
  const size = tr.doc.content.size;
  const anchor = Math.min(Math.max(selection.from, range.from + 1) + shift, size);
  const head = Math.min(selection.to + shift, size);
  tr.setSelection(
    selection.empty
      ? Selection.near(tr.doc.resolve(anchor))
      : TextSelection.between(tr.doc.resolve(anchor), tr.doc.resolve(head)),
  );
  dispatchStep(editor, tr.scrollIntoView());
}

export function duplicateBlock(editor: Editor, pos: number) {
  const { doc, tr } = editor.state;
  const range = blockRange(editor, pos);
  if (range) dispatchStep(editor, tr.insert(range.to, doc.slice(range.from, range.to).content));
}

export function deleteBlock(editor: Editor, pos: number) {
  const { tr } = editor.state;
  const range = blockRange(editor, pos);
  if (range) dispatchStep(editor, tr.delete(range.from, range.to));
}

const ANCHOR_LETTERS = 'abcdefghijklmnopqrstuvwxyz';
const ANCHOR_LENGTH = 8;

function newAnchorName(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(ANCHOR_LENGTH));
  const letters = Array.from(bytes, (byte) => ANCHOR_LETTERS[byte % ANCHOR_LETTERS.length]);
  return `block-${letters.join('')}`;
}

/** Where an anchor can go in a block: the first line of text in it, as a position inside that line. */
function firstLine(block: ProseNode, pos: number): { node: ProseNode; start: number } | null {
  if (block.isTextblock) return block.type.spec.code ? null : { node: block, start: pos + 1 };
  let found: { node: ProseNode; start: number } | null = null;
  block.descendants((node, offset) => {
    if (!found && node.isTextblock && !node.type.spec.code) found = { node, start: pos + 1 + offset + 1 };
    return found === null;
  });
  return found;
}

export const canLinkToBlock = (block: ProseNode, pos: number) => firstLine(block, pos) !== null;

/** Gives a block a place to link to (spec 027, D3). An anchor it already starts with is used again. */
export function anchorOfBlock(editor: Editor, pos: number): string | null {
  const block = editor.state.doc.nodeAt(pos);
  const line = block && firstLine(block, pos);
  if (!line) return null;
  const first = line.node.firstChild;
  if (first?.type.name === 'anchor') return first.attrs.name as string;
  const name = newAnchorName();
  const anchor = editor.schema.nodes.anchor;
  if (!anchor) return null;
  dispatchStep(editor, editor.state.tr.insert(line.start, anchor.create({ name })));
  return name;
}

/**
 * Changes the selected item and keeps it selected. A node without content is replaced
 * by such a change, which would otherwise end the selection and hide its tools.
 */
export function updateSelected(editor: Editor, attrs: Record<string, unknown>) {
  const { selection, tr } = editor.state;
  if (!(selection instanceof NodeSelection)) return;
  tr.setNodeMarkup(selection.from, undefined, { ...selection.node.attrs, ...attrs });
  editor.view.dispatch(tr.setSelection(NodeSelection.create(tr.doc, selection.from)));
}

/** Makes a selected image small enough for a line of text, in a line of its own to start with. */
export function imageIntoLine(editor: Editor) {
  const { selection, tr, schema } = editor.state;
  const inline = schema.nodes.inlineImage;
  const paragraph = schema.nodes.paragraph;
  if (!(selection instanceof NodeSelection) || !inline || !paragraph) return;
  const { attachment_id: id, alt } = selection.node.attrs as { attachment_id: string; alt: string | null };
  const line = paragraph.create(null, inline.create({ attachment_id: id, alt }));
  dispatchStep(editor, tr.replaceSelectionWith(line));
}

/** Puts the cursor in a block, so that a command runs on that block. */
export function selectBlock(editor: Editor, pos: number) {
  const { doc, tr } = editor.state;
  editor.view.dispatch(tr.setSelection(Selection.near(doc.resolve(pos + 1))));
}

function ancestor(editor: Editor, name: string) {
  const { $from } = editor.state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === name) return { node: $from.node(depth), pos: $from.before(depth) };
  }
  return null;
}

function childrenOf(node: ProseNode): ProseNode[] {
  const children: ProseNode[] = [];
  node.forEach((child) => children.push(child));
  return children;
}

/** Fewer columns move their content into the last one that stays, so nothing is lost. */
export function resizeColumns(columns: ProseNode[], count: number, empty: () => ProseNode): ProseNode[] {
  const kept = columns.slice(0, count);
  const removed = columns.slice(count);
  const last = kept[kept.length - 1];
  if (last && removed.length > 0) {
    const moved = removed.reduce((content, column) => content.append(column.content), last.content);
    kept[kept.length - 1] = last.copy(moved);
  }
  while (kept.length < count) kept.push(empty());
  return kept;
}

export function setColumnCount(editor: Editor, count: number) {
  const found = ancestor(editor, 'columns');
  const column = editor.schema.nodes.column;
  if (!found || !column) return;
  const empty = () => column.createAndFill() as ProseNode;
  const updated = found.node.copy(Fragment.fromArray(resizeColumns(childrenOf(found.node), count, empty)));
  const { tr } = editor.state;
  dispatchStep(editor, tr.replaceWith(found.pos, found.pos + found.node.nodeSize, updated));
}

export const columnCount = (editor: Editor) => ancestor(editor, 'columns')?.node.childCount ?? 0;

/** Takes away the frame of a panel, section or columns, and keeps everything inside it. */
export function unwrapBlock(editor: Editor, name: 'panel' | 'expand' | 'columns' | 'spotlight') {
  const found = ancestor(editor, name);
  if (!found) return;
  const content =
    name === 'columns'
      ? childrenOf(found.node).reduce((all, column) => all.append(column.content), Fragment.empty)
      : found.node.content;
  const { tr } = editor.state;
  dispatchStep(editor, tr.replaceWith(found.pos, found.pos + found.node.nodeSize, content));
}

export type DropSide = 'start' | 'end';

/** Puts a dragged block beside another, in two new columns (EDT-119). One Undo takes it back. */
export function columnsFromDrop(editor: Editor, source: number, target: number, side: DropSide): boolean {
  const { doc, tr, schema } = editor.state;
  const { columns, column } = schema.nodes;
  const moved = doc.nodeAt(source);
  const stays = doc.nodeAt(target);
  if (!columns || !column || !moved || !stays || source === target || stays.type === columns) return false;
  const pair = side === 'start' ? [moved, stays] : [stays, moved];
  const made = columns.create(
    null,
    pair.map((block) => column.create(null, block)),
  );
  // The later block goes first, so the position of the other stays right.
  if (source > target) {
    tr.delete(source, source + moved.nodeSize).replaceWith(target, target + stays.nodeSize, made);
  } else {
    tr.replaceWith(target, target + stays.nodeSize, made).delete(source, source + moved.nodeSize);
  }
  dispatchStep(editor, tr);
  return true;
}

/** Adds columns and puts the cursor in the first one. */
export function insertColumns(editor: Editor, count: number, layout?: string) {
  const before = editor.state.selection.from;
  const column = { type: 'column', content: [{ type: 'paragraph' }] };
  editor
    .chain()
    .focus()
    .insertContent({
      type: 'columns',
      attrs: layout ? { layout } : {},
      content: Array.from({ length: count }, () => column),
    })
    .run();
  const { doc, tr } = editor.state;
  const start = doc.resolve(Math.max(before - 1, 0));
  editor.view.dispatch(tr.setSelection(TextSelection.near(start, 1)));
}
