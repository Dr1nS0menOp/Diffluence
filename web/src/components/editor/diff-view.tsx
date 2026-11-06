// SPDX-License-Identifier: Apache-2.0
// A comparison as the page itself: the real blocks with marks on top (spec 018).
// Decorations change nothing in the document, so what is compared stays as stored.
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Extension } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import { EditorContent, useEditor } from '@tiptap/react';
import type { DiffBlock, Json, WordOp } from '../../api';
import { documentExtensions } from './extensions';
import { textOf } from './outline';

/** Words can only be marked where the text of a block is one line of words. */
const WORD_BLOCKS = new Set(['paragraph', 'heading']);

export interface ShownBlock {
  op: DiffBlock['op'];
  node: Json;
  words?: WordOp[];
}

export interface WordRanges {
  added: [number, number][];
  removed: { at: number; text: string }[];
}

/** Finds the words of a comparison in a text. Null when they do not fit it. */
export function wordRanges(text: string, words: WordOp[]): WordRanges | null {
  const found = [...text.matchAll(/\S+/g)];
  const out: WordRanges = { added: [], removed: [] };
  let next = 0;
  for (const run of words) {
    if (run.op === 'removed') {
      out.removed.push({ at: found[next]?.index ?? text.length, text: run.text });
      continue;
    }
    const count = run.text.split(/\s+/).filter(Boolean).length;
    const first = found[next];
    const last = found[next + count - 1];
    if (!first || !last) return null;
    const end = last.index + last[0].length;
    if (text.slice(first.index, end).split(/\s+/).join(' ') !== run.text) return null;
    if (run.op === 'added') out.added.push([first.index, end]);
    next += count;
  }
  return next === found.length ? out : null;
}

function changedBlock(block: DiffBlock, node: Json): ShownBlock[] {
  const fits = block.words && WORD_BLOCKS.has(block.type) && wordRanges(textOf(node), block.words) !== null;
  if (fits) return [{ op: 'changed', node, words: block.words }];
  // A list or a table cannot hold marks for single words, so it shows as it was and as it is.
  const before = block.previous ? [{ op: 'removed' as const, node: block.previous }] : [];
  return [...before, { op: 'added', node }];
}

/** What a comparison shows, block by block. */
export function shownBlocks(blocks: DiffBlock[]): ShownBlock[] {
  return blocks.flatMap((block): ShownBlock[] => {
    if (!block.node) return [];
    return block.op === 'changed' ? changedBlock(block, block.node) : [{ op: block.op, node: block.node }];
  });
}

function label(text: string) {
  const element = document.createElement('span');
  element.className = 'diff-label';
  element.textContent = text;
  return element;
}

function removedWords(text: string) {
  const element = document.createElement('span');
  const words = document.createElement('del');
  words.className = 'diff-del';
  words.textContent = text;
  element.append(words, ' ');
  return element;
}

/** Where each letter of a block's text sits in the document. */
function letterPositions(node: ProseNode, start: number): number[] {
  const out: number[] = [];
  node.descendants((child, pos) => {
    if (child.isText) {
      for (let i = 0; i < child.nodeSize; i++) out.push(start + pos + i);
    }
  });
  return out;
}

function wordMarks(node: ProseNode, offset: number, words: WordOp[], index: number): Decoration[] {
  const ranges = wordRanges(node.textContent, words);
  if (!ranges) return [];
  const letters = letterPositions(node, offset + 1);
  const end = offset + node.nodeSize - 1;
  const marks = ranges.added.flatMap(([from, to]) => {
    const start = letters[from];
    const last = letters[to - 1];
    if (start === undefined || last === undefined) return [];
    return [Decoration.inline(start, last + 1, { nodeName: 'ins', class: 'diff-ins' })];
  });
  ranges.removed.forEach((run, at) => {
    marks.push(
      Decoration.widget(letters[run.at] ?? end, () => removedWords(run.text), {
        side: -1,
        key: `removed-${index}-${at}`,
      }),
    );
  });
  return marks;
}

function diffMarks(doc: ProseNode, shown: ShownBlock[], labels: Record<string, string>) {
  const marks: Decoration[] = [];
  doc.forEach((node, offset, index) => {
    const block = shown[index];
    if (!block || block.op === 'equal') return;
    marks.push(Decoration.node(offset, offset + node.nodeSize, { class: `diff-block ${block.op}` }));
    marks.push(
      Decoration.widget(offset, () => label(labels[block.op] ?? ''), { side: -1, key: `label-${index}` }),
    );
    if (block.words) marks.push(...wordMarks(node, offset, block.words, index));
  });
  return DecorationSet.create(doc, marks);
}

const DiffMarks = Extension.create<{ shown: ShownBlock[]; labels: Record<string, string> }>({
  name: 'diffMarks',
  addOptions() {
    return { shown: [], labels: {} };
  },
  addProseMirrorPlugins() {
    const { shown, labels } = this.options;
    const key = new PluginKey<DecorationSet>('diffMarks');
    return [
      new Plugin({
        key,
        // Nothing here can change, so the marks are made once.
        state: {
          init: (_config, state) => diffMarks(state.doc, shown, labels),
          apply: (_tr, marks) => marks,
        },
        props: {
          decorations: (state) => key.getState(state),
        },
      }),
    ];
  },
});

/** Shows blocks of a comparison the way the page shows them. */
export function DiffDocument({ blocks }: { blocks: DiffBlock[] }) {
  const { t } = useTranslation();
  const shown = useMemo(() => shownBlocks(blocks), [blocks]);
  const content = useMemo(() => ({ type: 'doc', content: shown.map((block) => block.node) }), [shown]);
  const editor = useEditor(
    {
      extensions: [
        ...documentExtensions(),
        DiffMarks.configure({
          shown,
          labels: {
            added: t('history.added'),
            removed: t('history.removed'),
            changed: t('history.changed'),
          },
        }),
      ],
      content,
      editable: false,
      injectCSS: false,
      editorProps: { attributes: { class: 'prose', 'aria-label': t('history.changes') } },
    },
    [content],
  );
  return <EditorContent editor={editor} />;
}
