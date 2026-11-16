// SPDX-License-Identifier: Apache-2.0
// The spaces of a person: their favourites and the spaces they visited (SPC-085, NAV-133).
import { useQuery } from '@tanstack/react-query';
import { api, type Space } from '../api';

export interface MySpaces {
  favourites: Space[];
  recent: Space[];
}

export const mySpacesQuery = {
  queryKey: ['my-spaces'],
  queryFn: () => api.get<MySpaces>('/me/spaces'),
};

/** One order everywhere: favourites, then recent spaces, then the rest. */
export function orderSpaces(mine: MySpaces | undefined, all: Space[] | undefined): Space[] {
  const known = [...(mine?.favourites ?? []), ...(mine?.recent ?? [])];
  const knownIds = new Set(known.map((s) => s.id));
  return [...known, ...(all ?? []).filter((s) => !knownIds.has(s.id))];
}

export function useOrderedSpaces(): Space[] {
  const mine = useQuery(mySpacesQuery);
  const all = useQuery({
    queryKey: ['spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/spaces'),
  });
  return orderSpaces(mine.data, all.data?.spaces);
}
