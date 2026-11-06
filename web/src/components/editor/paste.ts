// SPDX-License-Identifier: Apache-2.0
// Pasted Markdown turns into blocks (EDT-140). The plain text goes in at once, as it
// would anyway, and the server's converter replaces it in a step of its own: Undo brings
// back the plain text, and when the server does not answer the text simply stays.
import type { Editor, JSONContent } from '@tiptap/core';
import { closeHistory } from '@tiptap/pm/history';
import { api, type Json } from '../../api';
import { endUndoStep } from './together';

// One sign is enough here: nobody writes these by accident.
const CLEAR_SIGNS = [
  /^#{1,6} \S/m,
  /^```/m,
  /^\|.*\|[ \t]*\n\|[\s:|-]+\|/m,
  /\[[^\]\n]+\]\(https?:\/\/[^)\s]+\)/,
  /\*\*[^*\n]+\*\*/,
];
const LIST_LINE = /^\s*(?:[-*+]|\d+\.) \S/;
const LIST_LINES_NEEDED = 2;

/** Asks for clear signs: taking plain text for Markdown would change what someone wrote. */
export function looksLikeMarkdown(text: string): boolean {
  if (CLEAR_SIGNS.some((sign) => sign.test(text))) return true;
  return text.split('\n').filter((line) => LIST_LINE.test(line)).length >= LIST_LINES_NEEDED;
}

function replaceWithBlocks(editor: Editor, from: number, to: number, blocks: JSONContent[]) {
  const { doc } = editor.state;
  const $from = doc.resolve(from);
  const $to = doc.resolve(to);
  // Text that fills its blocks goes with them, or an empty line would stay behind.
  const fills = $from.parentOffset === 0 && $to.parentOffset === $to.parent.content.size;
  endUndoStep(editor.state);
  editor
    .chain()
    .command(({ tr }) => {
      closeHistory(tr);
      return true;
    })
    .insertContentAt(fills ? { from: from - 1, to: to + 1 } : { from, to }, blocks)
    .run();
}

/** Call this while the paste happens: the text is in the document right after it. */
export function convertPastedMarkdown(editor: Editor, markdown: string) {
  const { from } = editor.state.selection;
  queueMicrotask(() => {
    const pasted = editor.state.doc;
    const { to } = editor.state.selection;
    api.post<{ document: Json }>('/markdown/document', { markdown }).then(
      ({ document }) => {
        // Someone typed on: what they wrote now counts for more than the blocks.
        if (editor.isDestroyed || !editor.state.doc.eq(pasted)) return;
        replaceWithBlocks(editor, from, to, (document.content ?? []) as JSONContent[]);
      },
      () => undefined, // The plain text is already there, which is what we promise.
    );
  });
}
