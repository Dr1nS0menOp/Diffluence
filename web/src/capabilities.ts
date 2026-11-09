// SPDX-License-Identifier: Apache-2.0
// What optional helpers are on, in one answer (ARC-071). A screen asks this before it
// offers something a helper does, so it never offers what would fail. A helper that is
// set up but did not answer its last check counts as off (ARC-072).
import { useQuery } from '@tanstack/react-query';
import { api } from './api';

export interface Capabilities {
  pdf_export: boolean;
  office_preview: boolean;
  drawing: boolean;
  collaboration: boolean;
}

/** Until the answer is there, nothing is offered: better a moment late than a broken button. */
export const NOTHING_ON: Capabilities = {
  pdf_export: false,
  office_preview: false,
  drawing: false,
  collaboration: false,
};

const CAPABILITIES_STALE_MS = 60_000;

export function useCapabilities(): Capabilities {
  const q = useQuery({
    queryKey: ['capabilities'],
    queryFn: () => api.get<Capabilities>('/system/capabilities'),
    staleTime: CAPABILITIES_STALE_MS,
  });
  return q.data ?? NOTHING_ON;
}

/** The parts of a list a helper makes possible, and which helper each needs. */
const NEEDS: Record<string, keyof Capabilities> = {
  block_drawing: 'drawing',
  pdf: 'pdf_export',
};

/** Leaves out what needs a helper that is off. */
export function offered<T extends { id: string }>(items: T[], capabilities: Capabilities): T[] {
  return items.filter((item) => {
    const need = NEEDS[item.id];
    return !need || capabilities[need];
  });
}

/** The same for plain names, such as export formats. */
export function offeredNames(names: string[], capabilities: Capabilities): string[] {
  return offered(
    names.map((id) => ({ id })),
    capabilities,
  ).map((item) => item.id);
}
