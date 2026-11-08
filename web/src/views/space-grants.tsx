// SPDX-License-Identifier: Apache-2.0
// Grants of a space and the way to change them, shared by Access and People (spec 033).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Grant, type GrantPreview, type Space, type Subject } from '../api';

export const TEMPLATES = ['viewer', 'editor', 'manager', 'space_admin'];
export interface NamedSet {
  id: string;
  name: string;
  permissions: string[];
}

export interface Grants {
  grants: Grant[];
  all_permissions: string[];
  templates: Record<string, string[]>;
  sets: NamedSet[];
}

export interface Change {
  subject: Subject;
  permissions: string[];
}

const sameSet = (a: string[], b: string[]) => [...a].sort().join() === [...b].sort().join();

export function templateOf(perms: string[], grants: Grants): string {
  const named = TEMPLATES.find((n) => sameSet(grants.templates[n] ?? [], perms));
  return named ?? grants.sets.find((s) => sameSet(s.permissions, perms))?.id ?? 'custom';
}

/** Saves a change after showing what it does. It only asks when someone loses view. */
export function useGrantChange(space: Space) {
  const client = useQueryClient();
  const [asking, setAsking] = useState<{ change: Change; preview: GrantPreview } | null>(null);
  const [lastPreview, setLastPreview] = useState<GrantPreview | null>(null);
  const save = useMutation({
    mutationFn: (change: Change) => api.put(`/spaces/${space.id}/grants`, change),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['grants', space.id] });
      await client.invalidateQueries({ queryKey: ['space-access', space.id] });
      await client.invalidateQueries({ queryKey: ['space-people', space.id] });
    },
  });
  const check = useMutation({
    mutationFn: (change: Change) => api.post<GrantPreview>(`/spaces/${space.id}/grants/preview`, change),
    onSuccess: (preview, change) => {
      if (preview.loses_view) {
        setAsking({ change, preview });
        return;
      }
      setLastPreview(preview);
      save.mutate(change);
    },
  });
  return {
    change: (next: Change) => check.mutate(next),
    confirm: () => {
      if (asking) save.mutate(asking.change);
      setAsking(null);
    },
    cancel: () => setAsking(null),
    asking,
    lastPreview,
    error: check.error ?? save.error,
  };
}

export function PreviewText({ preview }: { preview: GrantPreview }) {
  const { t } = useTranslation();
  const list = (names: string[]) => names.map((n) => t(`permissions.${n.replace('.', '_')}`)).join(', ');
  return (
    <>
      {preview.gains.length > 0 && <p>{t('access.gains', { list: list(preview.gains) })}</p>}
      {preview.loses.length > 0 && <p>{t('access.loses', { list: list(preview.loses) })}</p>}
      <p className="muted">{t('access.people', { count: preview.people })}</p>
    </>
  );
}
