// SPDX-License-Identifier: Apache-2.0
// The words a page goes by on screen. A guided template may leave a title empty (spec 074),
// and a link or a heading without words has no name for a screen reader.

type Translate = (key: string) => string;

/** The title of a page as people see it: its own title, or "Untitled" when it has none. */
export function pageTitle(t: Translate, title: string | null | undefined): string {
  return title?.trim() ? title : t('page.untitled');
}
