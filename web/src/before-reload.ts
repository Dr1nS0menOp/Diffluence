// SPDX-License-Identifier: Apache-2.0
// Spec 041: a reload the app offers waits for open work to be saved first. Screens that hold
// unsaved work listen for the event and hand in a promise. No shared state: the event carries
// the list.
import { useEffect, useRef } from 'react';

const BEFORE_RELOAD = 'diffluence:before-reload';
const SAVE_WAIT_MS = 5_000;

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

/** Lets a screen save its work before a reload the app offers. */
export function useSaveBeforeReload(save: () => Promise<unknown>) {
  const latest = useRef(save);
  latest.current = save;
  useEffect(() => {
    const listen = (event: Event) => (event as CustomEvent<BeforeReloadDetail>).detail.wait(latest.current());
    window.addEventListener(BEFORE_RELOAD, listen);
    return () => window.removeEventListener(BEFORE_RELOAD, listen);
  }, []);
}
