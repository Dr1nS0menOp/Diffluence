// SPDX-License-Identifier: Apache-2.0
// The three ways a page can stand (ACC-064), and the one way to change them, shared by the
// restrictions dialog and the access row of Share (COL-105).
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type NamedSubject, type PageSummary, type Restrictions } from '../api';

export const RESTRICTION_STATES = ['open', 'view_only', 'private'] as const;
export type RestrictionState = (typeof RESTRICTION_STATES)[number];

/** Who can reach a page without being named: everyone in the space, or only those chosen. */
export type Reach = 'space' | 'chosen';
/** What everyone in the space may do with a page they reach. */
export type Ability = 'view' | 'edit';

/** The three ways a page can stand, as a person thinks of them (ACC-064). */
export function restrictionState(r: { view: unknown[]; edit: unknown[] }): RestrictionState {
  if (r.view.length > 0) return 'private';
  return r.edit.length > 0 ? 'view_only' : 'open';
}

/** The state as the access row of Share shows it: the reach, and what everyone may do. */
export function accessOf(state: RestrictionState): { reach: Reach; ability: Ability } {
  if (state === 'private') return { reach: 'chosen', ability: 'edit' };
  return { reach: 'space', ability: state === 'open' ? 'edit' : 'view' };
}

/** The state that a reach and an ability make together. */
export function stateOf(reach: Reach, ability: Ability): RestrictionState {
  if (reach === 'chosen') return 'private';
  return ability === 'edit' ? 'open' : 'view_only';
}

export function restrictionsKey(pageId: string) {
  return ['restrictions', pageId];
}

/** The restrictions of a page, and choosing a state, which shows at once while the server answers. */
export function useRestrictionState(page: PageSummary) {
  const client = useQueryClient();
  const key = restrictionsKey(page.id);
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<Restrictions>(`/pages/${page.id}/restrictions`),
  });
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') }).data;
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: key });
    await client.invalidateQueries({ queryKey: ['page', page.id] });
    await client.invalidateQueries({ queryKey: ['access', page.id] });
  };
  const put = (kind: string, subjects: NamedSubject[]) =>
    api.put(`/pages/${page.id}/restrictions`, {
      kind,
      subjects: subjects.map(({ type, id }) => ({ type, id })),
    });
  // The person choosing starts on the list: the server would add them anyway (ACC-065).
  const startWith = (list: NamedSubject[]) =>
    list.length > 0 || !me ? list : [{ type: 'user', id: me.user.id, name: me.user.display_name }];
  const [chosen, setChosen] = useState<RestrictionState | null>(null);
  const choose = useMutation({
    mutationFn: async (state: RestrictionState) => {
      await put('view', state === 'private' ? startWith(q.data?.view ?? []) : []);
      await put('edit', state === 'view_only' ? startWith(q.data?.edit ?? []) : []);
    },
    onSettled: async () => {
      await refresh();
      setChosen(null);
    },
  });
  const set = useMutation({
    mutationFn: (v: { kind: string; subjects: NamedSubject[] }) => put(v.kind, v.subjects),
    onSettled: refresh,
  });
  return {
    restrictions: q.data,
    loadError: q.error,
    state: chosen ?? (q.data ? restrictionState(q.data) : 'open'),
    choose: (state: RestrictionState) => {
      setChosen(state);
      choose.mutate(state);
    },
    chooseError: choose.error,
    setList: (kind: string, subjects: NamedSubject[]) => set.mutate({ kind, subjects }),
    setError: set.error,
  };
}
