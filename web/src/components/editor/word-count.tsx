// SPDX-License-Identifier: Apache-2.0
// The size of the text, shown on request at the foot of the sheet (EDT-011, EDT-151).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { useEditorState } from '@tiptap/react';
import { readingMinutes } from './outline';

const BLOCK_SEPARATOR = ' ';

export const wordsIn = (text: string) => text.split(/\s+/).filter(Boolean).length;

function WordCountLine({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const { words, selected } = useEditorState({
    editor,
    selector: ({ editor: e }) => {
      const { doc, selection } = e.state;
      const between = (from: number, to: number) =>
        wordsIn(doc.textBetween(from, to, BLOCK_SEPARATOR, BLOCK_SEPARATOR));
      return {
        words: between(0, doc.content.size),
        selected: selection.empty ? null : between(selection.from, selection.to),
      };
    },
  });
  const minutes = readingMinutes(words);
  return (
    <span role="status" className="muted">
      {selected === null
        ? t('editor.words', { count: words })
        : t('editor.wordsSelected', { selected, count: words })}
      {minutes !== null && ` · ${t('page.readTime', { minutes })}`}
    </span>
  );
}

export function WordCount({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <div className="word-count">
      <button
        type="button"
        className="quiet"
        aria-expanded={open}
        // The selection must stay, because that is what gets counted.
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        {t('editor.wordCount')}
      </button>
      {open && <WordCountLine editor={editor} />}
    </div>
  );
}
