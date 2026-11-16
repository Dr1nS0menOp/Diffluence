// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { EditorView } from '@tiptap/pm/view';
import { api, type CommentAnchor, type NotificationSettings, type PageDetail } from '../api';
import { Link, navigate, useLocation } from '../router';
import { usePreferences } from '../preferences';
import type { Highlight } from '../components/comments/anchor';
import { Reactions } from '../components/comments/reactions';
import { CommentSection, CommentsPanel, commentsQuery } from '../components/comments/section';
import { NewThread, SelectionBar } from '../components/comments/selection-bar';
import { commentsKey } from '../components/comments/thread';
import { DocumentView } from '../components/editor/editor';
import { contentsOf, jumpToComment, jumpToHeading, nextThread } from '../components/editor/outline';
import { LabelBar } from '../components/page-labels';
import { ControlBand, mayOpenReview } from '../components/control-band';
import { ReviewPanel } from '../components/review-panel';
import { ReleaseBand, ReleaseDialogs, useReleaseStatus } from '../components/release-band';
import { ApprovalBanner, ApprovalsDialog, PageNotes, VerifyDialog } from '../components/page-marks';
import { MIN_HEADINGS_FOR_CONTENTS, PageContents } from '../components/page-contents';
import { ChildPages, DetailsPanel } from '../components/page-details';
import { SheetReader } from '../components/sheet/editor';
import { ShareDialog } from '../components/share-dialog';
import { useShortcut } from '../components/shortcuts';
import { useToast } from '../components/toast';
import { ErrorNote, Loading } from '../components/ui';
import { Lightbox, Presentation, type ShownImage } from '../components/viewers';
import { useDocumentTitle, useScrollMemory } from '../page-title';
import { ExportDialog, PublicLinkDialog } from './exchange';
import { CopyDialog, MoveDialog } from './move-dialogs';
import { ArchiveDialog, KindDialog, SaveTemplateDialog } from './page-actions';
import { usePresence, useViewCount } from '../components/presence';
import { AttachmentsDialog, DeleteDialog, RenameDialog, RestrictionsDialog } from './page-dialogs';
import { Byline, PageHeaderRow, useSpaceOf } from './page-head';
import { PageMenu, usePageWatch, type PageDialog } from './page-menu';
import { pageTitle } from '../page-name';
// Frames to wait for an address to appear: comments arrive after the document.
const JUMP_ATTEMPTS = 120;
const ATTACHMENT_ADDRESS = /\/attachments\/([0-9a-f-]+)\//;
const THREADS_CLOSED = { open: false, thread: null };

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
  const [exportFormat, setExportFormat] = useState('');
  const [details, setDetails] = useState(false);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [presenting, setPresenting] = useState(false);
  const [lightbox, setLightbox] = useState<{ images: ShownImage[]; start: number } | null>(null);
  const [onImage, setOnImage] = useState<CommentAnchor | null>(null);
  const [reader, setReader] = useReaderMode();
  const moveTo = useLocation().searchParams.get('move_to');
  const view = useRef<EditorView | null>(null);
  const keepView = useCallback((next: EditorView) => {
    view.current = next;
  }, []);
  const toast = useToast();
  // The star left the header row (PAG-141), so the change says itself in a short message.
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${pageId}/favourite`) : api.del(`/pages/${pageId}/favourite`),
    // The message waits for the page to be read again, so a second press acts on the new state.
    onSettled: async (_, error, on) => {
      await client.invalidateQueries({ queryKey: ['page', pageId] });
      if (!error) toast({ message: on ? t('page.favouriteAdded') : t('page.favouriteRemoved') });
    },
  });
  const tick = useMutation({
    mutationFn: (v: { id: string; done: boolean }) => api.post(`/tasks/${v.id}/done`, { done: v.done }),
    onSettled: () => client.invalidateQueries({ queryKey: ['page', pageId] }),
  });
  const page = q.data;
  const pageSpace = useSpaceOf(page);
  const release = useReleaseStatus(page, pageSpace.data?.pair_role);
  const { watching, watch } = usePageWatch(pageId, page?.space_id);
  const views = useViewCount(pageId, page?.status === 'published');
  const present = usePresence(pageId, 'reading');
  const { openThread, closeThreads, inline, ...threads } = useThreads(pageId, page?.status === 'published');
  const closeDetails = useCallback(() => setDetails(false), []);
  const closeReview = useCallback(() => setReviewOpen(false), []);
  // One panel at a time: they all sit in the same place beside the page.
  const openDetails = () => {
    closeThreads();
    setReviewOpen(false);
    setDetails(true);
  };
  const openReview = () => {
    closeThreads();
    setDetails(false);
    setReviewOpen(true);
  };
  const openComments = useCallback(
    (id: string | null) => {
      setDetails(false);
      setReviewOpen(false);
      openThread(id);
    },
    [openThread],
  );
  const inlineIds = useMemo(() => inline?.map((c) => c.id) ?? [], [inline]);
  useThreadInAddress(inlineIds, openComments);
  const headings = useMemo(() => (page ? contentsOf(page.document) : []), [page]);

  useDocumentTitle(page ? `${page.emoji ? `${page.emoji} ` : ''}${pageTitle(t, page.title)}` : undefined);
  useJumpToAddress(pageId, !!page);
  useScrollMemory(!!page);
  useShortcut('edit', () => {
    if (page?.can['page.edit']) navigate(`/pages/${pageId}/edit`);
  });
  useShortcut('favourite', () => {
    if (page && !favourite.isPending) favourite.mutate(!page.favourite);
  });
  useShortcut('details', () => (details ? setDetails(false) : openDetails()));
  useShortcut('reader', () => setReader(!reader));
  useShortcut('review', () => {
    if (!page || !mayOpenReview(page)) return;
    if (reviewOpen) setReviewOpen(false);
    else openReview();
  });
  // The address opens Share, for a request in a notification and a warning about a mention.
  useEffect(() => {
    if (wantsShare) setDialog('share');
  }, [wantsShare]);

  if (q.isPending) return <Loading />;
  if (q.error || !page) return <ErrorNote error={q.error} />;
  if (page.review_only) return <ReviewOnlyPage page={page} />;
  const close = () => {
    setDialog(null);
    if (wantsShare) navigate(`/pages/${pageId}`, true);
  };
  // A personal space's home page is its owner's own place: no byline (PAG-146).
  const personalHome = page.id === pageSpace.data?.home_page_id && pageSpace.data?.kind === 'personal';
  const showContents = preferences.contents && headings.length >= MIN_HEADINGS_FOR_CONTENTS;
  const isPublished = page.status === 'published';
  const canComment =
    isPublished && page.can['comment.add'] && page.features.comments && !page.comments_closed;
  return (
    <div className={`page-layout ${showContents ? 'with-contents' : ''} ${page.full_width ? 'full' : ''}`}>
      <PageHeaderRow
        page={page}
        space={pageSpace.data}
        present={present}
        onShare={() => setDialog('share')}
        more={
          <PageMenu
            page={page}
            watching={watching}
            release={release.data}
            onOpen={setDialog}
            onExport={(format) => {
              setExportFormat(format);
              setDialog('export');
            }}
            onDetails={openDetails}
            onReader={() => setReader(true)}
            onPresent={() => setPresenting(true)}
            onComments={() => openComments(null)}
            onWatch={watch}
            onFavourite={() => favourite.mutate(!page.favourite)}
          />
        }
      />
      <article className="page">
        {page.header_attachment_id && <HeaderImage page={page} />}
        <h1 className={page.title_align === 'center' ? 'title-center' : undefined}>
          {page.emoji && (
            <span className="title-emoji" aria-hidden="true">
              {page.emoji}{' '}
            </span>
          )}
          {pageTitle(t, page.title)}
        </h1>
        {!personalHome && <Byline page={page} views={views} onDetails={openDetails} onReview={openReview} />}
        <ErrorNote error={favourite.error} />
        <PageNotes page={page} />
        <ApprovalBanner page={page} onOpen={() => setDialog('approvals')} />
        {page.control ? (
          <ControlBand page={page} onReview={openReview}>
            <ReleaseBand status={release.data} pageId={page.id} inBand />
          </ControlBand>
        ) : (
          <ReleaseBand status={release.data} pageId={page.id} />
        )}
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
        {page.kind === 'database' && (
          <SheetReader
            document={page.document}
            title={page.title}
            canExport={page.can['space.export'] ?? false}
          />
        )}
        {page.kind !== 'folder' && page.kind !== 'link' && page.kind !== 'database' && (
          <DocumentView
            document={page.document}
            language={page.language || undefined}
            onImage={(images, start) => setLightbox({ images, start })}
            highlights={threads.highlights}
            onFound={threads.setFound}
            onHighlight={openComments}
            onTask={page.can['page.edit'] ? (id, done) => tick.mutate({ id, done }) : undefined}
            onView={keepView}
            checkLinks
            pageId={page.id}
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
        <ShareDialog
          page={page}
          onClose={close}
          onRestrictions={() => setDialog('restrictions')}
          onSharedPage={
            page.can['space.export'] && page.can['site.publish'] && page.current_version > 0
              ? () => setDialog('public-link')
              : undefined
          }
        />
      )}
      {dialog === 'attachments' && <AttachmentsDialog page={page} onClose={close} />}
      {dialog === 'export' && (
        <ExportDialog spaceId={page.space_id} page={page} startFormat={exportFormat} onClose={close} />
      )}
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
      <ReleaseDialogs dialog={dialog} page={page} status={release.data} onClose={close} />
      {dialog === 'archive' && <ArchiveDialog page={page} onClose={close} />}
      {dialog === 'template' && <SaveTemplateDialog page={page} onClose={close} />}
      {(dialog === 'folder' || dialog === 'page' || dialog === 'post') && (
        <KindDialog page={page} kind={dialog} onClose={close} />
      )}
      {details && <DetailsPanel page={page} onClose={closeDetails} />}
      {reviewOpen && <ReviewPanel page={page} onClose={closeReview} />}
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
        <Presentation
          title={pageTitle(t, page.title)}
          document={page.document}
          onClose={() => setPresenting(false)}
        />
      )}
    </div>
  );
}

/**
 * A page read only because its reader is asked to review it (DOC-203): the page, its band and
 * its review, and nothing around it, which the reviewer may not see.
 */
function ReviewOnlyPage({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const [reviewOpen, setReviewOpen] = useState(true);
  const closeReview = useCallback(() => setReviewOpen(false), []);
  useShortcut('review', () => setReviewOpen(!reviewOpen));
  return (
    <div className="page-layout">
      <article className="page">
        <h1>{pageTitle(t, page.title)}</h1>
        <p role="status" className="note">
          {t('review.onlyForReview')}
        </p>
        <ControlBand page={page} onReview={() => setReviewOpen(true)} />
        {page.kind !== 'folder' && page.kind !== 'link' && page.kind !== 'database' && (
          <DocumentView document={page.document} language={page.language || undefined} pageId={page.id} />
        )}
      </article>
      {reviewOpen && <ReviewPanel page={page} onClose={closeReview} />}
    </div>
  );
}
