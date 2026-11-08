// SPDX-License-Identifier: Apache-2.0
// Personal settings that follow a person to other devices (spec 009, B1).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Preferences } from './api';

const THEME_KEY = 'diffluence.theme';
const SYSTEM_THEME = 'system';

export const DEFAULT_PREFERENCES: Preferences = {
  theme: SYSTEM_THEME,
  shortcuts: true,
  contents: true,
  hover_cards: true,
  hide_follows: false,
  show_presence: true,
  typing_help: true,
  sidebar_width: 18,
  sidebar_shut: null,
  sidebar_links_height: 0,
  tree_view: 'tree',
  language: 'en',
  time_zone: '',
  date_format: 'medium',
  first_day: 1,
  welcome_dismissed: false,
  tour_seen: false,
};

/** The browser keeps a copy, so the first paint is right before the person's data arrives. */
export function applyTheme(theme: string): void {
  if (theme === SYSTEM_THEME) delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
  localStorage.setItem(THEME_KEY, theme);
}

export function applyStoredTheme(): void {
  applyTheme(localStorage.getItem(THEME_KEY) ?? SYSTEM_THEME);
}

export function usePreferences() {
  const client = useQueryClient();
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const preferences = me.data?.preferences ?? DEFAULT_PREFERENCES;
  const save = useMutation({
    mutationFn: (next: Preferences) => api.put<Preferences>('/me/preferences', next),
    // A switch answers at once, and goes back when the server refuses.
    onMutate: (next) => {
      const before = client.getQueryData<Me>(['me'])?.preferences;
      client.setQueryData<Me>(['me'], (old) => (old ? { ...old, preferences: next } : old));
      return { before };
    },
    onError: (_error, _next, context) => {
      const before = context?.before;
      if (before) client.setQueryData<Me>(['me'], (old) => (old ? { ...old, preferences: before } : old));
    },
    onSuccess: (stored) => applyTheme(stored.theme),
  });
  return {
    preferences,
    update: (change: Partial<Preferences>) => save.mutate({ ...preferences, ...change }),
    error: save.error,
  };
}
