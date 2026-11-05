// SPDX-License-Identifier: Apache-2.0
// What the browser keeps for one person. On a shared computer the next person must see
// nothing of the last: keys hold the person's id, and logging out removes them all
// (SEC-265, SEC-266). The theme and the sidebar belong to the device and stay.
import { createContext, useContext } from 'react';
import type { Json } from './api';

const PERSONAL_PREFIX = 'diffluence.person.';
const DRAFT_PART = '.draft.';
const UNKEYED_NAMES = ['diffluence.searches'];
const UNKEYED_PREFIXES = ['diffluence.tree.', 'diffluence.comment.'];
const DRAFT_KEEP_MS = 7 * 86_400_000;

const PersonContext = createContext('');

/** Says who is logged in, to everything that keeps something in the browser. */
export const PersonProvider = PersonContext.Provider;

export const usePerson = () => useContext(PersonContext);

export const personalKey = (person: string, name: string) => `${PERSONAL_PREFIX}${person}.${name}`;

const draftKey = (person: string, name: string) => personalKey(person, `draft.${name}`);

function storedKeys(): string[] {
  return Array.from({ length: localStorage.length }, (_, at) => localStorage.key(at) ?? '');
}

/** Removes what this browser holds for anyone: whoever logs out, nothing stays. */
export function forgetPersonalData(): void {
  storedKeys()
    .filter((key) => key.startsWith(PERSONAL_PREFIX))
    .forEach((key) => localStorage.removeItem(key));
  sessionStorage.clear();
}

/** Older versions kept this without a person. Nobody can say whose it is, so it goes. */
export function dropUnkeyedData(): void {
  storedKeys()
    .filter((key) => UNKEYED_NAMES.includes(key) || UNKEYED_PREFIXES.some((prefix) => key.startsWith(prefix)))
    .forEach((key) => localStorage.removeItem(key));
}

interface StoredDraft {
  saved: number;
  body: Json;
}

function parseDraft(stored: string | null): StoredDraft | null {
  try {
    return stored ? (JSON.parse(stored) as StoredDraft) : null;
  } catch {
    // An unreadable draft is no draft: the box simply starts empty.
    return null;
  }
}

const isFresh = (draft: StoredDraft | null, now: number): draft is StoredDraft =>
  draft !== null && now - draft.saved < DRAFT_KEEP_MS;

export function keepDraft(person: string, name: string, body: Json, now = Date.now()): void {
  localStorage.setItem(draftKey(person, name), JSON.stringify({ saved: now, body }));
}

export function dropDraft(person: string, name: string): void {
  localStorage.removeItem(draftKey(person, name));
}

/** A draft holds words of a page on a device, so it only stays for a short time. */
export function readDraft(person: string, name: string, now = Date.now()): Json | undefined {
  const draft = parseDraft(localStorage.getItem(draftKey(person, name)));
  if (isFresh(draft, now)) return draft.body;
  dropDraft(person, name);
  return undefined;
}

/** Drafts nobody came back for, of anyone. */
export function dropOldDrafts(now = Date.now()): void {
  storedKeys()
    .filter((key) => key.startsWith(PERSONAL_PREFIX) && key.includes(DRAFT_PART))
    .filter((key) => !isFresh(parseDraft(localStorage.getItem(key)), now))
    .forEach((key) => localStorage.removeItem(key));
}
