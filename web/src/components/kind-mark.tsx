// SPDX-License-Identifier: Apache-2.0
// The kind of an item before its title in a list: a page's own emoji where it has one,
// otherwise an icon of its kind, with the kind in words for screen readers and as a tooltip,
// so a kind never rests on colour or shape alone (NAV-156).
import type { ComponentType, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconAttach,
  IconBlog,
  IconFilePdf,
  IconFileText,
  IconFolder,
  IconHome,
  IconLink,
  IconPage,
  IconPicture,
  IconPresent,
  IconSheet,
  IconTable,
  IconTask,
  IconTick,
} from './icons';

const MARK_ICON_SIZE = 16;

export type PageKindName = 'page' | 'home' | 'post' | 'database' | 'folder' | 'link';
export type FileKindName = 'picture' | 'pdf' | 'document' | 'slides' | 'sheet' | 'file';

/** What a list knows of a page: its kind, whether it is a space's home page, its emoji. */
export interface PageKindFacts {
  kind?: string;
  home?: boolean;
  emoji?: string;
}

const PAGE_KINDS: readonly PageKindName[] = ['post', 'database', 'folder', 'link'];

/** The kind a list shows for a page; a kind it does not know shows as a page. */
export function pageKindOf(item: PageKindFacts): PageKindName {
  if (item.home) return 'home';
  return PAGE_KINDS.find((kind) => kind === item.kind) ?? 'page';
}

const FILE_KINDS: Record<string, FileKindName> = {
  png: 'picture',
  jpg: 'picture',
  jpeg: 'picture',
  gif: 'picture',
  webp: 'picture',
  svg: 'picture',
  pdf: 'pdf',
  doc: 'document',
  docx: 'document',
  odt: 'document',
  txt: 'document',
  md: 'document',
  rtf: 'document',
  ppt: 'slides',
  pptx: 'slides',
  odp: 'slides',
  xls: 'sheet',
  xlsx: 'sheet',
  ods: 'sheet',
  csv: 'sheet',
};

/** The kind of a file, from the extension of its name. */
export function fileKindOf(filename: string): FileKindName {
  const dot = filename.lastIndexOf('.');
  if (dot < 0) return 'file';
  return FILE_KINDS[filename.slice(dot + 1).toLowerCase()] ?? 'file';
}

const PAGE_ICONS: Record<PageKindName, ComponentType<{ size?: number }>> = {
  page: IconPage,
  home: IconHome,
  post: IconBlog,
  database: IconSheet,
  folder: IconFolder,
  link: IconLink,
};

const FILE_ICONS: Record<FileKindName, ComponentType<{ size?: number }>> = {
  picture: IconPicture,
  pdf: IconFilePdf,
  document: IconFileText,
  slides: IconPresent,
  sheet: IconTable,
  file: IconAttach,
};

function Mark({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className="kind-mark" role="img" aria-label={label} title={label}>
      {children}
    </span>
  );
}

/** The mark of a page, a blog post, a database, a folder or a link. */
export function KindMark({ item }: { item: PageKindFacts }) {
  const { t } = useTranslation();
  const kind = pageKindOf(item);
  const label = t(`kinds.page_${kind}`);
  if (item.emoji) {
    return (
      <Mark label={label}>
        <span className="page-emoji">{item.emoji}</span>
      </Mark>
    );
  }
  const Icon = PAGE_ICONS[kind];
  return (
    <Mark label={label}>
      <Icon size={MARK_ICON_SIZE} />
    </Mark>
  );
}

/** The mark of a task, open or done (COL-040). */
export function TaskMark({ done }: { done: boolean }) {
  const { t } = useTranslation();
  return (
    <Mark label={t(done ? 'kinds.task_done' : 'kinds.task_open')}>
      {done ? <IconTick size={MARK_ICON_SIZE} /> : <IconTask size={MARK_ICON_SIZE} />}
    </Mark>
  );
}

/** The mark of a file, by the kind its name says. */
export function FileMark({ filename }: { filename: string }) {
  const { t } = useTranslation();
  const kind = fileKindOf(filename);
  const Icon = FILE_ICONS[kind];
  return (
    <Mark label={t(`kinds.file_${kind}`)}>
      <Icon size={MARK_ICON_SIZE} />
    </Mark>
  );
}
