// SPDX-License-Identifier: Apache-2.0
// What the browser keeps for one person. On a shared computer the next person must see
// nothing of the last: keys hold the person's id, and logging out removes them all
// (SEC-265, SEC-266). Only work that never reached the server stays, under the id of the
// person who wrote it, for a short time. The theme and the sidebar belong to the device.
import { createContext, useContext } from 'react';
import type { Json } from './api';

const PERSONAL_PREFIX = 'diffluence.person.';
const DRAFT_PART = '.draft.';
const UNSAVED_PART = '.unsaved.';
const SHARED_PART = '.shared.';
const UNKEYED_NAMES = ['diffluence.searches'];
const UNKEYED_PREFIXES = ['diffluence.tree.', 'diffluence.comment.', 'diffluence.backup.'];
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

/** Page text that did not reach the server. Logging out must not cost someone their work. */
export const unsavedKey = (person: string, name: string) => personalKey(person, `unsaved.${name}`);

/** The database that holds a shared document in the browser, for working offline. */
export const sharedDocumentName = (person: string, page: string) => personalKey(person, `shared.${page}`);

const unsavedMarkOf = (documentName: string) => documentName.replace(SHARED_PART, `${UNSAVED_PART}shared.`);

/** Says that a shared document holds typing the service has not confirmed. */
export function markSharedUnsaved(person: string, page: string, now = Date.now()): void {
  localStorage.setItem(unsavedMarkOf(sharedDocumentName(person, page)), JSON.stringify({ saved: now }));
}

export function markSharedSaved(person: string, page: string): void {
  localStorage.removeItem(unsavedMarkOf(sharedDocumentName(person, page)));
}

/** Removes what this browser holds for anyone, except work that was never saved. */
export function forgetPersonalData(): void {
  storedKeys()
    .filter((key) => key.startsWith(PERSONAL_PREFIX) && !key.includes(UNSAVED_PART))
    .forEach((key) => localStorage.removeItem(key));
  sessionStorage.clear();
}

/** The shared documents to delete: all of ours, but not unsaved work and not the one in use. */
export function sharedDocumentsToForget(names: string[], person = ''): string[] {
  return names.filter(
    (name) =>
      name.startsWith(PERSONAL_PREFIX) &&
      name.includes(SHARED_PART) &&
      localStorage.getItem(unsavedMarkOf(name)) === null &&
      !(person !== '' && name.startsWith(personalKey(person, ''))),
  );
}

/**
 * Deletes shared documents from the browser. At logout nobody stays, and when the app
 * starts it clears those of others, who may never have logged out. A database still
 * open goes when the editor closes it.
 */
export async function forgetSharedDocuments(person = ''): Promise<void> {
  const databases = await indexedDB.databases();
  sharedDocumentsToForget(
    databases.flatMap((d) => (d.name ? [d.name] : [])),
    person,
  ).forEach((name) => indexedDB.deleteDatabase(name));
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
    .filter(
      (key) => key.startsWith(PERSONAL_PREFIX) && (key.includes(DRAFT_PART) || key.includes(UNSAVED_PART)),
    )
    .filter((key) => !isFresh(parseDraft(localStorage.getItem(key)), now))
    .forEach((key) => localStorage.removeItem(key));
}
