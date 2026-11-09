// SPDX-License-Identifier: Apache-2.0
// Every block with a preview (EDT-020). A preview is a small document, shown by the same
// code as a page, so it can never look different from the real thing.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import type { Json } from '../../api';
import { offered, useCapabilities } from '../../capabilities';
import { Dialog } from '../ui';
import { DocumentView } from './editor';
import { arrangedItems, INSERT_ITEMS, type ArrangedItem } from './insert';
import type { EditorActions } from './keys';
import { FIRST_PANEL_EMOJI } from './palette';

const ITEM_ICON_SIZE = 18;

const line = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const heading = (level: number) => ({
  type: 'heading',
  attrs: { level },
  content: [{ type: 'text', text: 'A heading' }],
});
const list = (type: string, item: string) => ({
  type,
  content: ['First', 'Second'].map((text) => ({ type: item, content: [line(text)] })),
});
const panel = (kind: string) => ({ type: 'panel', attrs: { kind }, content: [line('Something to notice')] });
const twoColumns = (layout: string | null) => ({
  type: 'columns',
  attrs: layout ? { layout } : {},
  content: ['Left', 'Right'].map((text) => ({ type: 'column', content: [line(text)] })),
});

// Blocks that show data of a page or a space have no sample of their own: their hint says what they do.
const SAMPLES: Record<string, unknown> = {
  text: line('Ordinary text'),
  h1: heading(1),
  h2: heading(2),
  h3: heading(3),
  h4: heading(4),
  h5: heading(5),
  h6: heading(6),
  bullet: list('bulletList', 'listItem'),
  ordered: list('orderedList', 'listItem'),
  task: {
    type: 'taskList',
    content: ['Open', 'Done'].map((text, i) => ({
      type: 'taskItem',
      attrs: { checked: i === 1 },
      content: [line(text)],
    })),
  },
  quote: { type: 'blockquote', content: [line('Words of someone else')] },
  code: { type: 'codeBlock', attrs: { language: 'js' }, content: [{ type: 'text', text: 'const a = 1;' }] },
  rule: { type: 'horizontalRule' },
  panelInfo: panel('info'),
  panelNote: panel('note'),
  panelSuccess: panel('success'),
  panelWarning: panel('warning'),
  panelError: panel('error'),
  panelNeutral: {
    type: 'panel',
    attrs: { kind: 'neutral', emoji: FIRST_PANEL_EMOJI },
    content: [line('Your own emoji')],
  },
  spotlight: {
    type: 'spotlight',
    attrs: { color: 'blue' },
    content: [heading(2), line('An important message')],
  },
  expand: {
    type: 'expand',
    attrs: { title: 'Open to read more', open: true },
    content: [line('Hidden text')],
  },
  columns2: twoColumns(null),
  columns3: {
    type: 'columns',
    content: ['One', 'Two', 'Three'].map((text) => ({ type: 'column', content: [line(text)] })),
  },
  columnsSidebar: twoColumns('sidebar-end'),
  decision: { type: 'decision', content: [{ type: 'text', text: 'We go with the second option' }] },
  table: {
    type: 'table',
    content: [0, 1].map((row) => ({
      type: 'tableRow',
      content: [0, 1].map((column) => ({
        type: 'tableCell',
        content: [line(`Cell ${row * 2 + column + 1}`)],
      })),
    })),
  },
  status: {
    type: 'paragraph',
    content: [{ type: 'status', attrs: { text: 'In progress', color: 'blue' } }],
  },
  date: { type: 'paragraph', content: [{ type: 'date', attrs: { value: '2026-11-07' } }] },
};

function Preview({ item }: { item: ArrangedItem }) {
  const { t } = useTranslation();
  const sample = SAMPLES[item.id];
  return (
    <div className="browse-preview" aria-live="polite">
      <h3>{t(`insert.${item.id}.name`)}</h3>
      <p className="muted">{t(`insert.${item.id}.hint`)}</p>
      {sample !== undefined && (
        <DocumentView key={item.id} plain document={{ type: 'doc', content: [sample] } as Json} />
      )}
    </div>
  );
}

export function BrowseDialog({
  editor,
  actions,
  onClose,
}: {
  editor: Editor;
  actions: EditorActions;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const items = arrangedItems(offered(INSERT_ITEMS, useCapabilities()), []);
  const [shown, setShown] = useState<ArrangedItem | undefined>(items[0]);
  const groups = [...new Set(items.map((item) => item.group))];
  return (
    <Dialog title={t('editor.browseAll')} onClose={onClose} wide>
      <div className="browse">
        <div className="browse-list">
          {groups.map((group) => (
            <section key={group} aria-label={t(`insert.group_${group}`)}>
              <h3 className="suggestion-group">{t(`insert.group_${group}`)}</h3>
              <ul className="plain-list">
                {items
                  .filter((item) => item.group === group)
                  .map((item) => (
                    <li key={item.id}>
                      <button
                        type="button"
                        className={item.id === shown?.id ? 'suggestion active' : 'suggestion'}
                        onMouseEnter={() => setShown(item)}
                        onFocus={() => setShown(item)}
                        onClick={() => {
                          onClose();
                          item.run(editor, actions);
                        }}
                      >
                        <item.icon size={ITEM_ICON_SIZE} />
                        <span className="suggestion-name">{t(`insert.${item.id}.name`)}</span>
                      </button>
                    </li>
                  ))}
              </ul>
            </section>
          ))}
        </div>
        {shown && <Preview item={shown} />}
      </div>
    </Dialog>
  );
}
