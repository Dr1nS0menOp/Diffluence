// SPDX-License-Identifier: Apache-2.0
// The idle timeout as seen from the browser (spec 013, F5). Only what a person does
// counts: requests marked as background do not (D8).

const MINUTE_MS = 60_000;

/** WCAG 2.2.1 asks for at least 20 seconds. Two minutes leaves time to come back. */
export const WARNING_MS = 2 * MINUTE_MS;

/** How long typing may go on before the server has to hear about it. */
export const PING_MS = MINUTE_MS;

export type IdleState = { kind: 'fine' } | { kind: 'warning'; seconds: number } | { kind: 'ended' };

/** Where a session stands, from the last request a person caused. */
export function idleState(now: number, lastRequest: number, idleMinutes: number): IdleState {
  if (idleMinutes <= 0) return { kind: 'fine' };
  const left = lastRequest + idleMinutes * MINUTE_MS - now;
  if (left <= 0) return { kind: 'ended' };
  if (left <= WARNING_MS) return { kind: 'warning', seconds: Math.ceil(left / 1000) };
  return { kind: 'fine' };
}

/** Typing sends nothing by itself, so a session would end in the middle of a sentence. */
export function needsPing(now: number, lastRequest: number, lastInput: number): boolean {
  return lastInput > lastRequest && now - lastRequest >= PING_MS;
}

/** Only an address inside this site: a link to the login page must not lead elsewhere. */
export function safeReturnPath(next: string | null): string {
  if (!next?.startsWith('/') || next.startsWith('//') || next.startsWith('/\\')) return '/';
  return next;
}
