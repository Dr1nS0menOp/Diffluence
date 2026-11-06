// SPDX-License-Identifier: Apache-2.0
// The animals of spaces (spec 018). The pictures are CC0, from Kenney's Animal Pack
// Remastered, and live in web/public/animals so they load only when shown.

export const ANIMALS = [
  'bear',
  'buffalo',
  'chick',
  'chicken',
  'cow',
  'crocodile',
  'dog',
  'duck',
  'elephant',
  'frog',
  'giraffe',
  'goat',
  'gorilla',
  'hippo',
  'horse',
  'monkey',
  'moose',
  'narwhal',
  'owl',
  'panda',
  'parrot',
  'penguin',
  'pig',
  'rabbit',
  'rhino',
  'sloth',
  'snake',
  'walrus',
  'whale',
  'zebra',
] as const;

export type Animal = (typeof ANIMALS)[number];

const FNV_OFFSET = 0x811c9dc5;
const FNV_PRIME = 0x01000193;

/** Keys that look alike should not land on the same animal, as a sum of letters would. */
function hashOf(text: string): number {
  let hash = FNV_OFFSET;
  for (const character of text) {
    hash = Math.imul(hash ^ (character.codePointAt(0) ?? 0), FNV_PRIME);
  }
  return hash >>> 0;
}

export function isAnimal(icon: string): icon is Animal {
  return (ANIMALS as readonly string[]).includes(icon);
}

/** The animal of a space: the one it chose, or one from its key. A character of its own means none. */
export function animalOf(space: { key: string; icon: string }): Animal | null {
  const icon = space.icon.trim();
  if (isAnimal(icon)) return icon;
  if (icon) return null;
  return ANIMALS[hashOf(space.key.toLowerCase()) % ANIMALS.length] ?? null;
}

export const animalPicture = (animal: Animal) => `/animals/${animal}.png`;
