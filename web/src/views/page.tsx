// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageDetail, type Space } from '../api';
import { Link, navigate } from '../router';
import { usePreferences } from '../preferences';
import { DocumentView } from '../components/editor/editor';
import { contentsOf, countWords, jumpToHeading, readingMinutes } from '../components/editor/outline';
import {
  IconAttach,
  IconClock,
  IconCopy,
  IconDownload,
  IconInfo,
  IconLock,
  IconMore,
  IconMove,
  IconPencil,
  IconPresent,
  IconPrint,
  IconReader,
  IconRename,
  IconStar,
  IconTrash,
} from '../components/icons';
import { Menu, MenuDownload, MenuItem, MenuLink, MenuSeparator } from '../components/menu';
import { MIN_HEADINGS_FOR_CONTENTS, PageContents } from '../components/page-contents';
import { ChildPages, DetailsPanel } from '../components/page-details';
import { useShortcut } from '../components/shortcuts';
import { Avatar, ErrorNote, formatDay, Loading } from '../components/ui';
import { Lightbox, Presentation, type ShownImage } from '../components/viewers';
import { useDocumentTitle, useScrollMemory } from '../page-title';
import {
  AttachmentsDialog,
  CopyDialog,
  DeleteDialog,
  MoveDialog,
  RenameDialog,
  RestrictionsDialog,
} from './page-dialogs';

type PageDialog = 'attachments' | 'restrictions' | 'move' | 'rename' | 'copy' | 'delete';

const CRUMBS_BEFORE_FOLDING = 4;
const MENU_ICON_SIZE = 18;
const JUMP_ATTEMPTS = 30;

function Crumb({ page }: { page: { id: string; title: string; emoji: string } }) {
  return (
    <Link href={`/pages/${page.id}`}>
      {page.emoji && <span aria-hidden="true">{page.emoji} </span>}
      {page.title}
    </Link>
  );
}

function Breadcrumbs({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const space = useQuery({
    queryKey: ['space-id', page.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.space_id}`),
  });
  const crumbs = page.breadcrumbs.filter((c) => c.id !== space.data?.home_page_id);
  const folded = crumbs.length > CRUMBS_BEFORE_FOLDING ? crumbs.slice(0, -1) : [];
  const shown = folded.length > 0 ? crumbs.slice(-1) : crumbs;
  return (
    <nav aria-label={t('page.breadcrumbs')} className="crumbs">
      <ol>
        {space.data && (
          <li>
            <Link href={`/spaces/${space.data.key}`}>{space.data.name}</Link>
          </li>
        )}
        {folded.length > 0 && (
          <li>
            <Menu label={t('page.moreCrumbs')} align="start" className="crumb-fold" trigger="…">
              {folded.map((c) => (
                <MenuLink key={c.id} href={`/pages/${c.id}`}>
                  {c.title}
                </MenuLink>
              ))}
            </Menu>
          </li>
        )}
        {shown.map((c) => (
          <li key={c.id}>
            <Crumb page={c} />
          </li>
        ))}
        {page.id === space.data?.home_page_id && <li aria-current="page">{t('space.home')}</li>}
      </ol>
    </nav>
  );
}

function Byline({ page, onDetails }: { page: PageDetail; onDetails: () => void }) {
  const { t } = useTranslation();
  const author = page.created_by_name || t('common.unknown');
  const minutes = useMemo(() => readingMinutes(countWords(page.document)), [page.document]);
  return (
    <div className="byline">
      <Avatar name={author} size="small" />
      <button type="button" className="byline-author" onClick={onDetails}>
        {t('page.byline', { name: author })}
      </button>
      <span aria-hidden="true">·</span>
      <span data-testid="last-updated">{t('page.lastUpdated', { date: formatDay(page.updated_at) })}</span>
      {page.current_version > 0 && (
        <>
          <span aria-hidden="true">·</span>
          <span>{t('history.version', { number: page.current_version })}</span>
        </>
      )}
      {minutes !== null && (
        <>
          <span aria-hidden="true">·</span>
          <span>{t('page.readTime', { minutes })}</span>
        </>
      )}
      {page.restricted && <span className="lozenge">{t('page.restricted')}</span>}
    </div>
  );
}

function MenuEntry({
  icon,
  onClick,
  className,
  children,
}: {
  icon: ReactNode;
  onClick: () => void;
  className?: string;
  children: ReactNode;
}) {
  return (
    <MenuItem className={className} onClick={onClick}>
      {icon} {children}
    </MenuItem>
  );
}

function PageMenu({
  page,
  onOpen,
  onDetails,
  onReader,
  onPresent,
}: {
  page: PageDetail;
  onOpen: (dialog: PageDialog) => void;
  onDetails: () => void;
  onReader: () => void;
  onPresent: () => void;
}) {
  const { t } = useTranslation();
  const canEdit = page.can['page.edit'];
  const published = page.current_version > 0;
  return (
    <Menu label={t('page.moreActions')} trigger={<IconMore />}>
      <MenuLink href={`/pages/${page.id}/history`}>
        <IconClock size={MENU_ICON_SIZE} /> {t('page.history')}
      </MenuLink>
      <MenuEntry icon={<IconInfo size={MENU_ICON_SIZE} />} onClick={onDetails}>
        {t('details.title')}
      </MenuEntry>
      <MenuEntry icon={<IconAttach size={MENU_ICON_SIZE} />} onClick={() => onOpen('attachments')}>
        {t('attachments.title')}
      </MenuEntry>
      {canEdit && (
        <>
          <MenuEntry icon={<IconRename size={MENU_ICON_SIZE} />} onClick={() => onOpen('rename')}>
            {t('page.rename')}
          </MenuEntry>
          <MenuEntry icon={<IconMove size={MENU_ICON_SIZE} />} onClick={() => onOpen('move')}>
            {t('page.move')}
          </MenuEntry>
        </>
      )}
      {page.can['page.create'] && published && (
        <MenuEntry icon={<IconCopy size={MENU_ICON_SIZE} />} onClick={() => onOpen('copy')}>
          {t('page.copy')}
        </MenuEntry>
      )}
      <MenuSeparator />
      <MenuEntry icon={<IconReader size={MENU_ICON_SIZE} />} onClick={onReader}>
        {t('page.readerMode')}
      </MenuEntry>
      <MenuEntry icon={<IconPresent size={MENU_ICON_SIZE} />} onClick={onPresent}>
        {t('page.present')}
      </MenuEntry>
      <MenuEntry icon={<IconPrint size={MENU_ICON_SIZE} />} onClick={() => window.print()}>
        {t('page.print')}
      </MenuEntry>
      {page.can['space.export'] && published && (
        <>
          <MenuSeparator />
          <MenuDownload href={`/api/v1/pages/${page.id}/export.md`}>
            <IconDownload size={MENU_ICON_SIZE} /> {t('page.exportMd')}
          </MenuDownload>
          <MenuDownload href={`/api/v1/pages/${page.id}/export.zip`}>
            <IconDownload size={MENU_ICON_SIZE} /> {t('page.exportZip')}
          </MenuDownload>
        </>
      )}
      {page.can['page.delete'] && (
        <>
          <MenuSeparator />
          <MenuEntry
            className="danger"
            icon={<IconTrash size={MENU_ICON_SIZE} />}
            onClick={() => onOpen('delete')}
          >
            {t('page.delete')}
          </MenuEntry>
        </>
      )}
    </Menu>
  );
}

/** Hides the bars around a page, to read without distraction. */
function useReaderMode(): [boolean, (on: boolean) => void] {
  const [on, setOn] = useState(false);
  useEffect(() => {
    if (!on) return;
    document.documentElement.dataset.reader = '';
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOn(false);
    };
    document.addEventListener('keydown', onKey);
    return () => {
      delete document.documentElement.dataset.reader;
      document.removeEventListener('keydown', onKey);
    };
  }, [on]);
  return [on, setOn];
}

/** An address with a heading jumps there, once the document has been drawn. */
function useJumpToAddress(pageId: string, ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    let frame = 0;
    const jump = () => {
      const slug = decodeURIComponent(window.location.hash.slice(1));
      let attempts = 0;
      const attempt = () => {
        if (!slug || jumpToHeading(slug, false) || attempts++ >= JUMP_ATTEMPTS) return;
        frame = requestAnimationFrame(attempt);
      };
      attempt();
    };
    jump();
    window.addEventListener('hashchange', jump);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener('hashchange', jump);
    };
  }, [pageId, ready]);
}

function HeaderImage({ page }: { page: PageDetail }) {
  const image = useRef<HTMLImageElement>(null);
  useEffect(() => {
    image.current?.style.setProperty('--header-position', `${page.header_position}%`);
  }, [page.header_position]);
  return (
    <img
      ref={image}
      className="page-header-image"
      src={`/api/v1/attachments/${page.header_attachment_id}/content`}
      alt=""
    />
  );
}

export function PageView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { preferences } = usePreferences();
  const q = useQuery({ queryKey: ['page', pageId], queryFn: () => api.get<PageDetail>(`/pages/${pageId}`) });
  const [dialog, setDialog] = useState<PageDialog | null>(null);
  const [details, setDetails] = useState(false);
  const [presenting, setPresenting] = useState(false);
  const [lightbox, setLightbox] = useState<{ images: ShownImage[]; start: number } | null>(null);
  const [reader, setReader] = useReaderMode();
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${pageId}/favourite`) : api.del(`/pages/${pageId}/favourite`),
    onSettled: () => client.invalidateQueries({ queryKey: ['page', pageId] }),
  });
  const page = q.data;
  const closeDetails = useCallback(() => setDetails(false), []);
  const headings = useMemo(() => (page ? contentsOf(page.document) : []), [page]);

  useDocumentTitle(page ? `${page.emoji ? `${page.emoji} ` : ''}${page.title}` : undefined);
  useJumpToAddress(pageId, !!page);
  useScrollMemory(!!page);
  useShortcut('edit', () => {
    if (page?.can['page.edit']) navigate(`/pages/${pageId}/edit`);
  });
  useShortcut('favourite', () => {
    if (page) favourite.mutate(!page.favourite);
  });
  useShortcut('details', () => setDetails(!details));
  useShortcut('reader', () => setReader(!reader));

  if (q.isPending) return <Loading />;
  if (q.error || !page) return <ErrorNote error={q.error} />;
  const close = () => setDialog(null);
  const favouriteLabel = page.favourite ? t('page.unfavourite') : t('page.favourite');
  const showContents = preferences.contents && headings.length >= MIN_HEADINGS_FOR_CONTENTS;
  return (
    <div className={`page-layout ${showContents ? 'with-contents' : ''} ${page.full_width ? 'full' : ''}`}>
      <article className="page">
        {page.header_attachment_id && <HeaderImage page={page} />}
        <div className="page-top">
          <Breadcrumbs page={page} />
          <div className="page-actions">
            {page.can['page.edit'] && (
              <Link className="button" href={`/pages/${page.id}/edit`}>
                <IconPencil size={16} /> {t('page.edit')}
              </Link>
            )}
            <button
              type="button"
              className="icon"
              aria-label={favouriteLabel}
              title={favouriteLabel}
              aria-pressed={page.favourite}
              onClick={() => favourite.mutate(!page.favourite)}
            >
              <IconStar />
            </button>
            <button
              type="button"
              className="icon"
              aria-label={t('page.restrictions')}
              title={t('page.restrictions')}
              onClick={() => setDialog('restrictions')}
            >
              <IconLock />
            </button>
            <PageMenu
              page={page}
              onOpen={setDialog}
              onDetails={() => setDetails(true)}
              onReader={() => setReader(true)}
              onPresent={() => setPresenting(true)}
            />
          </div>
        </div>
        <h1 className={page.title_align === 'center' ? 'title-center' : undefined}>
          {page.emoji && (
            <span className="title-emoji" aria-hidden="true">
              {page.emoji}{' '}
            </span>
          )}
          {page.title}
        </h1>
        <Byline page={page} onDetails={() => setDetails(true)} />
        <ErrorNote error={favourite.error} />
        {page.status === 'draft' && (
          <p role="status" className="note">
            {t('page.unpublished')}
          </p>
        )}
        {page.has_draft && page.status !== 'draft' && (
          <p role="status" className="note">
            <Link href={`/pages/${page.id}/edit`}>{t('page.draftExists')}</Link>
          </p>
        )}
        <DocumentView document={page.document} onImage={(images, start) => setLightbox({ images, start })} />
        {!page.hide_children && <ChildPages pageId={page.id} />}
        {reader && (
          <button type="button" className="reader-exit" onClick={() => setReader(false)}>
            {t('page.readerExit')}
          </button>
        )}
      </article>
      {showContents && <PageContents headings={headings} />}
      {dialog === 'attachments' && <AttachmentsDialog page={page} onClose={close} />}
      {dialog === 'restrictions' && <RestrictionsDialog page={page} onClose={close} />}
      {dialog === 'move' && <MoveDialog page={page} onClose={close} />}
      {dialog === 'rename' && <RenameDialog page={page} onClose={close} />}
      {dialog === 'copy' && <CopyDialog page={page} onClose={close} />}
      {dialog === 'delete' && <DeleteDialog page={page} onClose={close} />}
      {details && <DetailsPanel page={page} onClose={closeDetails} />}
      {lightbox && (
        <Lightbox images={lightbox.images} start={lightbox.start} onClose={() => setLightbox(null)} />
      )}
      {presenting && (
        <Presentation title={page.title} document={page.document} onClose={() => setPresenting(false)} />
      )}
    </div>
  );
}
