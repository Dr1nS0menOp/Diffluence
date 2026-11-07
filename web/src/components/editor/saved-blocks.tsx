// SPDX-License-Identifier: Apache-2.0
// Blocks a person keeps to use again (EDT-026, spec 027, D16).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Editor, JSONContent } from '@tiptap/core';
import { api } from '../../api';
import { IconCopy } from '../icons';
import { Dialog, ErrorNote, Field } from '../ui';
import { blockAtCursor, blockRange } from './block-commands';
import type { InsertItem } from './insert';

export interface SavedBlock {
  id: string;
  name: string;
  document: { content?: JSONContent[] };
  shared: boolean;
  is_own: boolean;
}

const SAVED_KEY = ['saved-blocks'];

export function useSavedBlocks() {
  return useQuery({
    queryKey: SAVED_KEY,
    queryFn: () => api.get<{ blocks: SavedBlock[] }>('/saved-blocks'),
    select: (found) => found.blocks,
  });
}

/** A saved block as an entry of the slash menu. Its name is a person's, so it is not translated. */
export function savedBlockItem(block: SavedBlock): InsertItem {
  return {
    id: `saved_${block.id}`,
    label: block.name,
    icon: IconCopy,
    run: (editor) => {
      editor
        .chain()
        .focus()
        .insertContent(block.document.content ?? [])
        .run();
    },
  };
}

/** What a saved block would hold: the blocks at the cursor or in the selection. */
export function selectedBlocks(editor: Editor): JSONContent {
  const pos = blockAtCursor(editor);
  const range = pos === null ? null : blockRange(editor, pos);
  const { doc } = editor.state;
  const content = range ? (doc.slice(range.from, range.to).content.toJSON() as JSONContent[]) : [];
  return { type: 'doc', content };
}

export function SavedBlocksDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const [shared, setShared] = useState(false);
  const blocks = useSavedBlocks();
  const refresh = () => client.invalidateQueries({ queryKey: SAVED_KEY });
  const save = useMutation({
    mutationFn: () => api.post('/saved-blocks', { name, shared, document: selectedBlocks(editor) }),
    onSuccess: refresh,
  });
  const share = useMutation({
    mutationFn: (block: SavedBlock) => api.patch(`/saved-blocks/${block.id}`, { shared: !block.shared }),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (block: SavedBlock) => api.del(`/saved-blocks/${block.id}`),
    onSuccess: refresh,
  });
  const own = blocks.data?.filter((block) => block.is_own) ?? [];
  return (
    <Dialog title={t('editor.saveBlock')} onClose={onClose}>
      <ErrorNote error={save.error ?? share.error ?? remove.error} />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(undefined, { onSuccess: onClose });
        }}
      >
        <Field label={t('editor.savedBlockName')}>
          <input required autoFocus maxLength={100} value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <label className="check">
          <input type="checkbox" checked={shared} onChange={(e) => setShared(e.target.checked)} />
          {t('editor.savedBlockShared')}
        </label>
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={save.isPending}>
            {t('common.save')}
          </button>
        </div>
      </form>
      {own.length > 0 && (
        <section aria-label={t('editor.savedBlocks')}>
          <h3>{t('editor.savedBlocks')}</h3>
          <ul className="plain-list">
            {own.map((block) => (
              <li key={block.id} className="row">
                <span className="grow">{block.name}</span>
                <label className="check">
                  <input type="checkbox" checked={block.shared} onChange={() => share.mutate(block)} />
                  {t('editor.savedBlockSharedShort')}
                </label>
                <button type="button" className="ghost" onClick={() => remove.mutate(block)}>
                  {t('common.delete')}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </Dialog>
  );
}
