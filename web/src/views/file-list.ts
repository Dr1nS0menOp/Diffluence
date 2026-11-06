// SPDX-License-Identifier: Apache-2.0
// The order of the files of a page (FIL-034). The server sends them all, so this happens here.
import type { Attachment } from '../api';

export const FILE_ORDERS = ['name', 'date', 'size', 'type'] as const;

export type FileOrder = (typeof FILE_ORDERS)[number];

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

const extension = (filename: string) => filename.slice(filename.lastIndexOf('.') + 1);

const COMPARE: Record<FileOrder, (a: Attachment, b: Attachment) => number> = {
  name: (a, b) => collator.compare(a.filename, b.filename),
  date: (a, b) => b.updated_at.localeCompare(a.updated_at),
  size: (a, b) => b.size - a.size,
  type: (a, b) => collator.compare(extension(a.filename), extension(b.filename)),
};

/** What fits the filter, in the chosen order. The newest and the largest come first. */
export function arrangedFiles(files: Attachment[], order: FileOrder, filter: string): Attachment[] {
  const needle = filter.trim().toLowerCase();
  return files
    .filter((file) => file.filename.toLowerCase().includes(needle))
    .sort((a, b) => COMPARE[order](a, b) || COMPARE.name(a, b));
}
