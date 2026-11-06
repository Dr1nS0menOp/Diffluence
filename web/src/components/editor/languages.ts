// SPDX-License-Identifier: Apache-2.0

/**
 * The languages to offer: what the page already uses first, then A to Z. An alias
 * such as `js` is not in the list of names, but is a good answer to what was typed.
 */
export function orderLanguages(
  names: string[],
  used: string[],
  filter: string,
  filterIsAlias: boolean,
): string[] {
  const fits = (name: string) => name.toLowerCase().includes(filter);
  const usedFirst = used.filter(fits);
  const others = names.filter((name) => fits(name) && !used.includes(name)).sort();
  const alias = filterIsAlias && !names.includes(filter) && !used.includes(filter) ? [filter] : [];
  return [...alias, ...usedFirst, ...others];
}
