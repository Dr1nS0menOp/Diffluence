// SPDX-License-Identifier: Apache-2.0
// The People screen of a space: who can reach it, how, and in what role. Someone who
// manages access changes a role right here (ADR-034, spec 033).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Holder, type Space } from '../api';
import { Link } from '../router';
import { PersonName, PersonPicture, personQuery } from '../components/person';
import { Tabs } from '../components/tabs';
import { Avatar, Dialog, ErrorNote, Loading } from '../components/ui';
import { PreviewText, TEMPLATES, templateOf, useGrantChange, type Grants } from './space-grants';

const PEOPLE_TABS = ['everyone', 'admins', 'groups', 'guests'] as const;
type PeopleTab = (typeof PEOPLE_TABS)[number];

/** Which rows a tab shows. */
export function inTab(holder: Holder, tab: PeopleTab): boolean {
  if (tab === 'admins') return holder.role === 'space_admin';
  if (tab === 'groups') return holder.subject.type === 'group';
  if (tab === 'guests') return holder.guest || holder.subject.type === 'all_guests';
  return true;
}

function holderName(holder: Holder, t: (key: string) => string): string {
  if (holder.subject.type === 'all_members') return t('permissions.allMembers');
  if (holder.subject.type === 'all_guests') return t('permissions.allGuests');
  return holder.name;
}

function HolderPicture({ holder, name }: { holder: Holder; name: string }) {
  const isPerson = holder.subject.type === 'user';
  const person = useQuery({ ...personQuery(holder.subject.id), enabled: isPerson }).data;
  if (isPerson && person) return <PersonPicture person={person} />;
  return <Avatar name={name} />;
}

function HowTheyGotIn({ holder }: { holder: Holder }) {
  const { t } = useTranslation();
  if (holder.source === 'group') return <>{t('access.viaGroup', { group: holder.via })}</>;
  if (holder.subject.type === 'group') return <>{t('people.aGroup')}</>;
  if (holder.source === 'everyone') return <>{t('people.everyoneOfAKind')}</>;
  return <>{t('access.direct')}</>;
}

function RoleChoice({
  holder,
  name,
  grants,
  onPick,
}: {
  holder: Holder;
  name: string;
  grants: Grants | undefined;
  onPick: (permissions: string[]) => void;
}) {
  const { t } = useTranslation();
  const known = TEMPLATES.includes(holder.role);
  if (!grants) {
    return (
      <span className="role-name">{known ? t(`permissions.${holder.role}`) : t('permissions.custom')}</span>
    );
  }
  const grant = grants.grants.find(
    (g) => g.subject.type === holder.subject.type && g.subject.id === holder.subject.id,
  );
  const value = grant ? templateOf(grant.permissions, grants) : 'custom';
  return (
    <select
      className="role-choice"
      aria-label={t('people.roleOf', { name })}
      value={value}
      onChange={(e) => onPick(grants.templates[e.target.value] ?? [])}
    >
      {TEMPLATES.map((n) => (
        <option key={n} value={n}>
          {t(`permissions.${n}`)}
        </option>
      ))}
      <option value="custom" disabled>
        {t('permissions.custom')}
      </option>
    </select>
  );
}

export function SpacePeople({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<PeopleTab>('everyone');
  const [text, setText] = useState('');
  const mayManage = space.can?.['space.manage_access'] ?? false;
  const people = useQuery({
    queryKey: ['space-people', space.id],
    queryFn: () => api.get<{ people: Holder[] }>(`/spaces/${space.id}/people`),
  });
  const grants = useQuery({
    queryKey: ['grants', space.id],
    queryFn: () => api.get<Grants>(`/spaces/${space.id}/grants`),
    enabled: mayManage,
  });
  const change = useGrantChange(space);
  const wanted = text.trim().toLowerCase();
  const rows = (people.data?.people ?? []).filter(
    (h) => inTab(h, tab) && holderName(h, t).toLowerCase().includes(wanted),
  );
  return (
    <>
      <div className="section-head">
        <h1>{t('access.peopleTitle')}</h1>
        {mayManage && (
          <Link href={`/spaces/${space.key}/access`} className="button primary">
            {t('people.addPeople')}
          </Link>
        )}
      </div>
      {people.data && <p className="muted">{t('people.canReach', { count: people.data.people.length })}</p>}
      <Tabs
        label={t('access.peopleTitle')}
        panel="people-panel"
        tabs={PEOPLE_TABS}
        current={tab}
        name={(name) => t(`people.tab_${name}`)}
        onChange={setTab}
      />
      <div id="people-panel" role="tabpanel" aria-labelledby={`people-panel-tab-${tab}`}>
        <input
          type="search"
          className="list-filter"
          value={text}
          aria-label={t('people.findInSpace')}
          placeholder={t('people.findInSpace')}
          onChange={(e) => setText(e.target.value)}
        />
        <ErrorNote error={people.error ?? change.error} />
        {people.isPending && <Loading />}
        {people.data && rows.length === 0 && <p className="muted">{t('people.noneHere')}</p>}
        <ul className="member-list">
          {rows.map((h) => {
            const name = holderName(h, t);
            return (
              <li key={h.subject.type + h.subject.id + h.source + h.via} className="member-row">
                <HolderPicture holder={h} name={name} />
                <span className="member-text">
                  <span className="member-name">
                    {h.subject.type === 'user' ? <PersonName id={h.subject.id} name={name} /> : name}
                    {h.guest && <span className="lozenge">{t('people.guest')}</span>}
                  </span>
                  <span className="muted">
                    <HowTheyGotIn holder={h} />
                  </span>
                </span>
                <RoleChoice
                  holder={h}
                  name={name}
                  grants={h.source === 'group' ? undefined : grants.data}
                  onPick={(permissions) => change.change({ subject: h.subject, permissions })}
                />
              </li>
            );
          })}
        </ul>
      </div>
      {change.asking && (
        <Dialog title={t('access.confirmTitle')} onClose={change.cancel}>
          <PreviewText preview={change.asking.preview} />
          <div className="dialog-actions">
            <button type="button" className="quiet" onClick={change.cancel}>
              {t('common.cancel')}
            </button>
            <button type="button" className="danger" onClick={change.confirm}>
              {t('common.save')}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}
