// SPDX-License-Identifier: Apache-2.0
// Comments under a page, and the panel with every thread.
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Comment, type CommentPage, type Json, type PageDetail } from '../../api';
import { jumpToComment } from '../editor/outline';
import { IconClose } from '../icons';
import { useToast } from '../toast';
import { ErrorNote, Loading, RelativeTime, Tabs } from '../ui';
import { CommentBox } from './comment-box';
import { commentsKey, Thread } from './thread';

const THREADS_PER_LOAD = 30;
const PANEL_TABS = ['open', 'resolved', 'unread', 'page'] as const;
type PanelTab = (typeof PANEL_TABS)[number];

export const commentsQuery = (pageId: string, order: string, shown = THREADS_PER_LOAD) => ({
  queryKey: [...commentsKey(pageId), order, shown],
  queryFn: () => api.get<CommentPage>(`/pages/${pageId}/comments?order=${order}&limit=${shown}&offset=0`),
  placeholderData: (previous: CommentPage | undefined) => previous,
});

function wordsOf(document: Json | null): string {
  const node = document as { text?: string; content?: Json[] } | null;
  if (!node) return '';
  return node.text ?? (node.content ?? []).map(wordsOf).join(' ');
}

/** The threads of one tab. */
export function threadsFor(tab: PanelTab, data: CommentPage): Comment[] {
  const all = [...data.inline, ...data.threads];
  if (tab === 'page') return data.threads;
  if (tab === 'unread') return all.filter((c) => c.unread || c.replies.some((r) => r.unread));
  return data.inline.filter((c) => c.status === tab);
}

export function CommentSection({ page, order }: { page: PageDetail; order: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const [shown, setShown] = useState(THREADS_PER_LOAD);
  const q = useQuery(commentsQuery(page.id, order, shown));
  const post = useMutation({
    mutationFn: (body: Json) => api.post(`/pages/${page.id}/comments`, { body }),
    onSuccess: () => client.invalidateQueries({ queryKey: commentsKey(page.id) }),
  });
  const canWrite = !!page.can['comment.add'] && page.features.comments && !q.data?.closed;
  const threads = q.data?.threads ?? [];
  const end = useRef<HTMLDivElement>(null);

  // Reaching the end of the page counts as seeing its comments.
  useEffect(() => {
    const target = end.current;
    if (!target || (q.data?.unread ?? 0) === 0) return;
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      observer.disconnect();
      void api.put(`/pages/${page.id}/comments/seen`);
    });
    observer.observe(target);
    return () => observer.disconnect();
  }, [page.id, q.data?.unread]);

  return (
    <section className="comments" aria-label={t('comments.title')}>
      <h2>{t('comments.title')}</h2>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && threads.length === 0 && <p className="muted">{t('comments.none')}</p>}
      <ol className="thread-list">
        {threads.map((thread) => (
          <Thread key={thread.id} page={page} thread={thread} canReply={canWrite} />
        ))}
      </ol>
      {(q.data?.total ?? 0) > threads.length && (
        <button type="button" onClick={() => setShown(shown + THREADS_PER_LOAD)}>
          {t('comments.showMore')}
        </button>
      )}
      <ErrorNote error={post.error} />
      {canWrite ? (
        <CommentBox
          pageId={page.id}
          draftKey={`${page.id}:page`}
          label={t('comments.write')}
          submitLabel={t('comments.post')}
          busy={post.isPending}
          canAttach={page.can['attachment.add']}
          onSubmit={(body, clear) =>
            post.mutate(body, {
              onSuccess: () => {
                clear();
                toast({ message: t('comments.added') });
              },
            })
          }
        />
      ) : (
        page.can['comment.add'] && <p className="muted">{t('comments.closed')}</p>
      )}
      <div ref={end} />
    </section>
  );
}

function Row({
  thread,
  found,
  onOpen,
}: {
  thread: Comment;
  found: boolean;
  onOpen: (thread: Comment) => void;
}) {
  const { t } = useTranslation();
  const quote = thread.anchor?.exact;
  return (
    <li>
      <button type="button" className="comment-row" onClick={() => onOpen(thread)}>
        <strong>{thread.author_name || t('common.unknown')}</strong>{' '}
        <span className="muted">
          <RelativeTime value={thread.created_at} />
        </span>
        {quote && <q className="comment-quote">{quote}</q>}
        <span>{thread.removed ? t('comments.removed') : wordsOf(thread.body)}</span>
        {thread.replies.length > 0 && (
          <span className="muted">{t('comments.replies', { count: thread.replies.length })}</span>
        )}
        {thread.anchor?.kind === 'text' && thread.status === 'open' && !found && (
          <span className="lozenge">{t('comments.textGone')}</span>
        )}
        {thread.anchor?.kind === 'image' && <span className="lozenge">{t('comments.onImage')}</span>}
        {thread.editors_only && <span className="lozenge">{t('comments.editorsOnly')}</span>}
      </button>
    </li>
  );
}

function OpenThread({
  page,
  thread,
  canReply,
  onBack,
}: {
  page: PageDetail;
  thread: Comment;
  canReply: boolean;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const change = useMutation({
    mutationFn: (to: 'resolve' | 'reopen') => api.post(`/comments/${thread.id}/${to}`),
    onSuccess: () => client.invalidateQueries({ queryKey: commentsKey(page.id) }),
  });
  const resolved = thread.status === 'resolved';
  return (
    <>
      <button type="button" className="ghost" onClick={onBack}>
        {t('comments.backToList')}
      </button>
      {thread.anchor?.exact && <q className="comment-quote">{thread.anchor.exact}</q>}
      <ErrorNote error={change.error} />
      <ol className="thread-list">
        <Thread page={page} thread={thread} canReply={canReply} />
      </ol>
      {thread.can.resolve && (
        <button type="button" onClick={() => change.mutate(resolved ? 'reopen' : 'resolve')}>
          {resolved ? t('comments.reopen') : t('comments.resolve')}
        </button>
      )}
    </>
  );
}

function PanelFrame({ onClose, children }: { onClose: () => void; children: ReactNode }) {
  const { t } = useTranslation();
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    const opener = document.activeElement;
    heading.current?.focus();
    return () => {
      if (opener instanceof HTMLElement) opener.focus();
    };
  }, []);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);
  return createPortal(
    <aside className="details-panel comments-panel" aria-labelledby="comments-title">
      <header className="dialog-head">
        <h2 id="comments-title" ref={heading} tabIndex={-1}>
          {t('comments.title')}
        </h2>
        <button type="button" className="icon" aria-label={t('common.close')} onClick={onClose}>
          <IconClose />
        </button>
      </header>
      {children}
    </aside>,
    document.body,
  );
}

/** Every thread of a page, beside it (spec 010, C3). */
export function CommentsPanel({
  page,
  order,
  found,
  openId,
  onClose,
}: {
  page: PageDetail;
  order: string;
  found: string[];
  openId: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const q = useQuery(commentsQuery(page.id, order));
  const [tab, setTab] = useState<PanelTab>('open');
  const [picked, setPicked] = useState<string | null>(openId);
  const [byPosition, setByPosition] = useState(false);
  const canReply = !!page.can['comment.add'] && page.features.comments && !q.data?.closed;

  const [lastOpen, setLastOpen] = useState(openId);
  if (openId !== lastOpen) {
    setLastOpen(openId);
    setPicked(openId);
  }

  useEffect(() => {
    void api.put(`/pages/${page.id}/comments/seen`);
  }, [page.id]);

  const list = useMemo(() => {
    if (!q.data) return [];
    const threads = threadsFor(tab, q.data);
    if (!byPosition) return threads;
    return [...threads].sort((a, b) => (a.anchor?.offset ?? 0) - (b.anchor?.offset ?? 0));
  }, [q.data, tab, byPosition]);
  const open = q.data && [...q.data.inline, ...q.data.threads].find((c) => c.id === picked);

  return (
    <PanelFrame onClose={onClose}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {open ? (
        <OpenThread page={page} thread={open} canReply={canReply} onBack={() => setPicked(null)} />
      ) : (
        <>
          <Tabs
            names={PANEL_TABS}
            current={tab}
            label={t('comments.filter')}
            nameOf={(name) => t(`comments.tab_${name}`)}
            onChange={setTab}
          />
          <label className="check">
            <input type="checkbox" checked={byPosition} onChange={(e) => setByPosition(e.target.checked)} />
            {t('comments.byPosition')}
          </label>
          {q.data && list.length === 0 && <p className="muted">{t('comments.noneHere')}</p>}
          <ul className="plain-list">
            {list.map((thread) => (
              <Row
                key={thread.id}
                thread={thread}
                found={found.includes(thread.id)}
                onOpen={(c) => {
                  setPicked(c.id);
                  jumpToComment(c.id);
                }}
              />
            ))}
          </ul>
        </>
      )}
    </PanelFrame>
  );
}
