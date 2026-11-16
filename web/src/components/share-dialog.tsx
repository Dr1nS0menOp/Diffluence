// SPDX-License-Identifier: Apache-2.0
// One place for "who can see this" (spec 010, C7; spec 079, COL-102 to COL-108). Sharing
// only notifies: it never gives anyone access, and the dialog says who cannot see the page.
// One field takes people, groups and, for someone with the role, e-mail addresses, which
// ask an administrator to invite them (spec 022). The access row changes the restrictions
// through the same calls as the restrictions dialog.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type AccessEntry,
  type AccessRequest,
  type Me,
  type MentionCandidate,
  type PageSummary,
  type Space,
} from '../api';
import { Link } from '../router';
import { IconChevronDown, IconClose, IconLink, IconLock, IconLockOpen, IconMore } from './icons';
import { Menu, MenuItem } from './menu';
import { accessOf, stateOf, useRestrictionState, type Ability, type Reach } from './restriction-state';
import { useCopy } from './toast';
import { Dialog, ErrorNote, Field, Loading } from './ui';

const MAX_MESSAGE = 500;
const MARK_SIZE = 18;
const CHIP_ICON_SIZE = 14;
// Enough to tell an address from a name; the server checks it properly.
const LOOKS_LIKE_EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const REACHES: Reach[] = ['space', 'chosen'];
const ABILITIES: Ability[] = ['view', 'edit'];

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
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  // The list shows only when it has someone in it (COL-107).
  if (!q.data || q.data.entries.length === 0) return null;
  const more = q.data.total - q.data.entries.length;
  return (
    <section className="share-section">
      <h3 className="share-heading">{t('share.whoHasAccess')}</h3>
      <ul className="plain-list">
        {q.data.entries.map((e) => (
          <li key={e.type + e.id}>{label(e)}</li>
        ))}
      </ul>
      {more > 0 && <p className="muted">{t('share.andMore', { count: more })}</p>}
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
    <section className="share-section">
      <h3 className="share-heading">{t('share.requests')}</h3>
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

/** Who is about to be told: people and groups, and addresses to invite. */
interface Recipients {
  people: MentionCandidate[];
  emails: string[];
}

function RecipientField({
  page,
  chosen,
  mayInvite,
  onChoose,
}: {
  page: PageSummary;
  chosen: Recipients;
  mayInvite: boolean;
  onChoose: (next: Recipients) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const typed = text.trim();
  const q = useQuery({
    queryKey: ['share-candidates', page.id, typed],
    queryFn: () =>
      api.get<{ candidates: MentionCandidate[] }>(
        `/pages/${page.id}/mention-candidates?for=share&q=${encodeURIComponent(typed)}`,
      ),
    enabled: typed !== '',
  });
  const found = (q.data?.candidates ?? []).filter((c) => !chosen.people.some((x) => x.id === c.id));
  const address = mayInvite && LOOKS_LIKE_EMAIL.test(typed) && !chosen.emails.includes(typed) ? typed : '';
  const label = mayInvite ? t('share.recipientsWithEmail') : t('share.recipients');
  return (
    <div className="share-recipients">
      <input
        className="share-field"
        value={text}
        autoFocus
        aria-label={label}
        placeholder={label}
        onChange={(e) => setText(e.target.value)}
      />
      <ErrorNote error={q.error} />
      {typed !== '' && (found.length > 0 || address) && (
        <ul className="plain-list insert-list">
          {found.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                className="suggestion"
                onClick={() => {
                  onChoose({ ...chosen, people: [...chosen.people, c] });
                  setText('');
                }}
              >
                <span className="suggestion-name">{c.name}</span>
                {!c.can_view && <span className="muted suggestion-hint">{t('mentions.cannotSee')}</span>}
              </button>
            </li>
          ))}
          {address && (
            <li>
              <button
                type="button"
                className="suggestion"
                onClick={() => {
                  onChoose({ ...chosen, emails: [...chosen.emails, address] });
                  setText('');
                }}
              >
                <span className="suggestion-name">{t('share.inviteAddress', { email: address })}</span>
              </button>
            </li>
          )}
        </ul>
      )}
      {(chosen.people.length > 0 || chosen.emails.length > 0) && (
        <ul className="plain-list share-chips">
          {chosen.people.map((c) => (
            <li key={c.id} className="share-chip">
              {c.name}
              {!c.can_view && <span className="lozenge">{t('mentions.cannotSee')}</span>}
              <button
                type="button"
                className="ghost"
                aria-label={t('share.removeRecipient', { name: c.name })}
                title={t('share.removeRecipient', { name: c.name })}
                onClick={() => onChoose({ ...chosen, people: chosen.people.filter((x) => x.id !== c.id) })}
              >
                <IconClose size={CHIP_ICON_SIZE} />
              </button>
            </li>
          ))}
          {chosen.emails.map((email) => (
            <li key={email} className="share-chip">
              {email}
              <button
                type="button"
                className="ghost"
                aria-label={t('share.removeRecipient', { name: email })}
                title={t('share.removeRecipient', { name: email })}
                onClick={() => onChoose({ ...chosen, emails: chosen.emails.filter((x) => x !== email) })}
              >
                <IconClose size={CHIP_ICON_SIZE} />
              </button>
            </li>
          ))}
        </ul>
      )}
      {chosen.emails.length > 0 && <p className="muted">{t('share.inviteHelp')}</p>}
    </div>
  );
}

/** A choice in the access row: plain words with an arrow, or plain words without the right to change. */
function Choice<T extends string>({
  label,
  value,
  options,
  words,
  editable,
  onChoose,
}: {
  label: string;
  value: T;
  options: T[];
  words: (option: T) => string;
  editable: boolean;
  onChoose: (option: T) => void;
}) {
  if (!editable) return <span className="access-choice">{words(value)}</span>;
  return (
    <Menu
      // The name holds the words it shows, so speech and sight agree (WCAG 2.5.3).
      label={`${label}: ${words(value)}`}
      align="start"
      className="access-choice"
      trigger={
        <>
          {words(value)} <IconChevronDown size={CHIP_ICON_SIZE} />
        </>
      }
    >
      {options.map((option) => (
        <MenuItem
          key={option}
          role="menuitemradio"
          aria-checked={option === value}
          onClick={() => {
            if (option !== value) onChoose(option);
          }}
        >
          {words(option)}
        </MenuItem>
      ))}
    </Menu>
  );
}

/**
 * Who can reach the page without being named, and what they may do: the three states of
 * ACC-064 as two plain choices (COL-105). Read-only without *manage restrictions*.
 */
function AccessRow({ page }: { page: PageSummary }) {
  const { t } = useTranslation();
  const { state, choose, chooseError, loadError } = useRestrictionState(page);
  const space = useQuery({
    queryKey: ['space-id', page.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.space_id}`),
  });
  const editable = page.can['restriction.manage'] === true;
  const { reach, ability } = accessOf(state);
  return (
    <section className="share-section">
      <h3 className="share-heading">{t('share.generalAccess')}</h3>
      <ErrorNote error={chooseError ?? loadError} />
      <div className="access-row" data-state={state}>
        <span className="access-mark">
          {state === 'open' ? <IconLockOpen size={MARK_SIZE} /> : <IconLock size={MARK_SIZE} />}
        </span>
        <div className="access-reach">
          <Choice
            label={t('share.reachLabel')}
            value={reach}
            options={REACHES}
            words={(option) => t(`share.reach_${option}`)}
            editable={editable}
            onChoose={(next) => choose(stateOf(next, ability))}
          />
          <span className="muted">
            {space.data && <Link href={`/spaces/${space.data.key}`}>{space.data.name}</Link>}
            {space.data && ': '}
            {t(`share.reachHelp_${reach}`)}
          </span>
        </div>
        {reach === 'space' && (
          <Choice
            label={t('share.abilityLabel')}
            value={ability}
            options={ABILITIES}
            words={(option) => t(`share.ability_${option}`)}
            editable={editable}
            onChoose={(next) => choose(stateOf(reach, next))}
          />
        )}
      </div>
    </section>
  );
}

export function ShareDialog({
  page,
  onClose,
  onRestrictions,
  onSharedPage,
}: {
  page: PageSummary;
  onClose: () => void;
  /** Opens the restrictions in detail, from More in the title row (COL-108). */
  onRestrictions?: () => void;
  /** Opens the single shared page, for someone who may make one (COL-106). */
  onSharedPage?: () => void;
}) {
  const { t } = useTranslation();
  const copy = useCopy();
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const mayInvite = me.data?.global_permissions.includes('invite_people') ?? false;
  const [chosen, setChosen] = useState<Recipients>({ people: [], emails: [] });
  const [message, setMessage] = useState('');
  const [notify, setNotify] = useState(true);
  const someone = chosen.people.length > 0 || chosen.emails.length > 0;
  const share = useMutation({
    mutationFn: async () => {
      let told = { notified: 0, no_access: [] as { id: string; name: string }[] };
      if (chosen.people.length > 0) {
        told = await api.post<typeof told>(`/pages/${page.id}/share`, {
          people: chosen.people.map((c) => ({ type: c.kind, id: c.id })),
          message,
          notify,
        });
      }
      for (const email of chosen.emails) {
        await api.post(`/pages/${page.id}/invitation-requests`, { email, note: message });
      }
      return { ...told, invited: chosen.emails.length };
    },
    onSuccess: () => {
      setChosen({ people: [], emails: [] });
      setMessage('');
    },
  });
  const more = onRestrictions && (
    <Menu label={t('share.more')} trigger={<IconMore />}>
      <MenuItem onClick={onRestrictions}>{t('share.restrictionsInDetail')}</MenuItem>
    </Menu>
  );
  return (
    <Dialog title={t('share.title')} onClose={onClose} wide nearTop tools={more}>
      <ErrorNote error={share.error} />
      {share.data && (
        <p role="status" className="note">
          {share.data.notified > 0 && t('share.sent', { count: share.data.notified })}
          {share.data.invited > 0 && ` ${t('share.inviteWaiting')}`}
          {share.data.no_access.length > 0 &&
            ` ${t('share.noAccess', { names: share.data.no_access.map((p) => p.name).join(', ') })}`}
        </p>
      )}
      <RecipientField page={page} chosen={chosen} mayInvite={mayInvite} onChoose={setChosen} />
      {someone && (
        <>
          <Field label={t('share.message')}>
            <textarea
              rows={2}
              maxLength={MAX_MESSAGE}
              value={message}
              onChange={(e) => setMessage(e.target.value)}
            />
          </Field>
          {chosen.people.length > 0 && (
            <label className="check">
              <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} />
              {t('share.notify')}
            </label>
          )}
        </>
      )}
      <AccessRow page={page} />
      <AccessList page={page} />
      {page.can['restriction.manage'] && <Requests page={page} />}
      <div className="share-footer">
        <button
          type="button"
          className="quiet"
          onClick={() => void copy(`${window.location.origin}/pages/${page.id}`, t('tree.linkCopied'))}
        >
          <IconLink size={MARK_SIZE} /> {t('tree.copyLink')}
        </button>
        {onSharedPage && (
          <button type="button" className="quiet" onClick={onSharedPage}>
            {t('exchange.publicLink')}
          </button>
        )}
        {someone && (
          <button
            type="button"
            className="primary share-send"
            disabled={share.isPending}
            onClick={() => share.mutate()}
          >
            {t('share.send')}
          </button>
        )}
      </div>
    </Dialog>
  );
}
