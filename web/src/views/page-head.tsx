// SPDX-License-Identifier: Apache-2.0
// The header row and the byline of a page (spec 079): breadcrumbs at the start; the
// publishing state, who else is reading, Edit, Share with Copy link, and More at the end;
// under the title one line of small parts, without a date or a version.
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageDetail, type Space } from '../api';
import { useDates } from '../dates';
import { Link } from '../router';
import { ReactionPicker } from '../components/comments/reactions';
import { commentsKey } from '../components/comments/thread';
import { mayOpenReview } from '../components/control-band';
import { IconEmoji, IconEye, IconLink, IconLock, IconLockOpen, IconPencil } from '../components/icons';
import { Menu, MenuLink } from '../components/menu';
import { StageControl, VerifiedMark } from '../components/page-marks';
import { PresencePeople, type PresentPerson } from '../components/presence';
import { useCopy } from '../components/toast';
import { Avatar, ErrorNote } from '../components/ui';
import { PublicMark } from './exchange';
import { pageTitle } from '../page-name';

const CRUMBS_BEFORE_FOLDING = 4;
const ROW_ICON_SIZE = 16;

export function useSpaceOf(page: PageDetail | undefined) {
  return useQuery({
    queryKey: ['space-id', page?.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page?.space_id}`),
    enabled: !!page,
  });
}

function Crumb({ page }: { page: { id: string; title: string; emoji: string } }) {
  const { t } = useTranslation();
  return (
    <Link href={`/pages/${page.id}`}>
      {page.emoji && <span aria-hidden="true">{page.emoji} </span>}
      {pageTitle(t, page.title)}
    </Link>
  );
}

/** The path to a page. The home page of a space has the space alone, as the place it is (PAG-143). */
function Breadcrumbs({ page, space }: { page: PageDetail; space: Space | undefined }) {
  const { t } = useTranslation();
  const isHome = page.id === space?.home_page_id;
  const crumbs = page.breadcrumbs.filter((c) => c.id !== space?.home_page_id);
  const folded = crumbs.length > CRUMBS_BEFORE_FOLDING ? crumbs.slice(0, -1) : [];
  const shown = folded.length > 0 ? crumbs.slice(-1) : crumbs;
  return (
    <nav aria-label={t('page.breadcrumbs')} className="crumbs">
      <ol>
        {space && (
          <li>
            <Link href={`/spaces/${space.key}`} aria-current={isHome ? 'page' : undefined}>
              {space.name}
            </Link>
          </li>
        )}
        {folded.length > 0 && (
          <li>
            <Menu label={t('page.moreCrumbs')} align="start" className="crumb-fold" trigger="…">
              {folded.map((c) => (
                <MenuLink key={c.id} href={`/pages/${c.id}`}>
                  {pageTitle(t, c.title)}
                </MenuLink>
              ))}
            </Menu>
          </li>
        )}
        {!isHome &&
          shown.map((c) => (
            <li key={c.id}>
              <Crumb page={c} />
            </li>
          ))}
      </ol>
    </nav>
  );
}

/** Says in words whether the page is published, with the date of its last update (PAG-142). */
function PublishState({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const dates = useDates();
  if (page.status === 'draft') return <span className="publish-state">{t('page.notPublished')}</span>;
  const day = dates.day(page.updated_at);
  return (
    <span className="publish-state" data-testid="last-updated" title={t('page.lastUpdated', { date: day })}>
      {t('page.published')} <span className="publish-date">{day}</span>
    </span>
  );
}

/** Share with the state of the lock on it, joined to Copy link as one control (PAG-141, COL-044). */
function ShareControl({ page, onShare }: { page: PageDetail; onShare: () => void }) {
  const { t } = useTranslation();
  const copy = useCopy();
  const lockWords = t(page.restricted ? 'page.restrictionsClosed' : 'page.restrictionsOpen');
  return (
    <div className="share-control" role="group" aria-label={t('share.button')}>
      <button
        type="button"
        className="share-button"
        aria-label={`${t('share.button')}. ${lockWords}`}
        title={lockWords}
        data-restricted={page.restricted}
        onClick={onShare}
      >
        {page.restricted ? <IconLock size={ROW_ICON_SIZE} /> : <IconLockOpen size={ROW_ICON_SIZE} />}
        {t('share.button')}
      </button>
      <button
        type="button"
        className="share-link"
        aria-label={t('tree.copyLink')}
        title={t('tree.copyLink')}
        onClick={() => void copy(`${window.location.origin}/pages/${page.id}`, t('tree.linkCopied'))}
      >
        <IconLink size={ROW_ICON_SIZE} />
      </button>
    </div>
  );
}

/**
 * The first row of a page. Its end stands outside the reading column (PAG-140); the home page
 * of a space has a thin line under it (PAG-164), and a personal home page no publishing state.
 */
export function PageHeaderRow({
  page,
  space,
  present,
  onShare,
  more,
}: {
  page: PageDetail;
  space: Space | undefined;
  present: PresentPerson[];
  onShare: () => void;
  more: ReactNode;
}) {
  const { t } = useTranslation();
  const isHome = page.id === space?.home_page_id;
  const personalHome = isHome && space?.kind === 'personal';
  return (
    <div className={isHome ? 'page-top home' : 'page-top'}>
      <Breadcrumbs page={page} space={space} />
      <div className="page-actions">
        {!personalHome && <PublishState page={page} />}
        <PresencePeople people={present} />
        {page.can['page.edit'] && (
          <Link className="button" href={`/pages/${page.id}/edit`}>
            <IconPencil size={ROW_ICON_SIZE} /> {t('page.edit')}
          </Link>
        )}
        <ShareControl page={page} onShare={onShare} />
        {more}
      </div>
    </div>
  );
}

/** Adds a reaction from the byline, with the same short list and picker as at the bottom (COL-099). */
function AddReaction({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLSpanElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const path = `/pages/${page.id}`;
  const add = useMutation({
    mutationFn: (glyph: string) => api.put(`${path}/reactions/${encodeURIComponent(glyph)}`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['reactions', path] });
      await client.invalidateQueries({ queryKey: commentsKey(page.id) });
    },
  });
  useEffect(() => {
    if (!open) return;
    const outside = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', outside);
    return () => document.removeEventListener('mousedown', outside);
  }, [open]);
  const close = () => {
    setOpen(false);
    button.current?.focus();
  };
  return (
    <span className="byline-react" ref={box}>
      <button
        ref={button}
        type="button"
        className="byline-part byline-author"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <IconEmoji size={ROW_ICON_SIZE} /> {t('reactions.add')}
      </button>
      {open && (
        <ReactionPicker
          onClose={close}
          onPick={(glyph) => {
            close();
            add.mutate(glyph);
          }}
        />
      )}
      <ErrorNote error={add.error} />
    </span>
  );
}
/** The state of a controlled page, which opens the review panel for whoever may open it (PAG-145). */
function ControlState({ page, onReview }: { page: PageDetail; onReview: () => void }) {
  const c = page.control;
  if (!c) return null;
  if (!mayOpenReview(page)) return <span className="byline-part">{c.state_name}</span>;
  return (
    <button type="button" className="byline-part byline-author" onClick={onReview}>
      {c.state_name}
    </button>
  );
}

/**
 * One line of small muted parts: the owner, the view count, adding a reaction, the state of a
 * controlled page, the marks, and the status at the end. No date and no version (PAG-144).
 */
export function Byline({
  page,
  views,
  onDetails,
  onReview,
}: {
  page: PageDetail;
  views: number | null;
  onDetails: () => void;
  onReview: () => void;
}) {
  const { t } = useTranslation();
  const author = page.owner_name || page.created_by_name || t('common.unknown');
  const mayReact =
    page.status === 'published' && page.features.reactions && page.can['reaction.set'] === true;
  return (
    <div className="byline">
      <Avatar name={author} size="small" />
      <button type="button" className="byline-author" onClick={onDetails}>
        {t('page.byline', { name: author })}
      </button>
      {page.owner_disabled && <span className="muted">{t('content.noLongerActive')}</span>}
      {views !== null && (
        <Link className="byline-part" href={`/pages/${page.id}/insights`}>
          <IconEye size={ROW_ICON_SIZE} /> {t('insights.viewCount', { count: views })}
        </Link>
      )}
      {mayReact && <AddReaction page={page} />}
      <ControlState page={page} onReview={onReview} />
      <VerifiedMark page={page} />
      <PublicMark pageId={page.id} />
      <StageControl page={page} />
    </div>
  );
}
