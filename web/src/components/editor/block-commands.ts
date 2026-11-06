// SPDX-License-Identifier: Apache-2.0
// Moving, copying and reshaping whole blocks. Every command here is one Undo.
import type { Editor } from '@tiptap/core';
import { closeHistory } from '@tiptap/pm/history';
import { endUndoStep } from './together';
import { Fragment, type Node as ProseNode } from '@tiptap/pm/model';
import { NodeSelection, Selection, TextSelection, type Transaction } from '@tiptap/pm/state';

export const MIN_COLUMNS = 2;
export const MAX_COLUMNS = 5;

/** The top-level block at the cursor, as its position in the document. */
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

export function blockAtCursor(editor: Editor): number | null {
  const { $from } = editor.state.selection;
  if ($from.depth === 0) return editor.state.selection.from;
  return $from.before(1);
}

export function moveBlock(editor: Editor, pos: number, direction: 'up' | 'down') {
  const { doc, tr } = editor.state;
  const index = doc.resolve(pos).index(0);
  const node = doc.child(index);
  const target = direction === 'up' ? index - 1 : index + 1;
  if (target < 0 || target >= doc.childCount) return;
  const neighbour = doc.child(target);
  const landing = direction === 'up' ? pos - neighbour.nodeSize : pos + neighbour.nodeSize;
  const offset = editor.state.selection.from - pos;
  tr.delete(pos, pos + node.nodeSize).insert(landing, node);
  tr.setSelection(
    Selection.near(tr.doc.resolve(Math.min(landing + Math.max(offset, 1), tr.doc.content.size))),
  );
  dispatchStep(editor, tr.scrollIntoView());
}

export function duplicateBlock(editor: Editor, pos: number) {
  const { doc, tr } = editor.state;
  const node = doc.nodeAt(pos);
  if (node) dispatchStep(editor, tr.insert(pos + node.nodeSize, node));
}

export function deleteBlock(editor: Editor, pos: number) {
  const { doc, tr } = editor.state;
  const node = doc.nodeAt(pos);
  if (node) dispatchStep(editor, tr.delete(pos, pos + node.nodeSize));
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
export function unwrapBlock(editor: Editor, name: 'panel' | 'expand' | 'columns') {
  const found = ancestor(editor, name);
  if (!found) return;
  const content =
    name === 'columns'
      ? childrenOf(found.node).reduce((all, column) => all.append(column.content), Fragment.empty)
      : found.node.content;
  const { tr } = editor.state;
  dispatchStep(editor, tr.replaceWith(found.pos, found.pos + found.node.nodeSize, content));
}

/** Adds columns and puts the cursor in the first one. */
export function insertColumns(editor: Editor, count: number) {
  const before = editor.state.selection.from;
  const column = { type: 'column', content: [{ type: 'paragraph' }] };
  editor
    .chain()
    .focus()
    .insertContent({ type: 'columns', content: Array.from({ length: count }, () => column) })
    .run();
  const { doc, tr } = editor.state;
  const start = doc.resolve(Math.max(before - 1, 0));
  editor.view.dispatch(tr.setSelection(TextSelection.near(start, 1)));
}
