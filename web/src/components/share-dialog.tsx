// SPDX-License-Identifier: Apache-2.0
// One place for "who can see this" (spec 010, C7). Sharing only notifies: it never gives
// anyone access, and the dialog says who cannot see the page. Someone with the role can
// also ask an administrator to invite a person without an account (spec 022).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type AccessEntry,
  type AccessRequest,
  type Me,
  type MentionCandidate,
  type PageSummary,
} from '../api';
import { useCopy } from './toast';
import { Dialog, ErrorNote, Field, Loading } from './ui';

const MAX_MESSAGE = 500;

function AccessList({ page }: { page: PageSummary }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['access', page.id],
    queryFn: () =>
      api.get<{ restricted: boolean; entries: AccessEntry[]; total: number }>(`/pages/${page.id}/access`),
  });
  const label = (e: AccessEntry) => {
    if (e.type === 'all_members') return t('permissions.allMembers');
    if (e.type === 'all_guests') return t('permissions.allGuests');
    return e.name;
  };
  return (
    <section>
      <h3>{t('share.whoHasAccess')}</h3>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data?.restricted && <p className="muted">{t('share.restricted')}</p>}
      <ul className="plain-list">
        {q.data?.entries.map((e) => (
          <li key={e.type + e.id}>{label(e)}</li>
        ))}
      </ul>
      {(q.data?.total ?? 0) > (q.data?.entries.length ?? 0) && (
        <p className="muted">
          {t('share.andMore', { count: (q.data?.total ?? 0) - (q.data?.entries.length ?? 0) })}
        </p>
      )}
    </section>
  );
}

function Requests({ page }: { page: PageSummary }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['access-requests', page.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ requests: AccessRequest[] }>(`/pages/${page.id}/access-requests`),
  });
  const answer = useMutation({
    mutationFn: (v: { id: string; allow: boolean }) =>
      api.post(`/access-requests/${v.id}/${v.allow ? 'allow' : 'refuse'}`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: key });
      await client.invalidateQueries({ queryKey: ['access', page.id] });
    },
  });
  if (!q.data || q.data.requests.length === 0) return null;
  return (
    <section>
      <h3>{t('share.requests')}</h3>
      <ErrorNote error={answer.error} />
      <ul className="plain-list">
        {q.data.requests.map((r) => (
          <li key={r.id}>
            <p>
              {t(`share.asks_${r.kind}`, { name: r.requester_name })} {t(`share.effect_${r.effect}`)}
            </p>
            <ul>
              {r.levels.map((level, at) => (
                <li key={at}>
                  {level.can_manage
                    ? t('share.level', { title: level.title, count: level.pages })
                    : t('share.levelOutOfReach')}
                </li>
              ))}
            </ul>
            {!r.can_allow && <p className="note">{t('share.cannotAllow')}</p>}
            <div className="row">
              <button
                type="button"
                disabled={!r.can_allow}
                onClick={() => answer.mutate({ id: r.id, allow: true })}
              >
                {t('share.allow')}
              </button>
              <button
                type="button"
                className="quiet"
                onClick={() => answer.mutate({ id: r.id, allow: false })}
              >
                {t('share.refuse')}
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

function PeopleField({
  page,
  chosen,
  onChoose,
}: {
  page: PageSummary;
  chosen: MentionCandidate[];
  onChoose: (next: MentionCandidate[]) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const q = useQuery({
    queryKey: ['share-candidates', page.id, text],
    queryFn: () =>
      api.get<{ candidates: MentionCandidate[] }>(
        `/pages/${page.id}/mention-candidates?for=share&q=${encodeURIComponent(text)}`,
      ),
    enabled: text.trim() !== '',
  });
  const found = (q.data?.candidates ?? []).filter((c) => !chosen.some((x) => x.id === c.id));
  return (
    <>
      <Field label={t('share.people')}>
        <input value={text} autoFocus onChange={(e) => setText(e.target.value)} />
      </Field>
      <ErrorNote error={q.error} />
      {text.trim() !== '' && found.length > 0 && (
        <ul className="plain-list insert-list">
          {found.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                className="suggestion"
                onClick={() => {
                  onChoose([...chosen, c]);
                  setText('');
                }}
              >
                <span className="suggestion-name">{c.name}</span>
                {!c.can_view && <span className="muted suggestion-hint">{t('mentions.cannotSee')}</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
      <ul className="plain-list">
        {chosen.map((c) => (
          <li key={c.id} className="row">
            {c.name}
            {!c.can_view && <span className="lozenge">{t('mentions.cannotSee')}</span>}
            <button
              type="button"
              className="ghost"
              onClick={() => onChoose(chosen.filter((x) => x.id !== c.id))}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}

/** Only someone with the role sees this, and the server asks for the role as well. */
function InviteSomeone({ page }: { page: PageSummary }) {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');
  const [note, setNote] = useState('');
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const ask = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/invitation-requests`, { email, note }),
    onSuccess: () => {
      setEmail('');
      setNote('');
    },
  });
  if (!me.data?.global_permissions.includes('invite_people')) return null;
  return (
    <section>
      <h3>{t('share.inviteTitle')}</h3>
      <p className="muted">{t('share.inviteHelp')}</p>
      <ErrorNote error={ask.error} />
      {ask.isSuccess && (
        <p role="status" className="note">
          {t('share.inviteWaiting')}
        </p>
      )}
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          ask.mutate();
        }}
      >
        <Field label={t('common.email')}>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        <Field label={t('share.inviteNote')}>
          <textarea rows={2} maxLength={MAX_MESSAGE} value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        <button type="submit" disabled={ask.isPending}>
          {t('share.inviteAsk')}
        </button>
      </form>
    </section>
  );
}

export function ShareDialog({
  page,
  onClose,
  onRestrictions,
}: {
  page: PageSummary;
  onClose: () => void;
  onRestrictions?: () => void;
}) {
  const { t } = useTranslation();
  const copy = useCopy();
  const [chosen, setChosen] = useState<MentionCandidate[]>([]);
  const [message, setMessage] = useState('');
  const [notify, setNotify] = useState(true);
  const share = useMutation({
    mutationFn: () =>
      api.post<{ notified: number; no_access: { id: string; name: string }[] }>(`/pages/${page.id}/share`, {
        people: chosen.map((c) => ({ type: c.kind, id: c.id })),
        message,
        notify,
      }),
    onSuccess: () => setChosen([]),
  });
  return (
    <Dialog title={t('share.title')} onClose={onClose} wide>
      <ErrorNote error={share.error} />
      {share.data && (
        <p role="status" className="note">
          {t('share.sent', { count: share.data.notified })}
          {share.data.no_access.length > 0 &&
            ` ${t('share.noAccess', { names: share.data.no_access.map((p) => p.name).join(', ') })}`}
        </p>
      )}
      <PeopleField page={page} chosen={chosen} onChoose={setChosen} />
      <Field label={t('share.message')}>
        <textarea
          rows={2}
          maxLength={MAX_MESSAGE}
          value={message}
          onChange={(e) => setMessage(e.target.value)}
        />
      </Field>
      <label className="check">
        <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} />
        {t('share.notify')}
      </label>
      <div className="dialog-actions">
        <button
          type="button"
          onClick={() => void copy(`${window.location.origin}/pages/${page.id}`, t('tree.linkCopied'))}
        >
          {t('tree.copyLink')}
        </button>
        <button
          type="button"
          className="primary"
          disabled={chosen.length === 0 || share.isPending}
          onClick={() => share.mutate()}
        >
          {t('share.send')}
        </button>
      </div>
      <InviteSomeone page={page} />
      <AccessList page={page} />
      {onRestrictions && page.can['restriction.manage'] && (
        <button type="button" onClick={onRestrictions}>
          {t('share.changeRestrictions')}
        </button>
      )}
      {page.can['restriction.manage'] && <Requests page={page} />}
    </Dialog>
  );
}
