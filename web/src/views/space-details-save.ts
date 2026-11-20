// SPDX-License-Identifier: Apache-2.0
// Details is one form, but its parts are saved by five routes, each with its own rule and audit
// event (spec 083, D2). A Save sends only what changed, part by part in a fixed order, and stops at
// the first refusal. The key goes last, because the address follows it.

export type DetailsPart = 'details' | 'owners' | 'categories' | 'home' | 'key';

/** The order in which a Save sends the parts. */
const ORDER: DetailsPart[] = ['details', 'owners', 'categories', 'home', 'key'];

export interface DetailsValues {
  name: string;
  description: string;
  owners: string[];
  /** Null while the fold has not loaded them. */
  categories: string[] | null;
  /** Null while the fold has not loaded it, or for a space without a home page. */
  home: string | null;
  key: string;
}

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x));

/** The parts that differ from what is stored, in the order a Save sends them. */
export function changedParts(stored: DetailsValues, form: DetailsValues): DetailsPart[] {
  const differs: Record<DetailsPart, boolean> = {
    details: stored.name !== form.name || stored.description !== form.description,
    owners: !sameSet(stored.owners, form.owners),
    categories:
      stored.categories !== null && form.categories !== null && !sameSet(stored.categories, form.categories),
    home: form.home !== null && stored.home !== form.home,
    key: stored.key !== form.key,
  };
  return ORDER.filter((part) => differs[part]);
}

/**
 * The form after the stored values changed, such as when the fold loaded the categories or a Save
 * stopped half way: a field the person did not touch takes the new stored value, one they changed
 * keeps what they typed.
 */
export function rebased(before: DetailsValues, now: DetailsValues, form: DetailsValues): DetailsValues {
  const next = { ...form };
  for (const field of Object.keys(now) as (keyof DetailsValues)[]) {
    if (JSON.stringify(form[field]) === JSON.stringify(before[field])) {
      Object.assign(next, { [field]: now[field] });
    }
  }
  return next;
}

export interface SaveResult {
  saved: DetailsPart[];
  refused?: { part: DetailsPart; error: unknown };
}

/** Sends each part in turn and stops at the first one the server refuses. */
export async function saveInOrder(
  parts: DetailsPart[],
  send: (part: DetailsPart) => Promise<void>,
): Promise<SaveResult> {
  const saved: DetailsPart[] = [];
  for (const part of parts) {
    try {
      await send(part);
    } catch (error) {
      return { saved, refused: { part, error } };
    }
    saved.push(part);
  }
  return { saved };
}
