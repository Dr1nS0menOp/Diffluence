// SPDX-License-Identifier: Apache-2.0
// Selecting text in a page shows one small row: comment on it, or copy a link to it.
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import type { EditorView } from '@tiptap/pm/view';
import { api, type Comment, type CommentAnchor, type Json, type PageDetail } from '../../api';
import { useShortcut } from '../shortcuts';
import { useCopy } from '../toast';
import { Dialog, ErrorNote } from '../ui';
import { anchorOfSelection } from './anchor';
import { CommentBox } from './comment-box';
import { commentsKey } from './thread';

const BAR_GAP_PX = 8;

interface Shown {
  anchor: CommentAnchor;
  top: number;
  left: number;
}

function selectionPlace(): { top: number; left: number } | null {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) return null;
  const box = selection.getRangeAt(0).getBoundingClientRect();
  return { top: box.top - BAR_GAP_PX, left: box.left + box.width / 2 };
}

/** The dialog that starts a thread on a selection or an image. */
export function NewThread({
  page,
  anchor,
  editorsOnly,
  onDone,
}: {
  page: PageDetail;
  anchor: CommentAnchor;
  editorsOnly?: boolean;
  onDone: (created: Comment | null) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const post = useMutation({
    mutationFn: (body: Json) =>
      api.post<Comment>(`/pages/${page.id}/comments`, { body, anchor, editors_only: editorsOnly ?? false }),
    onSuccess: async (created) => {
      await client.invalidateQueries({ queryKey: commentsKey(page.id) });
      onDone(created);
    },
  });
  return (
    <Dialog
      title={anchor.kind === 'image' ? t('comments.onThisImage') : t('comments.onSelection')}
      onClose={() => onDone(null)}
    >
      {anchor.exact && <q className="comment-quote">{anchor.exact}</q>}
      <ErrorNote error={post.error} />
      <CommentBox
        pageId={page.id}
        draftKey={`${page.id}:selection`}
        label={t('comments.write')}
        submitLabel={t('comments.post')}
        busy={post.isPending}
        autoFocus
        canAttach={page.can['attachment.add']}
        onSubmit={(body, clear) => post.mutate(body, { onSuccess: clear })}
        onCancel={() => onDone(null)}
      />
    </Dialog>
  );
}

export function SelectionBar({
  page,
  view,
  onCreated,
}: {
  page: PageDetail;
  view: () => EditorView | null;
  onCreated: (id: string) => void;
}) {
  const { t } = useTranslation();
  const copy = useCopy();
  const bar = useRef<HTMLDivElement>(null);
  const [shown, setShown] = useState<Shown | null>(null);
  const [writing, setWriting] = useState<CommentAnchor | null>(null);
  const canComment = page.can['comment.add'] && page.features.comments && !page.comments_closed;

  const read = () => {
    const editor = view();
    const anchor = editor && anchorOfSelection(editor, page.current_version);
    const place = selectionPlace();
    return anchor && place ? { anchor, ...place } : null;
  };

  useEffect(() => {
    const update = () => setShown(read());
    document.addEventListener('selectionchange', update);
    return () => document.removeEventListener('selectionchange', update);
  });

  useEffect(() => {
    if (!bar.current || !shown) return;
    bar.current.style.top = `${shown.top}px`;
    bar.current.style.left = `${shown.left}px`;
  }, [shown]);

  useShortcut('comment', () => {
    const selected = read();
    if (selected && canComment) {
      setWriting(selected.anchor);
      return;
    }
    document.getElementById('comment-box')?.focus();
  });

  const address = () =>
    `${window.location.origin}/pages/${page.id}#:~:text=${encodeURIComponent(shown?.anchor.exact ?? '')}`;
  return (
    <>
      {shown && !writing && (
        <div ref={bar} className="selection-bar">
          {canComment && (
            <button
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => setWriting(shown.anchor)}
            >
              {t('comments.comment')}
            </button>
          )}
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => void copy(address(), t('tree.linkCopied'))}
          >
            {t('comments.copyLink')}
          </button>
        </div>
      )}
      {writing && (
        <NewThread
          page={page}
          anchor={writing}
          onDone={(created) => {
            setWriting(null);
            if (created) onCreated(created.id);
          }}
        />
      )}
    </>
  );
}
