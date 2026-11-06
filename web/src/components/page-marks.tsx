// SPDX-License-Identifier: Apache-2.0
// What a page says about its own state: status, verified, approval, archive.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ApprovalRound, type DirectoryEntry, type PageDetail, type Stage } from '../api';
import { Link } from '../router';
import { DocumentView } from './editor/editor';
import { IconChevronDown, IconVerified } from './icons';
import { Menu, MenuItem, MenuSeparator } from './menu';
import { SubjectPicker } from './picker';
import { useToast } from './toast';
import { Dialog, ErrorNote, Field, formatDate, Loading, RelativeTime } from './ui';

const MARK_ICON_SIZE = 16;

function useRefresh(page: PageDetail) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['page', page.id] });
    await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    await client.invalidateQueries({ queryKey: ['approvals', page.id] });
  };
}

/** A status: always a word and a colour, so it never rests on colour alone. */
export function StageMark({ text, color }: { text: string; color: string }) {
  if (!text) return null;
  return <span className={`status-mark mark-${color}`}>{text}</span>;
}

function OwnStageDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const refresh = useRefresh(page);
  const [text, setText] = useState('');
  const save = useMutation({
    mutationFn: () => api.put(`/pages/${page.id}/stage`, { text, color: 'grey' }),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  return (
    <Dialog title={t('organise.ownStatus')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        <Field label={t('organise.statusWord')}>
          <input value={text} maxLength={30} required autoFocus onChange={(e) => setText(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** The status by the byline. Set while reading, it applies at once (PAG-057). */
export function StageControl({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const refresh = useRefresh(page);
  const [own, setOwn] = useState(false);
  const list = useQuery({
    queryKey: ['statuses', page.space_id],
    queryFn: () => api.get<{ statuses: Stage[]; free: boolean }>(`/spaces/${page.space_id}/statuses`),
    enabled: page.can['page.edit'] ?? false,
  });
  const save = useMutation({
    mutationFn: (stage: Stage) => api.put(`/pages/${page.id}/stage`, stage),
    onSettled: refresh,
  });
  if (!page.can['page.edit']) return <StageMark text={page.stage_text} color={page.stage_color} />;
  return (
    <>
      <Menu
        label={page.stage_text ? t('organise.statusIs', { text: page.stage_text }) : t('organise.setStatus')}
        align="start"
        className="stage-button"
        trigger={
          <>
            {page.stage_text ? (
              <StageMark text={page.stage_text} color={page.stage_color} />
            ) : (
              t('organise.setStatus')
            )}
            <IconChevronDown size={14} />
          </>
        }
      >
        {list.data?.statuses.map((s) => (
          <MenuItem key={s.text} onClick={() => save.mutate(s)}>
            <StageMark text={s.text} color={s.color} />
          </MenuItem>
        ))}
        {list.data?.free && <MenuItem onClick={() => setOwn(true)}>{t('organise.ownStatus')}</MenuItem>}
        {page.stage_text && (
          <>
            <MenuSeparator />
            <MenuItem onClick={() => save.mutate({ text: '', color: '' })}>{t('organise.noStatus')}</MenuItem>
          </>
        )}
      </Menu>
      <ErrorNote error={save.error} />
      {own && <OwnStageDialog page={page} onClose={() => setOwn(false)} />}
    </>
  );
}

export function VerifyDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const refresh = useRefresh(page);
  const [until, setUntil] = useState(page.verified?.until ?? '');
  const done = async () => {
    await refresh();
    onClose();
  };
  const save = useMutation({
    mutationFn: () => api.put(`/pages/${page.id}/verification`, { until }),
    onSuccess: done,
  });
  const remove = useMutation({
    mutationFn: () => api.del(`/pages/${page.id}/verification`),
    onSuccess: done,
  });
  return (
    <Dialog title={t('organise.verify')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error ?? remove.error} />
        <Field label={t('organise.verifiedUntil')}>
          <input type="date" value={until} required autoFocus onChange={(e) => setUntil(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          {page.verified && (
            <button type="button" className="quiet" onClick={() => remove.mutate()}>
              {t('organise.removeVerified')}
            </button>
          )}
          <button type="submit" className="primary">
            {t('organise.verify')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export function VerifiedMark({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const v = page.verified;
  if (!v) return null;
  return (
    <>
      <span className={v.expired ? 'verified expired' : 'verified'}>
        <IconVerified size={MARK_ICON_SIZE} />
        {v.expired
          ? t('organise.verifiedExpired', { name: v.by_name, date: v.until })
          : t('organise.verifiedBy', { name: v.by_name, date: v.until })}
        {v.changed_after && <span className="muted"> {t('organise.changedAfter')}</span>}
      </span>
      {page.can['page.verify'] && (
        <button type="button" className="ghost" onClick={() => setOpen(true)}>
          {t('organise.changeVerified')}
        </button>
      )}
      {open && <VerifyDialog page={page} onClose={() => setOpen(false)} />}
    </>
  );
}

/** What the approvers sign: the text as it was when they were asked. */
function FrozenText({ round }: { round: ApprovalRound }) {
  const { t } = useTranslation();
  const [shown, setShown] = useState(false);
  const q = useQuery({
    queryKey: ['approval', round.id],
    queryFn: () => api.get<ApprovalRound>(`/approvals/${round.id}`),
    enabled: shown,
  });
  return (
    <>
      <button type="button" className="ghost" aria-expanded={shown} onClick={() => setShown(!shown)}>
        {t('organise.readFrozen')}
      </button>
      <ErrorNote error={q.error} />
      {shown && q.data?.document && <DocumentView document={q.data.document} plain />}
    </>
  );
}

function ApprovalAnswers({ round, onRemove }: { round: ApprovalRound; onRemove?: (userId: string) => void }) {
  const { t } = useTranslation();
  return (
    <ul className="plain-list details-links">
      {round.answers.map((a) => (
        <li key={a.user_id}>
          {a.name || t('common.unknown')}
          <span className="muted">
            {a.answer ? t(`organise.answer_${a.answer}`) : t('organise.answerWaiting')}
            {a.comment ? `: ${a.comment}` : ''}
          </span>
          {onRemove && !a.answer && (
            <button type="button" className="ghost" onClick={() => onRemove(a.user_id)}>
              {t('common.remove')}
            </button>
          )}
        </li>
      ))}
    </ul>
  );
}

function AskApproval({ page, onDone }: { page: PageDetail; onDone: () => void }) {
  const { t } = useTranslation();
  const toast = useToast();
  const [people, setPeople] = useState<DirectoryEntry[]>([]);
  const [message, setMessage] = useState('');
  const [due, setDue] = useState('');
  const ask = useMutation({
    mutationFn: () =>
      api.post<{ cannot_see: string[] }>(`/pages/${page.id}/approvals`, {
        user_ids: people.map((p) => p.id),
        message,
        due_on: due,
      }),
    onSuccess: (r) => {
      if (r.cannot_see.length > 0)
        toast({ message: t('organise.cannotSee', { count: r.cannot_see.length }) });
      onDone();
    },
  });
  return (
    <section>
      <h3>{t('organise.askApproval')}</h3>
      <ErrorNote error={ask.error} />
      <ul className="plain-list">
        {people.map((p) => (
          <li key={p.id} className="row">
            {p.name}
            <button
              type="button"
              className="ghost"
              onClick={() => setPeople(people.filter((x) => x.id !== p.id))}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <SubjectPicker
        label={t('organise.approver')}
        usersOnly
        onPick={(entry) => {
          if (!people.some((p) => p.id === entry.id)) setPeople([...people, entry]);
        }}
      />
      <Field label={t('organise.message')}>
        <textarea rows={2} value={message} maxLength={2000} onChange={(e) => setMessage(e.target.value)} />
      </Field>
      <Field label={t('organise.dueDate')}>
        <input type="date" value={due} onChange={(e) => setDue(e.target.value)} />
      </Field>
      <div className="dialog-actions">
        <button
          type="button"
          className="primary"
          disabled={people.length === 0 || ask.isPending}
          onClick={() => ask.mutate()}
        >
          {t('organise.sendRequest')}
        </button>
      </div>
    </section>
  );
}

function AnswerRound({ round, onDone }: { round: ApprovalRound; onDone: () => void }) {
  const { t } = useTranslation();
  const [comment, setComment] = useState('');
  const answer = useMutation({
    mutationFn: (value: string) => api.put(`/approvals/${round.id}/answer`, { answer: value, comment }),
    onSuccess: onDone,
  });
  return (
    <>
      <ErrorNote error={answer.error} />
      <Field label={t('organise.comment')}>
        <textarea rows={2} value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      <div className="dialog-actions">
        <button type="button" onClick={() => answer.mutate('changes')}>
          {t('organise.askChanges')}
        </button>
        <button type="button" className="primary" onClick={() => answer.mutate('approve')}>
          {t('organise.approve')}
        </button>
      </div>
    </>
  );
}

export function ApprovalsDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const refresh = useRefresh(page);
  const rounds = useQuery({
    queryKey: ['approvals', page.id],
    queryFn: () => api.get<{ rounds: ApprovalRound[] }>(`/pages/${page.id}/approvals`),
  });
  const open = rounds.data?.rounds.find((r) => r.status === 'open');
  const closeRound = useMutation({
    mutationFn: (id: string) => api.post(`/approvals/${id}/close`),
    onSettled: refresh,
  });
  const removePerson = useMutation({
    mutationFn: (v: { round: string; user: string }) => api.del(`/approvals/${v.round}/people/${v.user}`),
    onSettled: refresh,
  });
  const mayEnd = page.can['page.edit'] ?? false;
  return (
    <Dialog title={t('organise.approvals')} onClose={onClose} wide>
      {rounds.isPending && <Loading />}
      <ErrorNote error={rounds.error ?? closeRound.error ?? removePerson.error} />
      {open && (
        <section>
          <h3>{t('organise.openRound', { name: open.asked_by_name })}</h3>
          {open.message && <p>{open.message}</p>}
          <ApprovalAnswers
            round={open}
            onRemove={mayEnd ? (user) => removePerson.mutate({ round: open.id, user }) : undefined}
          />
          <FrozenText round={open} />
          {mayEnd && (
            <button type="button" className="ghost" onClick={() => closeRound.mutate(open.id)}>
              {t('organise.closeRound')}
            </button>
          )}
          {page.approval?.must_answer && <AnswerRound round={open} onDone={() => void refresh()} />}
        </section>
      )}
      {page.can['page.edit'] && <AskApproval page={page} onDone={() => void refresh()} />}
      {rounds.data?.rounds
        .filter((r) => r.status !== 'open')
        .map((r) => (
          <section key={r.id}>
            <h3>
              {t(`organise.round_${r.status}`)} <span className="muted">{formatDate(r.created_at)}</span>
            </h3>
            <ApprovalAnswers round={r} />
          </section>
        ))}
    </Dialog>
  );
}

export function ApprovalBanner({ page, onOpen }: { page: PageDetail; onOpen: () => void }) {
  const { t } = useTranslation();
  const a = page.approval;
  if (!a) return null;
  if (a.status === 'open') {
    return (
      <p role="status" className="note">
        {t('organise.awaitingApproval', { answered: a.answered, asked: a.asked })}{' '}
        <button type="button" className="ghost" onClick={onOpen}>
          {a.must_answer ? t('organise.giveAnswer') : t('organise.seeRound')}
        </button>
      </p>
    );
  }
  if (a.is_current) {
    return (
      <p role="status" className="note">
        <IconVerified size={MARK_ICON_SIZE} /> {t('organise.approvedVersion', { number: a.approved_version })}
      </p>
    );
  }
  if (a.approved_version === 0) return null;
  return (
    <p role="status" className="note">
      {t('organise.olderApproved', { number: a.approved_version })}{' '}
      <Link href={`/pages/${page.id}/compare?from=${a.approved_version}&to=${page.current_version}`}>
        {t('organise.compareApproved')}
      </Link>
    </p>
  );
}

function RestoreButton({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const refresh = useRefresh(page);
  const [withParents, setWithParents] = useState(false);
  const restore = useMutation({
    mutationFn: (parents: boolean) => api.post(`/pages/${page.id}/unarchive`, { with_parents: parents }),
    onSuccess: refresh,
    onError: (e) => {
      if (e instanceof Error && e.message.includes('Restore both')) setWithParents(true);
    },
  });
  return (
    <>
      <button type="button" className="ghost" onClick={() => restore.mutate(withParents)}>
        {withParents ? t('organise.restoreBoth') : t('organise.restore')}
      </button>
      <ErrorNote error={restore.error} />
    </>
  );
}

export function PageNotes({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const today = new Date().toISOString().slice(0, 10);
  const pastReview = page.review_on !== '' && page.review_on <= today && page.can['page.edit'];
  const whatChanged = page.seen_version > 0 && page.seen_version < page.current_version;
  return (
    <>
      {page.archived_at && (
        <p role="status" className="note">
          {t('organise.archivedPage')} {page.can['page.archive'] && <RestoreButton page={page} />}
        </p>
      )}
      {page.scheduled && (
        <p role="status" className="note">
          {t('organise.scheduled')} <RelativeTime value={page.scheduled.at} />
        </p>
      )}
      {pastReview && <p className="muted">{t('organise.pastReview', { date: page.review_on })}</p>}
      {whatChanged && (
        <p>
          <Link href={`/pages/${page.id}/compare?from=${page.seen_version}&to=${page.current_version}`}>
            {t('organise.whatChanged')}
          </Link>
        </p>
      )}
    </>
  );
}
