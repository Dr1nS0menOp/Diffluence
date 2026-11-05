// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Comment, type Json, type PageDetail } from '../../api';
import { DocumentView } from '../editor/editor';
import { useToast } from '../toast';
import { Avatar, ConfirmDialog, ErrorNote, RelativeTime } from '../ui';
import { Lightbox, type ShownImage } from '../viewers';
import { CommentBox } from './comment-box';
import { Reactions } from './reactions';

export const commentsKey = (pageId: string) => ['comments', pageId];

function useRefresh(pageId: string) {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: commentsKey(pageId) });
}

function Item({ page, comment, onReply }: { page: PageDetail; comment: Comment; onReply?: () => void }) {
  const { t } = useTranslation();
  const toast = useToast();
  const refresh = useRefresh(page.id);
  const [editing, setEditing] = useState(false);
  const [asking, setAsking] = useState(false);
  const [viewing, setViewing] = useState<{ images: ShownImage[]; start: number } | null>(null);
  const edit = useMutation({
    mutationFn: (body: Json) => api.patch(`/comments/${comment.id}`, { body }),
    onSuccess: async () => {
      setEditing(false);
      await refresh();
    },
  });
  const undo = async () => {
    await api.post(`/comments/${comment.id}/undo-delete`);
    await refresh();
  };
  const remove = useMutation({
    mutationFn: () => api.del(`/comments/${comment.id}`),
    onSuccess: async () => {
      await refresh();
      toast({
        message: t('comments.deleted'),
        action: { label: t('page.undo'), run: () => void undo() },
      });
    },
  });
  const name = comment.author_name || t('common.unknown');
  if (comment.removed) {
    return (
      <p id={`comment-${comment.id}`} className="muted comment-removed">
        {t('comments.removed')}
      </p>
    );
  }
  return (
    <article id={`comment-${comment.id}`} className="comment" aria-label={t('comments.by', { name })}>
      <Avatar name={name} size="small" />
      <div className="comment-main">
        <div className="comment-head">
          <strong>{name}</strong>
          {!comment.author_active && <span className="muted">{t('comments.inactive')}</span>}
          <a className="comment-date" href={`#comment-${comment.id}`}>
            <RelativeTime value={comment.created_at} />
          </a>
          {comment.edited_at && <span className="muted">{t('comments.edited')}</span>}
          {comment.unread && <span className="comment-new">{t('comments.new')}</span>}
        </div>
        {editing ? (
          <CommentBox
            pageId={page.id}
            draftKey={`${comment.id}:edit`}
            label={t('comments.editLabel')}
            submitLabel={t('common.save')}
            initial={comment.body ?? undefined}
            busy={edit.isPending}
            autoFocus
            canAttach={page.can['attachment.add']}
            onSubmit={(body) => edit.mutate(body)}
            onCancel={() => setEditing(false)}
          />
        ) : (
          comment.body && (
            <DocumentView
              document={comment.body}
              plain
              onImage={(images, start) => setViewing({ images, start })}
            />
          )
        )}
        <ErrorNote error={edit.error ?? remove.error} />
        <div className="comment-actions">
          {page.features.reactions && (
            <Reactions
              path={`/comments/${comment.id}`}
              queryKey={commentsKey(page.id)}
              initial={comment.reactions}
              readOnly={!page.can['reaction.set']}
            />
          )}
          {onReply && (
            <button type="button" className="ghost" onClick={onReply}>
              {t('comments.reply')}
            </button>
          )}
          {comment.can.edit && !editing && (
            <button type="button" className="ghost" onClick={() => setEditing(true)}>
              {t('common.edit')}
            </button>
          )}
          {comment.can.delete && (
            <button type="button" className="ghost" onClick={() => setAsking(true)}>
              {t('common.delete')}
            </button>
          )}
        </div>
      </div>
      {viewing && <Lightbox images={viewing.images} start={viewing.start} onClose={() => setViewing(null)} />}
      {asking && (
        <ConfirmDialog
          title={t('comments.deleteTitle')}
          message={t('comments.confirmDelete')}
          confirmLabel={t('common.delete')}
          onConfirm={() => remove.mutate()}
          onClose={() => setAsking(false)}
        />
      )}
    </article>
  );
}

/** A comment with its replies, which form one level. */
export function Thread({ page, thread, canReply }: { page: PageDetail; thread: Comment; canReply: boolean }) {
  const { t } = useTranslation();
  const refresh = useRefresh(page.id);
  const [replying, setReplying] = useState(false);
  const reply = useMutation({
    mutationFn: (body: Json) => api.post(`/pages/${page.id}/comments`, { body, parent_id: thread.id }),
    onSuccess: refresh,
  });
  const openReply = canReply ? () => setReplying(true) : undefined;
  return (
    <li className="thread">
      <Item page={page} comment={thread} onReply={thread.removed ? undefined : openReply} />
      {thread.replies.length > 0 && (
        <ol className="thread-replies">
          {thread.replies.map((r) => (
            <li key={r.id}>
              <Item page={page} comment={r} onReply={openReply} />
            </li>
          ))}
        </ol>
      )}
      {replying && (
        <div className="thread-replies">
          <ErrorNote error={reply.error} />
          <CommentBox
            pageId={page.id}
            draftKey={`${page.id}:${thread.id}`}
            label={t('comments.replyLabel')}
            submitLabel={t('comments.reply')}
            busy={reply.isPending}
            autoFocus
            canAttach={page.can['attachment.add']}
            onSubmit={(body, clear) =>
              reply.mutate(body, {
                onSuccess: () => {
                  clear();
                  setReplying(false);
                },
              })
            }
            onCancel={() => setReplying(false)}
          />
        </div>
      )}
    </li>
  );
}
