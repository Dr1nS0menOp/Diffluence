// SPDX-License-Identifier: Apache-2.0
// Recent, Favourites and Spaces open a panel beside the sidebar, so a person picks where to go
// without leaving the screen (spec 078, NAV-131 to NAV-136). Each list loads when its panel opens.
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ListedPage, type Me, type Space } from '../api';
import { useCapabilities } from '../capabilities';
import { pageTitle } from '../page-name';
import { Link, navigate } from '../router';
import { useCurrentSpace } from './current-space';
import { IconApps, IconClose, IconPlus, IconStar, IconStarFilled, IconUpload } from './icons';
import { mySpacesQuery } from './my-spaces';
import type { PanelName } from './sidebar-entry-views';
import { PageMark } from './topbar';
import { PAGE_DRAG_TYPE } from './tree-model';
import { ErrorNote, Loading, SpaceBadge } from './ui';

const ROWS_PER_GROUP = 5;
const FAVOURITE_PAGES_SHOWN = 12;
const ACTION_ICON_SIZE = 18;
const STAR_SIZE = 16;

const matches = (text: string, wanted: string) => text.toLowerCase().includes(wanted.trim().toLowerCase());

interface PanelProps {
  name: PanelName;
  /** The entry that opened it, which gets the focus back. */
  opener: RefObject<HTMLButtonElement | null>;
  onClose: () => void;
}

/** The card that holds a panel: title and Close, the filter, the groups, the actions at the foot. */
function PanelCard({
  name,
  title,
  opener,
  onClose,
  filter,
  onFilter,
  actions,
  children,
}: PanelProps & {
  title: string;
  filter: string;
  onFilter: (text: string) => void;
  actions: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const card = useRef<HTMLDivElement>(null);
  const closeToOpener = () => {
    onClose();
    opener.current?.focus();
  };
  useEffect(() => {
    // A click outside shuts the panel, as it shuts a menu; the focus stays where the click put it.
    const outside = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!card.current?.contains(target) && !opener.current?.contains(target)) onClose();
    };
    document.addEventListener('mousedown', outside);
    return () => document.removeEventListener('mousedown', outside);
  }, [onClose, opener]);
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'Escape') return;
    e.stopPropagation();
    closeToOpener();
  };
  const titleId = `panel-${name}-title`;
  return (
    <div
      ref={card}
      id={`panel-${name}`}
      role="dialog"
      aria-labelledby={titleId}
      className="side-panel"
      onKeyDown={keys}
    >
      <div className="side-panel-head">
        <h2 id={titleId}>{title}</h2>
        <button
          type="button"
          className="icon"
          aria-label={t('common.close')}
          title={t('common.close')}
          onClick={closeToOpener}
        >
          <IconClose size={ACTION_ICON_SIZE} />
        </button>
      </div>
      <input
        type="search"
        className="side-panel-filter"
        value={filter}
        aria-label={t('panel.filter')}
        placeholder={t('panel.filter')}
        // The panel opened on request to choose a row: typing narrows the rows at once (NAV-131).
        autoFocus
        onChange={(e) => onFilter(e.target.value)}
      />
      <div className="side-panel-body">{children}</div>
      <div className="side-panel-actions">{actions}</div>
    </div>
  );
}

/** A group of rows under a small muted heading; an empty group says so. */
function PanelGroup({
  id,
  title,
  pending,
  error,
  empty,
  children,
}: {
  id: string;
  title: string;
  pending?: boolean;
  error?: unknown;
  /** Shown when the group holds nothing. */
  empty: string | null;
  children: ReactNode;
}) {
  return (
    <section aria-labelledby={id} className="side-panel-group">
      <h3 id={id}>{title}</h3>
      {pending && <Loading />}
      <ErrorNote error={error} />
      {empty !== null && <p className="muted">{empty}</p>}
      <ul>{children}</ul>
    </section>
  );
}

function PageRow({ page, href, onChoose }: { page: ListedPage; href: string; onChoose: () => void }) {
  const { t } = useTranslation();
  return (
    <li>
      <Link href={href} className="panel-row" onClick={onChoose}>
        <PageMark page={page} />
        <span className="panel-row-text">
          <span>{pageTitle(t, page.title)}</span>
          <span className="muted">{page.space_name}</span>
        </span>
      </Link>
    </li>
  );
}

function PanelAction({
  href,
  icon,
  label,
  onChoose,
}: {
  href: string;
  icon: ReactNode;
  label: string;
  onChoose: () => void;
}) {
  return (
    <Link href={href} className="panel-row" onClick={onChoose}>
      {icon} {label}
    </Link>
  );
}

const RECENT_GROUPS = ['visited', 'worked', 'drafts'] as const;

/** Pages the person visited, worked on, and their drafts (NAV-136, D7). */
export function RecentPanel(props: PanelProps) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  return (
    <PanelCard
      {...props}
      title={t('nav.recent')}
      filter={filter}
      onFilter={setFilter}
      actions={<PanelAction href="/recent" icon={null} label={t('nav.allRecent')} onChoose={props.onClose} />}
    >
      {RECENT_GROUPS.map((kind) => (
        <RecentGroup key={kind} kind={kind} filter={filter} onChoose={props.onClose} />
      ))}
    </PanelCard>
  );
}

function RecentGroup({
  kind,
  filter,
  onChoose,
}: {
  kind: (typeof RECENT_GROUPS)[number];
  filter: string;
  onChoose: () => void;
}) {
  const { t } = useTranslation();
  // The same list the Recent screen reads, so both stay in step.
  const q = useQuery({
    queryKey: ['recent-list', kind],
    queryFn: () => api.get<{ pages: ListedPage[] }>(`/me/recent?kind=${kind}`),
  });
  const pages = (q.data?.pages ?? [])
    .filter((p) => matches(pageTitle(t, p.title), filter))
    .slice(0, ROWS_PER_GROUP);
  return (
    <PanelGroup
      id={`panel-recent-${kind}`}
      title={t(`recent.tab_${kind}`)}
      pending={q.isPending}
      error={q.error}
      empty={q.data && pages.length === 0 ? t('panel.nothing') : null}
    >
      {pages.map((p) => (
        <PageRow
          key={p.id}
          page={p}
          href={kind === 'drafts' ? `/pages/${p.id}/edit` : `/pages/${p.id}`}
          onChoose={onChoose}
        />
      ))}
    </PanelGroup>
  );
}

/** Favourite pages and spaces (NAV-136). */
export function FavouritesPanel(props: PanelProps) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const pages = useQuery({
    queryKey: ['/me/favourites'],
    queryFn: () => api.get<{ pages: ListedPage[] }>('/me/favourites'),
  });
  const spaces = useQuery(mySpacesQuery);
  const shownPages = (pages.data?.pages ?? [])
    .filter((p) => matches(pageTitle(t, p.title), filter))
    .slice(0, FAVOURITE_PAGES_SHOWN);
  const shownSpaces = (spaces.data?.favourites ?? []).filter((s) => matches(s.name, filter));
  return (
    <PanelCard
      {...props}
      title={t('nav.favourites')}
      filter={filter}
      onFilter={setFilter}
      actions={
        <PanelAction href="/favourites" icon={null} label={t('nav.allFavourites')} onChoose={props.onClose} />
      }
    >
      <PanelGroup
        id="panel-favourites-pages"
        title={t('panel.pages')}
        pending={pages.isPending}
        error={pages.error}
        empty={pages.data && shownPages.length === 0 ? t('nav.noFavourites') : null}
      >
        {shownPages.map((p) => (
          <PageRow key={p.id} page={p} href={`/pages/${p.id}`} onChoose={props.onClose} />
        ))}
      </PanelGroup>
      <PanelGroup
        id="panel-favourites-spaces"
        title={t('nav.spaces')}
        pending={spaces.isPending}
        error={spaces.error}
        empty={spaces.data && shownSpaces.length === 0 ? t('panel.nothing') : null}
      >
        {shownSpaces.map((s) => (
          <SpaceRow key={s.id} space={s} favourite onChoose={props.onClose} />
        ))}
      </PanelGroup>
    </PanelCard>
  );
}

/** A space in a panel: its badge and name, and a star that adds or removes it as a favourite. */
function SpaceRow({
  space,
  favourite,
  onChoose,
}: {
  space: Space;
  favourite: boolean;
  onChoose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const star = useMutation({
    mutationFn: () =>
      favourite ? api.del(`/spaces/${space.id}/favourite`) : api.put(`/spaces/${space.id}/favourite`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
      await client.invalidateQueries({ queryKey: ['space', space.key] });
      await client.invalidateQueries({ queryKey: ['space-id', space.id] });
    },
  });
  const label = favourite
    ? t('panel.unfavouriteSpace', { name: space.name })
    : t('panel.favouriteSpace', { name: space.name });
  return (
    <li
      className={favourite ? 'panel-space favourite' : 'panel-space'}
      // A page dragged onto a space moves there, as it did onto the menu (SPC-044).
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes(PAGE_DRAG_TYPE)) e.preventDefault();
      }}
      onDrop={(e) => {
        const dragged = e.dataTransfer.getData(PAGE_DRAG_TYPE);
        if (!dragged) return;
        e.preventDefault();
        onChoose();
        navigate(`/pages/${dragged}?move_to=${space.id}`);
      }}
    >
      <Link href={`/spaces/${space.key}`} className="panel-row" onClick={onChoose}>
        <SpaceBadge space={space} size="small" />
        <span className="panel-row-text">{space.name}</span>
      </Link>
      <button
        type="button"
        className="icon panel-star"
        aria-label={label}
        title={label}
        disabled={star.isPending}
        onClick={() => star.mutate()}
      >
        {favourite ? <IconStarFilled size={STAR_SIZE} /> : <IconStar size={STAR_SIZE} />}
      </button>
    </li>
  );
}

/** The current space, favourites and recent spaces, and the ways to all spaces (NAV-133, NAV-134). */
export function SpacesPanel({
  me,
  onCreateSpace,
  ...props
}: PanelProps & {
  me: Me;
  /** Shuts the panel and asks for the new space, whose dialog outlives the panel. */
  onCreateSpace: () => void;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const { bundles } = useCapabilities();
  const mine = useQuery(mySpacesQuery);
  const { space: current } = useCurrentSpace();
  const favourites = mine.data?.favourites ?? [];
  const isFavourite = (space: Space) => favourites.some((f) => f.id === space.id);
  const shown = (list: Space[]) => list.filter((s) => s.status !== 'archived' && matches(s.name, filter));
  const recent = shown((mine.data?.recent ?? []).filter((s) => !isFavourite(s)));
  const shownFavourites = shown(favourites);
  const mayCreate = me.global_permissions.includes('create_space');
  const mayImport = bundles && me.global_permissions.includes('administrator');
  return (
    <PanelCard
      {...props}
      title={t('nav.spaces')}
      filter={filter}
      onFilter={setFilter}
      actions={
        <>
          <PanelAction
            href="/spaces"
            icon={<IconApps size={ACTION_ICON_SIZE} />}
            label={t('nav.allSpaces')}
            onChoose={props.onClose}
          />
          {mayCreate && (
            <button type="button" className="panel-row" onClick={onCreateSpace}>
              <IconPlus size={ACTION_ICON_SIZE} /> {t('panel.createSpace')}
            </button>
          )}
          {mayImport && (
            <PanelAction
              href="/admin/export"
              icon={<IconUpload size={ACTION_ICON_SIZE} />}
              label={t('panel.import')}
              onChoose={props.onClose}
            />
          )}
        </>
      }
    >
      {current && matches(current.name, filter) && (
        <PanelGroup id="panel-spaces-current" title={t('panel.currentSpace')} empty={null}>
          <SpaceRow space={current} favourite={isFavourite(current)} onChoose={props.onClose} />
        </PanelGroup>
      )}
      <PanelGroup
        id="panel-spaces-favourites"
        title={t('nav.favourites')}
        pending={mine.isPending}
        error={mine.error}
        empty={mine.data && shownFavourites.length === 0 ? t('panel.noFavouriteSpaces') : null}
      >
        {shownFavourites.map((s) => (
          <SpaceRow key={s.id} space={s} favourite onChoose={props.onClose} />
        ))}
      </PanelGroup>
      <PanelGroup
        id="panel-spaces-recent"
        title={t('panel.recentSpaces')}
        empty={mine.data && recent.length === 0 ? t('panel.nothing') : null}
      >
        {recent.map((s) => (
          <SpaceRow key={s.id} space={s} favourite={false} onChoose={props.onClose} />
        ))}
      </PanelGroup>
    </PanelCard>
  );
}
