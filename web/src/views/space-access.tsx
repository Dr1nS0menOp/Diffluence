// SPDX-License-Identifier: Apache-2.0
// Who has access to a space, how they got it, and what a change would do.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Holder, type Space } from '../api';
import { useDates } from '../dates';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { PersonName } from '../components/person';
import { Tabs } from '../components/tabs';
import { ErrorNote, Loading } from '../components/ui';
import { SpaceRolesPanel } from './access-roles';
import { SpacePeople } from './space-people';
import { TEMPLATES } from './space-grants';
import { TableBox } from '../components/table-box';

const ACCESS_TABS = ['grants', 'people'] as const;
type AccessTab = (typeof ACCESS_TABS)[number];
const AUDIT_ID_LENGTH = 8;

function InviteGuest({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');
  const invite = useMutation({
    mutationFn: () => api.post<{ link?: string; waiting?: boolean }>(`/spaces/${space.id}/guests`, { email }),
    onSuccess: () => setEmail(''),
  });
  return (
    <section>
      <h3>{t('access.inviteGuest')}</h3>
      <p className="muted">{t('access.inviteGuestHelp')}</p>
      <ErrorNote error={invite.error} />
      {invite.data && (
        <p role="status" className="note">
          {invite.data.waiting && t('access.inviteWaiting')}
          {!invite.data.waiting &&
            (invite.data.link ? t('access.inviteLink', { link: invite.data.link }) : t('access.inviteSent'))}
        </p>
      )}
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          invite.mutate();
        }}
      >
        <input
          type="email"
          className="grow"
          value={email}
          aria-label={t('common.email')}
          placeholder={t('common.email')}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <button type="submit" disabled={invite.isPending}>
          {t('access.invite')}
        </button>
      </form>
    </section>
  );
}

function Presets({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const preset = useMutation({
    mutationFn: (name: string) => api.post(`/spaces/${space.id}/preset`, { preset: name }),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['grants', space.id] });
      await client.invalidateQueries({ queryKey: ['role-assignments'] });
    },
  });
  return (
    <>
      <h3>{t('permissions.preset')}</h3>
      <ErrorNote error={preset.error} />
      <div className="row">
        <button type="button" onClick={() => preset.mutate('team_space')}>
          {t('permissions.team_space')}
        </button>
        <button type="button" onClick={() => preset.mutate('read_only_kb')}>
          {t('permissions.read_only_kb')}
        </button>
      </div>
    </>
  );
}

function GrantsPanel({ space }: { space: Space }) {
  return (
    <>
      <SpaceRolesPanel space={space} />
      {space.can?.['space.administer'] && <Presets space={space} />}
      <InviteGuest space={space} />
    </>
  );
}
function HolderSource({ holder }: { holder: Holder }) {
  const { t } = useTranslation();
  if (holder.source === 'group') return <>{t('access.viaGroup', { group: holder.via })}</>;
  if (holder.source === 'all_members') return <>{t('permissions.allMembers')}</>;
  if (holder.source === 'all_guests') return <>{t('permissions.allGuests')}</>;
  return <>{t('access.direct')}</>;
}

function HolderTable({ people }: { people: Holder[] }) {
  const { t } = useTranslation();
  return (
    <TableBox>
      <table>
        <thead>
          <tr>
            <th>{t('common.name')}</th>
            <th>{t('permissions.template')}</th>
            <th>{t('access.how')}</th>
          </tr>
        </thead>
        <tbody>
          {people.map((p) => (
            <tr key={p.subject.type + p.subject.id + p.source + p.via}>
              <td>
                {p.subject.type === 'user' ? (
                  <PersonName id={p.subject.id} name={p.name} />
                ) : (
                  p.name || t(`permissions.${p.subject.type === 'all_guests' ? 'allGuests' : 'allMembers'}`)
                )}
              </td>
              <td>{TEMPLATES.includes(p.role) ? t(`permissions.${p.role}`) : t('permissions.custom')}</td>
              <td>
                <HolderSource holder={p} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}

function PeoplePanel({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const q = useQuery({
    queryKey: ['space-access', space.id, text],
    queryFn: () => api.get<{ people: Holder[] }>(`/spaces/${space.id}/access?q=${encodeURIComponent(text)}`),
  });
  return (
    <section>
      <div className="row filters">
        <input
          type="search"
          className="grow"
          value={text}
          aria-label={t('access.search')}
          placeholder={t('access.search')}
          onChange={(e) => setText(e.target.value)}
        />
        <a className="button" href={`/api/v1/spaces/${space.id}/access.csv`}>
          {t('admin.exportCsv')}
        </a>
      </div>
      <ErrorNote error={q.error} />
      {q.data && <HolderTable people={q.data.people} />}
    </section>
  );
}

/** Loads the space of a screen, and frames it. */
function SpaceScreen({
  spaceKey,
  children,
}: {
  spaceKey: string;
  children: (space: Space) => React.ReactNode;
}) {
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!q.data) return <Loading />;
  return <Frame space={q.data}>{children(q.data)}</Frame>;
}

export function SpaceAccessView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<AccessTab>('grants');
  return (
    <SpaceScreen spaceKey={spaceKey}>
      {(space) => (
        <>
          <h1>{t('access.title')}</h1>
          <Tabs
            label={t('access.title')}
            panel="access-panel"
            tabs={ACCESS_TABS}
            current={tab}
            name={(name) => t(`access.tab_${name}`)}
            onChange={setTab}
          />
          <div id="access-panel" role="tabpanel" aria-labelledby={`access-panel-tab-${tab}`}>
            {tab === 'grants' ? <GrantsPanel space={space} /> : <PeoplePanel space={space} />}
          </div>
        </>
      )}
    </SpaceScreen>
  );
}

export function SpacePeopleView({ spaceKey }: { spaceKey: string }) {
  return <SpaceScreen spaceKey={spaceKey}>{(space) => <SpacePeople space={space} />}</SpaceScreen>;
}

interface SpaceEvent {
  seq: number;
  occurred_at: string;
  actor_id: string;
  action: string;
  object_type: string;
  object_id: string;
  result: string;
}

function SpaceAuditTable({ space }: { space: Space }) {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['space-audit', space.id],
    queryFn: () => api.get<{ events: SpaceEvent[] }>(`/spaces/${space.id}/audit`),
  });
  return (
    <>
      <p className="muted">{t('access.auditHelp')}</p>
      <ErrorNote error={q.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('admin.when')}</th>
              <th>{t('admin.actor')}</th>
              <th>{t('admin.action')}</th>
              <th>{t('admin.object')}</th>
              <th>{t('admin.result')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.events.map((e) => (
              <tr key={e.seq}>
                <td>{dates.moment(e.occurred_at)}</td>
                <td>
                  {e.actor_id ? (
                    <PersonName id={e.actor_id} name={e.actor_id.slice(0, AUDIT_ID_LENGTH)} />
                  ) : (
                    ''
                  )}
                </td>
                <td>{e.action}</td>
                <td>{e.object_type || t('access.hiddenPage')}</td>
                <td>{e.result}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </>
  );
}

export function SpaceAuditView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  return (
    <SpaceScreen spaceKey={spaceKey}>
      {(space) => (
        <>
          <h1>{t('access.auditTitle')}</h1>
          <SpaceAuditTable space={space} />
        </>
      )}
    </SpaceScreen>
  );
}
