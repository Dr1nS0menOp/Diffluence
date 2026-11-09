// SPDX-License-Identifier: Apache-2.0
// Spec 041: a reload the app offers waits for open work to be saved first. Screens that hold
// unsaved work listen for the event and hand in a promise. No shared state: the event carries
// the list. Spec 043 adds boxes that are open with text: they say so for the next load of the
// tab, which opens them again.
import { useEffect, useRef, useState } from 'react';

const BEFORE_RELOAD = 'diffluence:before-reload';
const SAVE_WAIT_MS = 5_000;
// sessionStorage, so it belongs to this tab and goes with it.
const REOPEN_KEY = 'diffluence.reopen';

interface BeforeReloadDetail {
  wait: (work: Promise<unknown>) => void;
}

/** Reloads the tab once open work is saved, or after a short wait, whichever comes first. */
export async function reloadWhenSaved(): Promise<void> {
  const waits: Promise<unknown>[] = [];
  window.dispatchEvent(
    new CustomEvent<BeforeReloadDetail>(BEFORE_RELOAD, { detail: { wait: (work) => waits.push(work) } }),
  );
  await Promise.race([Promise.allSettled(waits), new Promise((done) => setTimeout(done, SAVE_WAIT_MS))]);
  window.location.reload();
}

/** Calls save before a reload the app offers, until the returned function is called. */
export function onBeforeReload(save: () => Promise<unknown>): () => void {
  const listen = (event: Event) => (event as CustomEvent<BeforeReloadDetail>).detail.wait(save());
  window.addEventListener(BEFORE_RELOAD, listen);
  return () => window.removeEventListener(BEFORE_RELOAD, listen);
}

/** Lets a screen save its work before a reload the app offers. */
export function useSaveBeforeReload(save: () => Promise<unknown>) {
  const latest = useRef(save);
  latest.current = save;
  useEffect(() => onBeforeReload(() => latest.current()), []);
}

function reopenList(): string[] {
  try {
    const list: unknown = JSON.parse(sessionStorage.getItem(REOPEN_KEY) ?? '[]');
    return Array.isArray(list) ? list.filter((name): name is string => typeof name === 'string') : [];
  } catch {
    // A list this tab cannot read opens nothing again; the drafts themselves stay.
    return [];
  }
}

/** Remembers, for the next load of this tab, that the box with this name was open. */
export function keepOpenAcrossReload(name: string): void {
  const list = reopenList();
  if (!list.includes(name)) sessionStorage.setItem(REOPEN_KEY, JSON.stringify([...list, name]));
}

/** Whether the box with this name was open when the tab reloaded. It is said once: the next load forgets it. */
export function useOpenAfterReload(name: string): boolean {
  const [wasOpen] = useState(() => reopenList().includes(name));
  useEffect(() => {
    const list = reopenList();
    if (!list.includes(name)) return;
    const rest = list.filter((n) => n !== name);
    if (rest.length === 0) sessionStorage.removeItem(REOPEN_KEY);
    else sessionStorage.setItem(REOPEN_KEY, JSON.stringify(rest));
  }, [name]);
  return wasOpen;
}
