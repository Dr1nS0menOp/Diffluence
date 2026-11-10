// SPDX-License-Identifier: Apache-2.0
// Which permission makes which kind of item (spec 045). The server decides; this only keeps
// the screens from offering what it would refuse.

/** What a space or a page says a person may do. */
type Capabilities = Record<string, boolean> | undefined;

/** The permission that makes an item of a kind: posts and databases have their own. */
export function createPermission(kind: string): string {
  if (kind === 'post') return 'post.create';
  if (kind === 'database') return 'database.create';
  return 'page.create';
}

/** Whether a person may make an item of a kind, by what the space or page says. */
export function mayCreate(can: Capabilities, kind: string): boolean {
  return can?.[createPermission(kind)] ?? false;
}

/** Whether a person may make anything at all in a space. */
export function mayCreateAnything(can: Capabilities): boolean {
  return mayCreate(can, 'page') || mayCreate(can, 'post') || mayCreate(can, 'database');
}
