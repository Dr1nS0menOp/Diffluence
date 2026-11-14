// SPDX-License-Identifier: Apache-2.0
// The review panel of a controlled page (spec 064, DOC-250, DOC-251, DOC-921, DOC-922): the
// rule and the count in words, who was asked with their answers as a table, the change since
// the approved version, the comments of the round, and the answers as buttons.
import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageDetail, type ReviewPanelData, type ReviewerRow } from '../api';
import { DiffDocument } from './editor/diff-view';
import { IconClose } from './icons';
import { SubjectPicker } from './picker';
import { TableBox } from './table-box';
import { Dialog, ErrorNote, Field, Loading, RelativeTime } from './ui';

type Answer = 'approve' | 'changes';

export const reviewKey = (pageId: string) => ['review', pageId];

/** The panel beside a page, in the place of Details and comments: one of them at a time. */
export function ReviewPanel({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const heading = useRef<HTMLHeadingElement>(null);
  const q = useQuery({
    queryKey: reviewKey(page.id),
    queryFn: () => api.get<ReviewPanelData>(`/pages/${page.id}/review`),
  });

  useEffect(() => {
    const opener = document.activeElement;
    heading.current?.focus();
    return () => {
      if (opener instanceof HTMLElement) opener.focus();
    };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // A dialog of the panel takes its own Escape first.
      if (e.key === 'Escape' && !document.querySelector('dialog[open]')) onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  return createPortal(
    <aside className="details-panel review-panel" aria-labelledby="review-title">
      <header className="dialog-head">
        <h2 id="review-title" ref={heading} tabIndex={-1}>
          {t('review.title')}
        </h2>
        <button type="button" className="icon" aria-label={t('common.close')} onClick={onClose}>
          <IconClose />
        </button>
      </header>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && <ReviewBody page={page} panel={q.data} />}
    </aside>,
    document.body,
  );
}

function ruleText(
  t: (key: string, values?: Record<string, unknown>) => string,
  panel: ReviewPanelData,
): string {
  if (panel.rule === 'any') return t('review.ruleAny');
  if (panel.rule === 'quorum') return t('review.ruleQuorum', { count: panel.needed });
  return t('review.ruleAll');
}

function ReviewBody({ page, panel }: { page: PageDetail; panel: ReviewPanelData }) {
  const { t } = useTranslation();
  const [answering, setAnswering] = useState<Answer | null>(null);
  const [removing, setRemoving] = useState<ReviewerRow | null>(null);
  if (!panel.round_id) return <p className="muted">{t('review.noRound')}</p>;
  const active = panel.reviewers.filter((r) => !r.removed);
  return (
    <>
      <p>
        <strong>{panel.state_name}</strong>
      </p>
      {panel.in_review ? (
        <p role="status">
          {ruleText(t, panel)} {t('review.count', { approved: panel.approved, needed: panel.needed })}
        </p>
      ) : (
        <p className="muted">
          {t(`review.status_${panel.status}`, { defaultValue: t('review.status_other') })}
        </p>
      )}
      {panel.in_review && (
        <p className="muted">{panel.independent ? t('review.independent') : t('review.notIndependent')}</p>
      )}
      {panel.can.answer && (
        <div className="review-actions">
          {panel.my_answer && <p>{t(`review.mine_${panel.my_answer}`)}</p>}
          <button type="button" className="primary" onClick={() => setAnswering('approve')}>
            {t('review.approve')}
          </button>
          <button type="button" onClick={() => setAnswering('changes')}>
            {t('review.askChanges')}
          </button>
        </div>
      )}
      <h3>{t('review.reviewers')}</h3>
      {active.length === 0 && panel.reviewers.length === 0 ? (
        <p className="muted">{t('review.nobody')}</p>
      ) : (
        <ReviewerTable panel={panel} onRemove={setRemoving} />
      )}
      {panel.in_review && panel.can.add && <AddReviewer pageId={page.id} />}
      <h3>{t('review.change')}</h3>
      <ReviewChange panel={panel} />
      <h3>{t('review.comments')}</h3>
      <ReviewComments pageId={page.id} panel={panel} />
      {answering && (
        <AnswerDialog page={page} panel={panel} answer={answering} onClose={() => setAnswering(null)} />
      )}
      {removing && <RemoveDialog page={page} reviewer={removing} onClose={() => setRemoving(null)} />}
    </>
  );
}

function ReviewerTable({ panel, onRemove }: { panel: ReviewPanelData; onRemove: (r: ReviewerRow) => void }) {
  const { t } = useTranslation();
  return (
    <TableBox>
      <table className="review-table">
        <thead>
          <tr>
            <th scope="col">{t('review.columnName')}</th>
            <th scope="col">{t('review.columnAsked')}</th>
            <th scope="col">{t('review.columnAnswer')}</th>
            {panel.can.remove && panel.in_review && (
              <th scope="col">
                <span className="visually-hidden">{t('review.columnActions')}</span>
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          {panel.reviewers.map((r) => (
            <tr key={r.user_id}>
              <td>{r.name}</td>
              <td>
                {r.source_name
                  ? t(`review.source_${r.source}_named`, { name: r.source_name })
                  : t(`review.source_${r.source}`)}
              </td>
              <td>
                {r.removed ? (
                  <>
                    {t('review.removed')} <span className="muted">{r.removed_reason}</span>
                  </>
                ) : (
                  t(`review.answer_${r.answer || 'none'}`)
                )}
                {r.comment && <p className="review-comment">{r.comment}</p>}
              </td>
              {panel.can.remove && panel.in_review && (
                <td>
                  {!r.removed && (
                    <button type="button" className="ghost" onClick={() => onRemove(r)}>
                      {t('review.remove')}
                    </button>
                  )}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}

/** The change since the approved version, which is what readers will receive (DOC-250). */
function ReviewChange({ panel }: { panel: ReviewPanelData }) {
  const { t } = useTranslation();
  const { base, head } = panel;
  if (!head) return <p className="muted">{t('review.noText')}</p>;
  return (
    <>
      <p className="muted">
        {base
          ? t('review.since', { from: base.version, to: head.version })
          : t('review.firstVersion', { number: head.version })}
      </p>
      <DiffDocument blocks={panel.changes ?? []} />
    </>
  );
}

function ReviewComments({ pageId, panel }: { pageId: string; panel: ReviewPanelData }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [body, setBody] = useState('');
  const add = useMutation({
    mutationFn: () => api.post(`/pages/${pageId}/review/comments`, { body }),
    onSuccess: async () => {
      setBody('');
      await client.invalidateQueries({ queryKey: reviewKey(pageId) });
    },
  });
  return (
    <>
      {panel.comments.length === 0 ? (
        <p className="muted">{t('review.noComments')}</p>
      ) : (
        <ul className="plain-list review-comments">
          {panel.comments.map((c) => (
            <li key={c.id}>
              <strong>{c.name}</strong> <RelativeTime value={c.created_at} />
              <p>{c.body}</p>
            </li>
          ))}
        </ul>
      )}
      {panel.can.comment && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            add.mutate();
          }}
        >
          <Field label={t('review.commentLabel')} help={t('review.commentHelp')}>
            <textarea value={body} onChange={(e) => setBody(e.target.value)} />
          </Field>
          <ErrorNote error={add.error} />
          <button type="submit" disabled={add.isPending || body.trim() === ''}>
            {t('review.commentAdd')}
          </button>
        </form>
      )}
    </>
  );
}

function useReviewDone(page: PageDetail, onClose: () => void) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: reviewKey(page.id) });
    await client.invalidateQueries({ queryKey: ['page', page.id] });
    await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    onClose();
  };
}

/** The comment comes first and has focus; Escape closes without answering (DOC-922). */
function AnswerDialog({
  page,
  panel,
  answer,
  onClose,
}: {
  page: PageDetail;
  panel: ReviewPanelData;
  answer: Answer;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [comment, setComment] = useState('');
  const field = useRef<HTMLTextAreaElement>(null);
  // The dialog opens as a modal after its children mount and then focuses its first
  // control, so the comment field takes the focus one frame later (DOC-922).
  useEffect(() => {
    const frame = requestAnimationFrame(() => field.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, []);
  const done = useReviewDone(page, onClose);
  const send = useMutation({
    mutationFn: () =>
      api.post(`/pages/${page.id}/review/answer`, { answer, comment, version: panel.head?.version ?? 0 }),
    onSuccess: done,
  });
  const label = answer === 'approve' ? t('review.approve') : t('review.askChanges');
  const needsComment = answer === 'changes';
  return (
    <Dialog title={label} onClose={onClose}>
      <Field label={needsComment ? t('review.answerCommentNeeded') : t('review.answerComment')}>
        <textarea ref={field} value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      <ErrorNote error={send.error} />
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button
          type="button"
          className="primary"
          disabled={send.isPending || (needsComment && comment.trim() === '')}
          onClick={() => send.mutate()}
        >
          {label}
        </button>
      </div>
    </Dialog>
  );
}

/** Taking someone off a round needs a reason, and says so (DOC-202). */
function RemoveDialog({
  page,
  reviewer,
  onClose,
}: {
  page: PageDetail;
  reviewer: ReviewerRow;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [reason, setReason] = useState('');
  const done = useReviewDone(page, onClose);
  const remove = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/reviewers/${reviewer.user_id}/remove`, { reason }),
    onSuccess: done,
  });
  return (
    <Dialog title={t('review.removeTitle', { name: reviewer.name })} onClose={onClose}>
      <p>{t('review.removeHelp')}</p>
      <Field label={t('review.reason')}>
        <textarea autoFocus value={reason} onChange={(e) => setReason(e.target.value)} />
      </Field>
      <ErrorNote error={remove.error} />
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button
          type="button"
          className="danger"
          disabled={remove.isPending || reason.trim() === ''}
          onClick={() => remove.mutate()}
        >
          {t('review.remove')}
        </button>
      </div>
    </Dialog>
  );
}

function AddReviewer({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const add = useMutation({
    mutationFn: (id: string) => api.post(`/pages/${pageId}/reviewers`, { user_id: id }),
    onSuccess: () => client.invalidateQueries({ queryKey: reviewKey(pageId) }),
  });
  return (
    <>
      <SubjectPicker usersOnly label={t('review.add')} onPick={(e) => add.mutate(e.id)} />
      <ErrorNote error={add.error} />
    </>
  );
}
