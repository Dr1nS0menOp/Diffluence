// SPDX-License-Identifier: Apache-2.0
// Spec 043: what someone typed into a settings form survives a reload the app offers for a
// new version. Before the reload, every field typed in is written to this tab's
// sessionStorage; after it, each comes back as its form appears again. A password never is.
import { onBeforeReload } from './before-reload';

const KEPT_KEY = 'diffluence.kept-fields';
const SETTINGS_FORMS = '.settings-area form';
const KEPT_TYPES = new Set(['text', 'email', 'url', 'number', 'tel']);
// Long enough for a settings screen to load its data, short enough that a value never
// lands in a form opened much later.
const RESTORE_WINDOW_MS = 15_000;

type TextField = HTMLInputElement | HTMLTextAreaElement;

interface Kept {
  path: string;
  fields: Record<string, string>;
}

function isKeptField(target: EventTarget | null): target is TextField {
  if (target instanceof HTMLTextAreaElement) return true;
  return target instanceof HTMLInputElement && KEPT_TYPES.has(target.type);
}

function here(): string {
  return window.location.pathname + window.location.search;
}

/** The text fields of the settings forms on screen, by a key that is the same after a reload. */
export function settingsFields(root: ParentNode = document): Map<string, TextField> {
  const out = new Map<string, TextField>();
  root.querySelectorAll(SETTINGS_FORMS).forEach((form, f) => {
    [...form.querySelectorAll('input, textarea')].filter(isKeptField).forEach((field, i) => {
      out.set(`${f}:${i}:${field.name || field.getAttribute('aria-label') || ''}`, field);
    });
  });
  return out;
}

/** Reads what the last load of this tab kept, if it is valid. */
export function parseKept(text: string | null): Kept | null {
  if (!text) return null;
  try {
    const value: unknown = JSON.parse(text);
    if (typeof value !== 'object' || value === null) return null;
    const { path, fields } = value as Record<string, unknown>;
    if (typeof path !== 'string' || typeof fields !== 'object' || fields === null) return null;
    const entries = Object.entries(fields).filter((e): e is [string, string] => typeof e[1] === 'string');
    return { path, fields: Object.fromEntries(entries) };
  } catch {
    // Text this tab cannot read brings nothing back; the form shows what was saved.
    return null;
  }
}

/** Puts text in a field the way typing does, so the screen's own state follows. */
function typeInto(field: TextField, value: string) {
  const proto =
    field instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(proto, 'value')?.set?.call(field, value);
  field.dispatchEvent(new Event('input', { bubbles: true }));
}

/**
 * Brings back kept text as its fields appear, for a short while, then forgets it. Returns how
 * to stop early; stopping early keeps the text, so an installation that follows still has it.
 */
function restore(kept: Kept, typed: WeakSet<TextField>): () => void {
  const left = new Map(Object.entries(kept.fields));
  const observer = new MutationObserver(() => void fill());
  const timer = window.setTimeout(() => finish(), RESTORE_WINDOW_MS);
  let stopped = false;
  let filling = false;
  function stop() {
    stopped = true;
    observer.disconnect();
    window.clearTimeout(timer);
  }
  function finish() {
    stop();
    sessionStorage.removeItem(KEPT_KEY);
  }
  async function fill() {
    if (filling) return;
    filling = true;
    for (const [key, field] of settingsFields()) {
      const value = left.get(key);
      if (stopped || value === undefined) continue;
      typeInto(field, value);
      typed.add(field);
      left.delete(key);
      // One field at a time, as typing goes: a form that copies its state on each change
      // must see the previous field before the next.
      await new Promise((done) => window.setTimeout(done));
    }
    filling = false;
    if (!stopped && left.size === 0) finish();
  }
  observer.observe(document.body, { childList: true, subtree: true });
  void fill();
  return stop;
}

/**
 * Keeps typed text of settings forms across a reload the app offers. The state lives in this
 * call, so each installation is separate; the returned function removes it.
 */
export function keepSettingsTyping(): () => void {
  const typed = new WeakSet<TextField>();
  const kept = parseKept(sessionStorage.getItem(KEPT_KEY));
  if (kept?.path !== here()) sessionStorage.removeItem(KEPT_KEY);
  const stopRestoring = kept && kept.path === here() ? restore(kept, typed) : () => undefined;
  const noteTyping = (event: Event) => {
    if (isKeptField(event.target) && event.target.closest(SETTINGS_FORMS)) typed.add(event.target);
  };
  document.addEventListener('input', noteTyping, true);
  const stopSaving = onBeforeReload(async () => {
    const fields: Record<string, string> = {};
    for (const [key, field] of settingsFields()) if (typed.has(field)) fields[key] = field.value;
    if (Object.keys(fields).length > 0)
      sessionStorage.setItem(KEPT_KEY, JSON.stringify({ path: here(), fields }));
  });
  return () => {
    stopRestoring();
    stopSaving();
    document.removeEventListener('input', noteTyping, true);
  };
}
