// SPDX-License-Identifier: Apache-2.0
// A person on screen: a picture, a mark, and a name that leads to them.
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Person } from '../api';
import { usePreferences } from '../preferences';
import { Link } from '../router';
import { Avatar } from './ui';

const CARD_DELAY_MS = 400;
const CARD_LEAVE_MS = 150;
const PERSON_STALE_MS = 60_000;

export const personQuery = (id: string) => ({
  queryKey: ['person', id],
  queryFn: () => api.get<Person>(`/people/${id}`),
  staleTime: PERSON_STALE_MS,
});

/** The empty alt is on purpose: the name always stands beside the picture. */
export function PersonPicture({ person, size }: { person: Person; size?: 'small' | 'large' }) {
  if (!person.has_picture) return <Avatar name={person.name} size={size} />;
  return (
    <img className={`avatar-picture ${size ?? ''}`} src={`/api/v1/people/${person.id}/picture`} alt="" />
  );
}

/** In words, because colour alone cannot say this (WCAG 1.4.1). */
export function PersonMark({ person }: { person: Person }) {
  const { t } = useTranslation();
  if (!person.active) return <span className="lozenge">{t('people.notActive')}</span>;
  if (person.type === 'guest') return <span className="lozenge">{t('people.guest')}</span>;
  return null;
}

function PersonCard({ person }: { person: Person }) {
  const { t } = useTranslation();
  return (
    <span role="tooltip" className="person-card">
      <PersonPicture person={person} />
      <span className="person-card-text">
        <strong>{person.name}</strong>
        {person.job_title && <span className="muted">{person.job_title}</span>}
        {person.department && <span className="muted">{person.department}</span>}
        <Link href={`/people/${person.id}`}>{t('people.open')}</Link>
      </span>
    </span>
  );
}

/** Follows WCAG 1.4.13: it can be dismissed, hovered, and stays until the pointer leaves. */
function useCard(enabled: boolean) {
  const [shown, setShown] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  useEffect(() => {
    if (!shown) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setShown(false);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [shown]);
  const later = (next: boolean, delay: number) => {
    clearTimeout(timer.current);
    if (enabled) timer.current = setTimeout(() => setShown(next), delay);
  };
  return { shown, open: () => later(true, CARD_DELAY_MS), close: () => later(false, CARD_LEAVE_MS) };
}

/**
 * A name on someone's work. It leads to the person when the viewer may see them, and
 * stays plain text when not: the server answers "not found" for both cases.
 */
export function PersonName({ id, name }: { id: string; name: string }) {
  const { preferences } = usePreferences();
  const person = useQuery({ ...personQuery(id), enabled: !!id }).data;
  const card = useCard(preferences.hover_cards);
  if (!person) return <span>{name}</span>;
  return (
    <span
      className="person-name"
      onMouseEnter={card.open}
      onMouseLeave={card.close}
      onFocus={card.open}
      onBlur={card.close}
    >
      <Link href={`/people/${id}`}>{person.name}</Link> <PersonMark person={person} />
      {card.shown && <PersonCard person={person} />}
    </span>
  );
}
