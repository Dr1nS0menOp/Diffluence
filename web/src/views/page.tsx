// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useDates } from '../dates';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { EditorView } from '@tiptap/pm/view';
import { api, type CommentAnchor, type NotificationSettings, type PageDetail, type Space } from '../api';
import { Link, navigate, useLocation } from '../router';
import { usePreferences } from '../preferences';
import type { Highlight } from '../components/comments/anchor';
import { Reactions } from '../components/comments/reactions';
import { CommentSection, CommentsPanel, commentsQuery } from '../components/comments/section';
import { NewThread, SelectionBar } from '../components/comments/selection-bar';
import { commentsKey } from '../components/comments/thread';
import { DocumentView } from '../components/editor/editor';
import {
  contentsOf,
  countWords,
  jumpToComment,
  jumpToHeading,
  nextThread,
  readingMinutes,
} from '../components/editor/outline';
import {
  IconArchive,
  IconAttach,
  IconClock,
  IconComment,
  IconCopy,
  IconDownload,
  IconLink,
  IconEye,
  IconFolder,
  IconInfo,
  IconLock,
  IconMore,
  IconMove,
  IconPage,
  IconPencil,
  IconPresent,
  IconPrint,
  IconReader,
  IconRename,
  IconStar,
  IconTrash,
} from '../components/icons';
import { Menu, MenuItem, MenuLink, MenuSeparator } from '../components/menu';
import { LabelBar } from '../components/page-labels';
import {
  ApprovalBanner,
  ApprovalsDialog,
  PageNotes,
  StageControl,
  VerifiedMark,
  VerifyDialog,
} from '../components/page-marks';
import { MIN_HEADINGS_FOR_CONTENTS, PageContents } from '../components/page-contents';
import { ChildPages, DetailsPanel } from '../components/page-details';
import { ShareDialog } from '../components/share-dialog';
import { useShortcut } from '../components/shortcuts';
import { useToast } from '../components/toast';
import { Avatar, ErrorNote, Loading } from '../components/ui';
import { Lightbox, Presentation, type ShownImage } from '../components/viewers';
import { useDocumentTitle, useScrollMemory } from '../page-title';
import { ExportDialog, PublicLinkDialog, PublicMark } from './exchange';
import { CopyDialog, MoveDialog } from './move-dialogs';
import { ArchiveDialog, KindDialog, SaveTemplateDialog } from './page-actions';
import { PresencePeople, usePresence, useViewCount } from '../components/presence';
import { AttachmentsDialog, DeleteDialog, RenameDialog, RestrictionsDialog } from './page-dialogs';

type PageDialog =
  | 'attachments'
  | 'export'
  | 'public-link'
  | 'restrictions'
  | 'move'
  | 'rename'
  | 'copy'
  | 'delete'
  | 'share'
  | 'approvals'
  | 'verify'
  | 'archive'
  | 'template'
  | 'folder'
  | 'page'
  | 'post';
type WatchScope = 'page' | 'branch';

interface WatchState {
  page: boolean;
  branch: boolean;
  space: boolean;
}

const CRUMBS_BEFORE_FOLDING = 4;
const MENU_ICON_SIZE = 18;
// Frames to wait for an address to appear: comments arrive after the document.
const JUMP_ATTEMPTS = 120;
const ATTACHMENT_ADDRESS = /\/attachments\/([0-9a-f-]+)\//;
const THREADS_CLOSED = { open: false, thread: null };

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

function Byline({
  page,
  comments,
  views,
  onDetails,
  onComments,
}: {
  page: PageDetail;
  comments: number;
  views: number | null;
  onDetails: () => void;
  onComments: () => void;
}) {
  const { t } = useTranslation();
  const dates = useDates();
  const author = page.owner_name || page.created_by_name || t('common.unknown');
  const minutes = useMemo(() => readingMinutes(countWords(page.document)), [page.document]);
  return (
    <div className="byline">
      <Avatar name={author} size="small" />
      <button type="button" className="byline-author" onClick={onDetails}>
        {t('page.byline', { name: author })}
      </button>
      {page.owner_disabled && <span className="muted">{t('content.noLongerActive')}</span>}
      <span aria-hidden="true">·</span>
      <span data-testid="last-updated">{t('page.lastUpdated', { date: dates.day(page.updated_at) })}</span>
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
      {comments > 0 && (
        <>
          <span aria-hidden="true">·</span>
          <button type="button" className="byline-author" onClick={onComments}>
            {t('comments.count', { count: comments })}
          </button>
        </>
      )}
      {views !== null && (
        <>
          <span aria-hidden="true">·</span>
          <Link href={`/pages/${page.id}/insights`}>{t('insights.viewCount', { count: views })}</Link>
        </>
      )}
      {page.restricted && <span className="lozenge">{t('page.restricted')}</span>}
      <PublicMark pageId={page.id} />
      <StageControl page={page} />
      <VerifiedMark page={page} />
    </div>
  );
}

/** Rules someone can start by hand, with their names, in More. */
function ManualRules({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const toast = useToast();
  const q = useQuery({
    queryKey: ['page-rules', pageId],
    queryFn: () => api.get<{ rules: { id: string; name: string }[] }>(`/pages/${pageId}/rules`),
  });
  const run = useMutation({
    mutationFn: (id: string) => api.post(`/rules/${id}/run`, { page_id: pageId }),
    onSuccess: () => toast({ message: t('rules2.started') }),
  });
  return (
    <>
      {q.data?.rules.map((rule) => (
        <MenuItem key={rule.id} onClick={() => run.mutate(rule.id)}>
          {t('rules2.runOne', { name: rule.name })}
        </MenuItem>
      ))}
    </>
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
  watching,
  onOpen,
  onDetails,
  onReader,
  onPresent,
  onComments,
  onWatch,
}: {
  page: PageDetail;
  watching: WatchState | undefined;
  onOpen: (dialog: PageDialog) => void;
  onDetails: () => void;
  onReader: () => void;
  onPresent: () => void;
  onComments: () => void;
  onWatch: (scope: WatchScope, on: boolean) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const canEdit = page.can['page.edit'];
  const published = page.current_version > 0;
  const ask = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/access-requests`, { kind: 'edit' }),
    onSuccess: () => toast({ message: t('share.asked') }),
    onError: (e) => toast({ message: e.message }),
  });
  const closeComments = useMutation({
    mutationFn: () => api.patch(`/pages/${page.id}/comments-closed`, { closed: !page.comments_closed }),
    onError: (e) => toast({ message: e.message }),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      await client.invalidateQueries({ queryKey: commentsKey(page.id) });
    },
  });
  return (
    <Menu label={t('page.moreActions')} trigger={<IconMore />}>
      <ManualRules pageId={page.id} />
      <MenuLink href={`/pages/${page.id}/history`}>
        <IconClock size={MENU_ICON_SIZE} /> {t('page.history')}
      </MenuLink>
      <MenuEntry icon={<IconInfo size={MENU_ICON_SIZE} />} onClick={onDetails}>
        {t('details.title')}
      </MenuEntry>
      <MenuEntry icon={<IconAttach size={MENU_ICON_SIZE} />} onClick={() => onOpen('attachments')}>
        {t('attachments.title')}
      </MenuEntry>
      {published && (
        <MenuEntry icon={<IconComment size={MENU_ICON_SIZE} />} onClick={onComments}>
          {t('comments.title')}
        </MenuEntry>
      )}
      <MenuEntry icon={<IconEye size={MENU_ICON_SIZE} />} onClick={() => onWatch('page', !watching?.page)}>
        {watching?.page ? t('watching.stopPage') : t('watching.watchPage')}
      </MenuEntry>
      <MenuEntry
        icon={<IconEye size={MENU_ICON_SIZE} />}
        onClick={() => onWatch('branch', !watching?.branch)}
      >
        {watching?.branch ? t('watching.stopBranch') : t('watching.watchBranch')}
      </MenuEntry>
      {!canEdit && published && (
        <MenuEntry icon={<IconPencil size={MENU_ICON_SIZE} />} onClick={() => ask.mutate()}>
          {t('share.askToEdit')}
        </MenuEntry>
      )}
      {canEdit && (
        <>
          {published && (
            <MenuEntry icon={<IconComment size={MENU_ICON_SIZE} />} onClick={() => closeComments.mutate()}>
              {page.comments_closed ? t('comments.openAgain') : t('comments.closePage')}
            </MenuEntry>
          )}
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
      <MenuEntry icon={<IconPage size={MENU_ICON_SIZE} />} onClick={() => onOpen('approvals')}>
        {t('organise.approvals')}
      </MenuEntry>
      {page.can['page.verify'] && !page.verified && (
        <MenuEntry icon={<IconPage size={MENU_ICON_SIZE} />} onClick={() => onOpen('verify')}>
          {t('organise.verify')}
        </MenuEntry>
      )}
      {page.can['space.administer'] && published && (
        <MenuEntry icon={<IconPage size={MENU_ICON_SIZE} />} onClick={() => onOpen('template')}>
          {t('organise.saveAsTemplate')}
        </MenuEntry>
      )}
      {canEdit && page.kind === 'page' && (
        <MenuEntry icon={<IconFolder size={MENU_ICON_SIZE} />} onClick={() => onOpen('folder')}>
          {t('organise.turnInto_folder')}
        </MenuEntry>
      )}
      {canEdit && page.kind === 'folder' && (
        <MenuEntry icon={<IconPage size={MENU_ICON_SIZE} />} onClick={() => onOpen('page')}>
          {t('organise.turnInto_page')}
        </MenuEntry>
      )}
      {canEdit && !published && page.kind === 'page' && (
        <MenuEntry icon={<IconPage size={MENU_ICON_SIZE} />} onClick={() => onOpen('post')}>
          {t('organise.turnInto_post')}
        </MenuEntry>
      )}
      {canEdit && !published && page.kind === 'post' && (
        <MenuEntry icon={<IconPage size={MENU_ICON_SIZE} />} onClick={() => onOpen('page')}>
          {t('organise.turnInto_page')}
        </MenuEntry>
      )}
      {page.can['page.archive'] && !page.archived_at && (
        <MenuEntry icon={<IconArchive size={MENU_ICON_SIZE} />} onClick={() => onOpen('archive')}>
          {t('organise.archivePage')}
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
          <MenuEntry icon={<IconDownload size={MENU_ICON_SIZE} />} onClick={() => onOpen('export')}>
            {t('exchange.export')}
          </MenuEntry>
          {page.can['space.administer'] && (
            <MenuEntry icon={<IconLink size={MENU_ICON_SIZE} />} onClick={() => onOpen('public-link')}>
              {t('exchange.publicLink')}
            </MenuEntry>
          )}
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

/** Whether you watch a page or its branch, with `w` as the way to change it. */
function usePageWatch(pageId: string) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const state = useQuery({
    queryKey: ['watch', pageId],
    queryFn: () => api.get<WatchState>(`/pages/${pageId}/watch`),
  });
  const set = useMutation({
    mutationFn: (v: { scope: WatchScope; on: boolean }) => {
      const path = `/pages/${pageId}/watch?scope=${v.scope}`;
      return v.on ? api.put(path) : api.del(path);
    },
    onSuccess: (_, v) => toast({ message: v.on ? t('watching.started') : t('watching.stopped') }),
    onError: (e) => toast({ message: e.message }),
    onSettled: () => client.invalidateQueries({ queryKey: ['watch', pageId] }),
  });
  useShortcut('watch', () => set.mutate({ scope: 'page', on: !state.data?.page }));
  return {
    watching: state.data,
    watch: (scope: WatchScope, on: boolean) => set.mutate({ scope, on }),
  };
}

/** What a page does with its threads: highlights, the panel, and `n` and `p`. */
function useThreads(pageId: string, published: boolean) {
  const { t } = useTranslation();
  const settings = useQuery({
    queryKey: ['notification-settings'],
    queryFn: () => api.get<NotificationSettings>('/me/notification-settings'),
  });
  const order = settings.data?.comment_order ?? 'oldest';
  const comments = useQuery({ ...commentsQuery(pageId, order), enabled: published });
  const [found, setFound] = useState<string[]>([]);
  const [panel, setPanel] = useState<{ open: boolean; thread: string | null }>(THREADS_CLOSED);
  const [current, setCurrent] = useState<string | null>(null);
  const openThread = useCallback((id: string | null) => setPanel({ open: true, thread: id }), []);
  const closeThreads = useCallback(() => setPanel(THREADS_CLOSED), []);
  const highlights = useMemo<Highlight[]>(
    () =>
      (comments.data?.inline ?? []).flatMap((c) =>
        c.status === 'open' && !c.removed && c.anchor?.kind === 'text'
          ? [
              {
                id: c.id,
                anchor: c.anchor,
                label: t('comments.highlight', {
                  name: c.author_name || t('common.unknown'),
                  quote: c.anchor.exact,
                }),
              },
            ]
          : [],
      ),
    [comments.data, t],
  );
  const move = (step: 1 | -1) => {
    const data = comments.data;
    if (!data) return;
    const all = [...data.inline, ...data.threads];
    const unread = all.filter((c) => c.unread || c.replies.some((r) => r.unread)).map((c) => c.id);
    const next = nextThread(unread.length > 0 ? unread : highlights.map((h) => h.id), current, step);
    if (!next) return;
    setCurrent(next);
    if (data.inline.some((c) => c.id === next)) {
      jumpToComment(next);
      openThread(next);
    } else {
      jumpToHeading(`comment-${next}`, false);
    }
  };
  useShortcut('nextComment', () => move(1));
  useShortcut('previousComment', () => move(-1));
  return {
    order,
    inline: comments.data?.inline,
    count: (comments.data?.total ?? 0) + (comments.data?.inline.length ?? 0),
    found,
    setFound,
    panel,
    highlights,
    openThread,
    closeThreads,
  };
}

/** An address that points at an inline thread opens it, once. */
function useThreadInAddress(inlineIds: string[], open: (id: string) => void) {
  const handled = useRef('');
  useEffect(() => {
    const follow = () => {
      const id = window.location.hash.replace('#comment-', '');
      if (handled.current === id || !inlineIds.includes(id)) return;
      handled.current = id;
      open(id);
      jumpToComment(id);
    };
    follow();
    window.addEventListener('hashchange', follow);
    return () => window.removeEventListener('hashchange', follow);
  }, [inlineIds, open]);
}

function imageAnchor(image: ShownImage): CommentAnchor | null {
  const id = ATTACHMENT_ADDRESS.exec(image.src)?.[1];
  return id ? { kind: 'image', attachment_id: id } : null;
}

export function PageView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { preferences } = usePreferences();
  const { searchParams } = useLocation();
  const q = useQuery({ queryKey: ['page', pageId], queryFn: () => api.get<PageDetail>(`/pages/${pageId}`) });
  const wantsShare = searchParams.has('share');
  const [dialog, setDialog] = useState<PageDialog | null>(null);
  const [details, setDetails] = useState(false);
  const [presenting, setPresenting] = useState(false);
  const [lightbox, setLightbox] = useState<{ images: ShownImage[]; start: number } | null>(null);
  const [onImage, setOnImage] = useState<CommentAnchor | null>(null);
  const [reader, setReader] = useReaderMode();
  const moveTo = useLocation().searchParams.get('move_to');
  const view = useRef<EditorView | null>(null);
  const keepView = useCallback((next: EditorView) => {
    view.current = next;
  }, []);
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${pageId}/favourite`) : api.del(`/pages/${pageId}/favourite`),
    onSettled: () => client.invalidateQueries({ queryKey: ['page', pageId] }),
  });
  const tick = useMutation({
    mutationFn: (v: { id: string; done: boolean }) => api.post(`/tasks/${v.id}/done`, { done: v.done }),
    onSettled: () => client.invalidateQueries({ queryKey: ['page', pageId] }),
  });
  const page = q.data;
  const { watching, watch } = usePageWatch(pageId);
  const views = useViewCount(pageId, page?.status === 'published');
  const present = usePresence(pageId, 'reading');
  const { openThread, closeThreads, inline, ...threads } = useThreads(pageId, page?.status === 'published');
  const closeDetails = useCallback(() => setDetails(false), []);
  // One panel at a time: both sit in the same place beside the page.
  const openDetails = () => {
    closeThreads();
    setDetails(true);
  };
  const openComments = useCallback(
    (id: string | null) => {
      setDetails(false);
      openThread(id);
    },
    [openThread],
  );
  const inlineIds = useMemo(() => inline?.map((c) => c.id) ?? [], [inline]);
  useThreadInAddress(inlineIds, openComments);
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
  useShortcut('details', () => (details ? setDetails(false) : openDetails()));
  useShortcut('reader', () => setReader(!reader));
  // The address opens Share, for a request in a notification and a warning about a mention.
  useEffect(() => {
    if (wantsShare) setDialog('share');
  }, [wantsShare]);

  if (q.isPending) return <Loading />;
  if (q.error || !page) return <ErrorNote error={q.error} />;
  const close = () => {
    setDialog(null);
    if (wantsShare) navigate(`/pages/${pageId}`, true);
  };
  const favouriteLabel = page.favourite ? t('page.unfavourite') : t('page.favourite');
  const showContents = preferences.contents && headings.length >= MIN_HEADINGS_FOR_CONTENTS;
  const isPublished = page.status === 'published';
  const canComment =
    isPublished && page.can['comment.add'] && page.features.comments && !page.comments_closed;
  return (
    <div className={`page-layout ${showContents ? 'with-contents' : ''} ${page.full_width ? 'full' : ''}`}>
      <article className="page">
        {page.header_attachment_id && <HeaderImage page={page} />}
        <div className="page-top">
          <Breadcrumbs page={page} />
          <div className="page-actions">
            <PresencePeople people={present} />
            {page.can['page.edit'] && (
              <Link className="button" href={`/pages/${page.id}/edit`}>
                <IconPencil size={16} /> {t('page.edit')}
              </Link>
            )}
            <button type="button" onClick={() => setDialog('share')}>
              {t('share.button')}
            </button>
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
              watching={watching}
              onOpen={setDialog}
              onDetails={openDetails}
              onReader={() => setReader(true)}
              onPresent={() => setPresenting(true)}
              onComments={() => openComments(null)}
              onWatch={watch}
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
        <Byline
          page={page}
          comments={threads.count}
          views={views}
          onDetails={openDetails}
          onComments={() => openComments(null)}
        />
        <ErrorNote error={favourite.error} />
        <PageNotes page={page} />
        <ApprovalBanner page={page} onOpen={() => setDialog('approvals')} />
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
        {page.kind === 'link' && (
          <p>
            <a href={page.link_target} target="_blank" rel="noopener noreferrer">
              {page.link_target}
            </a>
          </p>
        )}
        {page.kind !== 'folder' && page.kind !== 'link' && (
          <DocumentView
            document={page.document}
            onImage={(images, start) => setLightbox({ images, start })}
            highlights={threads.highlights}
            onFound={threads.setFound}
            onHighlight={openComments}
            onTask={page.can['page.edit'] ? (id, done) => tick.mutate({ id, done }) : undefined}
            onView={keepView}
          />
        )}
        <ErrorNote error={tick.error} />
        {(!page.hide_children || page.kind === 'folder') && <ChildPages pageId={page.id} />}
        <LabelBar page={page} />
        {isPublished && page.features.reactions && (
          <Reactions
            path={`/pages/${page.id}`}
            queryKey={commentsKey(page.id)}
            readOnly={!page.can['reaction.set']}
          />
        )}
        {isPublished && (page.features.comments || threads.count > 0) && (
          <CommentSection page={page} order={threads.order} />
        )}
        {reader && (
          <button type="button" className="reader-exit" onClick={() => setReader(false)}>
            {t('page.readerExit')}
          </button>
        )}
      </article>
      {showContents && <PageContents headings={headings} />}
      {isPublished && <SelectionBar page={page} view={() => view.current} onCreated={openComments} />}
      {dialog === 'share' && (
        <ShareDialog page={page} onClose={close} onRestrictions={() => setDialog('restrictions')} />
      )}
      {dialog === 'attachments' && <AttachmentsDialog page={page} onClose={close} />}
      {dialog === 'export' && <ExportDialog spaceId={page.space_id} page={page} onClose={close} />}
      {dialog === 'public-link' && <PublicLinkDialog page={page} onClose={close} />}
      {dialog === 'restrictions' && <RestrictionsDialog page={page} onClose={close} />}
      {dialog === 'move' && <MoveDialog page={page} onClose={close} />}
      {dialog === 'rename' && <RenameDialog page={page} onClose={close} />}
      {dialog === 'copy' && <CopyDialog page={page} onClose={close} />}
      {dialog === 'delete' && <DeleteDialog page={page} onClose={close} />}
      {moveTo && (
        <MoveDialog page={page} toSpace={moveTo} onClose={() => navigate(`/pages/${page.id}`, true)} />
      )}
      {dialog === 'approvals' && <ApprovalsDialog page={page} onClose={close} />}
      {dialog === 'verify' && <VerifyDialog page={page} onClose={close} />}
      {dialog === 'archive' && <ArchiveDialog page={page} onClose={close} />}
      {dialog === 'template' && <SaveTemplateDialog page={page} onClose={close} />}
      {(dialog === 'folder' || dialog === 'page' || dialog === 'post') && (
        <KindDialog page={page} kind={dialog} onClose={close} />
      )}
      {details && <DetailsPanel page={page} onClose={closeDetails} />}
      {threads.panel.open && (
        <CommentsPanel
          page={page}
          order={threads.order}
          found={threads.found}
          openId={threads.panel.thread}
          onClose={closeThreads}
        />
      )}
      {lightbox && (
        <Lightbox
          images={lightbox.images}
          start={lightbox.start}
          onClose={() => setLightbox(null)}
          onComment={
            canComment
              ? (image) => {
                  setLightbox(null);
                  setOnImage(imageAnchor(image));
                }
              : undefined
          }
        />
      )}
      {onImage && (
        <NewThread
          page={page}
          anchor={onImage}
          onDone={(created) => {
            setOnImage(null);
            if (created) openComments(created.id);
          }}
        />
      )}
      {presenting && (
        <Presentation title={page.title} document={page.document} onClose={() => setPresenting(false)} />
      )}
    </div>
  );
}
