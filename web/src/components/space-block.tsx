// SPDX-License-Identifier: Apache-2.0
// Inside a space the sidebar holds, after the global part, the block of the space: its header
// row, then headed groups for shortcuts, content, blogs and the space's own apps (spec 078,
// SPC-097 to SPC-108, SPC-160).
import { useEffect, useRef, useState, type ComponentType, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type BlogPost, type PageSummary, type Shortcut, type Space } from '../api';
import { useCapabilities } from '../capabilities';
import { mayCreate } from '../create-permission';
import { pageTitle } from '../page-name';
import { Link, navigate, useLocation } from '../router';
import { CopyDialog, MoveDialog } from '../views/move-dialogs';
import { ArchiveDialog, KindDialog, SortChildrenDialog } from '../views/page-actions';
import { DeleteDialog, RenameDialog } from '../views/page-dialogs';
import { ExportDialog } from '../views/exchange';
import { DeleteSpaceDialog } from '../views/space-organise';
import { mayOpenSpaceSettings } from '../views/space-sections';
import { NewDatabaseDialog, NewLinkDialog, NewPageDialog, NewPostDialog } from './create';
import {
  IconApps,
  IconArchive,
  IconArrowUp,
  IconAttach,
  IconAutomation,
  IconBell,
  IconBlog,
  IconDownload,
  IconExternal,
  IconEye,
  IconInfo,
  IconLink,
  IconMore,
  IconPage,
  IconPeople,
  IconPlus,
  IconSheet,
  IconSliders,
  IconStar,
  IconStarFilled,
  IconTrash,
  IconUpload,
} from './icons';
import { Menu, MenuItem, MenuLink, MenuSeparator } from './menu';
import { shortcutsQuery, ShortcutsForm } from './shortcut-links';
import { useCopy } from './toast';
import { PagesSection, type RowDialog } from './tree';
import { ConfirmDialog, Dialog, ErrorNote, SpaceBadge } from './ui';

const MENU_ICON_SIZE = 18;
const ROW_ICON_SIZE = 16;
const POSTS_SHOWN = 5;
const HEAD_LINK_ID = 'space-head-link';

/** Whether you watch a space, and the way to change it. */
export function useSpaceWatch(space: Space) {
  const client = useQueryClient();
  const key = ['space-watch', space.id];
  const state = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ watching: boolean }>(`/spaces/${space.id}/watch`),
  });
  const set = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/spaces/${space.id}/watch`) : api.del(`/spaces/${space.id}/watch`),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  return { watching: state.data?.watching ?? false, set: set.mutate };
}

/** Whether you hear of every approval in a space, the approvals digest (spec 064, DOC-607). */
function useApprovalsDigest(space: Space) {
  const client = useQueryClient();
  const key = ['approvals-digest', space.id];
  const state = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ on: boolean }>(`/spaces/${space.id}/approvals-digest`),
  });
  const set = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/spaces/${space.id}/approvals-digest`) : api.del(`/spaces/${space.id}/approvals-digest`),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  return { on: state.data?.on ?? false, set: set.mutate };
}

/** Changes of a whole space that the menu starts: favourite, archive, restore and delete. */
function useSpaceChanges(space: Space) {
  const client = useQueryClient();
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['space-id', space.id] });
    await client.invalidateQueries({ queryKey: ['space', space.key] });
    await client.invalidateQueries({ queryKey: ['my-spaces'] });
    await client.invalidateQueries({ queryKey: ['spaces'] });
  };
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/spaces/${space.id}/favourite`) : api.del(`/spaces/${space.id}/favourite`),
    onSettled: refresh,
  });
  const archive = useMutation({
    mutationFn: (to: 'archive' | 'unarchive') => api.post(`/spaces/${space.id}/${to}`),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: () => api.del(`/spaces/${space.id}`),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
      navigate('/');
    },
  });
  return { favourite, archive, remove };
}

type SpaceDialog = 'export' | 'archive' | 'delete' | null;

/** The space menu, from the More button of the header row (SPC-107, SPC-108). */
function SpaceMenu({ space }: { space: Space }) {
  const [dialog, setDialog] = useState<SpaceDialog>(null);
  const capabilities = useCapabilities();
  const { automation } = capabilities;
  const { t } = useTranslation();
  const copy = useCopy();
  const watch = useSpaceWatch(space);
  const approvals = useApprovalsDigest(space);
  const { favourite, archive, remove } = useSpaceChanges(space);
  const base = `/spaces/${space.key}`;
  const administers = space.can?.['space.administer'] ?? false;
  const close = () => setDialog(null);
  return (
    <>
      <Menu label={t('space.more')} trigger={<IconMore size={MENU_ICON_SIZE} />} floating>
        <MenuItem onClick={() => favourite.mutate(!space.favourite)}>
          {space.favourite ? <IconStarFilled size={MENU_ICON_SIZE} /> : <IconStar size={MENU_ICON_SIZE} />}{' '}
          {space.favourite ? t('space.unfavourite') : t('space.favourite')}
        </MenuItem>
        <MenuItem onClick={() => watch.set(!watch.watching)}>
          <IconEye size={MENU_ICON_SIZE} />{' '}
          {watch.watching ? t('watching.stopSpace') : t('watching.watchSpace')}
        </MenuItem>
        <MenuItem onClick={() => approvals.set(!approvals.on)}>
          <IconBell size={MENU_ICON_SIZE} />{' '}
          {approvals.on ? t('watching.stopApprovals') : t('watching.approvals')}
        </MenuItem>
        <MenuItem onClick={() => void copy(`${window.location.origin}${base}`, t('tree.linkCopied'))}>
          <IconLink size={MENU_ICON_SIZE} /> {t('tree.copyLink')}
        </MenuItem>
        <MenuSeparator />
        <div role="group" aria-labelledby="space-tools-heading">
          <div role="presentation" id="space-tools-heading" className="menu-group-title">
            {t('space.tools')}
          </div>
          <MenuLink href={`${base}/overview`}>
            <IconInfo size={MENU_ICON_SIZE} /> {t('space.overview')}
          </MenuLink>
          <MenuLink
            href={space.can?.['space.manage_access'] ? `${base}/settings?section=people` : `${base}/people`}
          >
            <IconPeople size={MENU_ICON_SIZE} /> {t('access.peopleTitle')}
          </MenuLink>
          <MenuLink href={`${base}/files`}>
            <IconAttach size={MENU_ICON_SIZE} /> {t('files.title')}
          </MenuLink>
          <MenuLink href={`${base}/archive`}>
            <IconArchive size={MENU_ICON_SIZE} /> {t('space.archivedPages')}
          </MenuLink>
        </div>
        <MenuSeparator />
        {space.can?.['space.export'] && (
          <MenuItem onClick={() => setDialog('export')}>
            <IconDownload size={MENU_ICON_SIZE} /> {t('exchange.export')}
          </MenuItem>
        )}
        {automation && space.can?.['space.automation'] && (
          <MenuLink href={`${base}/settings/automation`}>
            <IconAutomation size={MENU_ICON_SIZE} /> {t('admin.automation')}
          </MenuLink>
        )}
        {mayOpenSpaceSettings(space, capabilities) && (
          <MenuLink href={`${base}/settings`}>
            <IconSliders size={MENU_ICON_SIZE} /> {t('space.settings')}
          </MenuLink>
        )}
        {administers && <MenuSeparator />}
        {administers &&
          (space.status === 'archived' ? (
            <MenuItem onClick={() => archive.mutate('unarchive')}>
              <IconUpload size={MENU_ICON_SIZE} /> {t('space.restoreSpace')}
            </MenuItem>
          ) : (
            <MenuItem onClick={() => setDialog('archive')}>
              <IconArchive size={MENU_ICON_SIZE} /> {t('space.archiveSpace')}
            </MenuItem>
          ))}
        {administers && (
          <MenuItem className="danger" onClick={() => setDialog('delete')}>
            <IconTrash size={MENU_ICON_SIZE} /> {t('space.deleteSpace')}
          </MenuItem>
        )}
      </Menu>
      <ErrorNote error={favourite.error ?? archive.error ?? remove.error} />
      {dialog === 'export' && <ExportDialog spaceId={space.id} onClose={close} />}
      {dialog === 'archive' && (
        <ConfirmDialog
          title={t('space.archiveSpace')}
          message={t('space.confirmArchive')}
          confirmLabel={t('space.archiveSpace')}
          tone="primary"
          onConfirm={() => archive.mutate('archive')}
          onClose={close}
        />
      )}
      {dialog === 'delete' && (
        <DeleteSpaceDialog space={space} onDelete={() => remove.mutate()} onClose={close} />
      )}
    </>
  );
}

/** A group of the block: a heading of an icon and a word, its buttons at the end, its rows below. */
function SideGroup({
  id,
  icon: Icon,
  title,
  href,
  current,
  tools,
  children,
}: {
  id: string;
  icon: ComponentType<{ size?: number }>;
  title: string;
  /** A heading that leads to a screen, such as the list of posts. */
  href?: string;
  current?: boolean;
  tools?: ReactNode;
  children: ReactNode;
}) {
  const headingId = `${id}-heading`;
  return (
    <section className="side-group" aria-labelledby={headingId}>
      <div className="side-group-head">
        <h2 id={headingId} className="side-group-title">
          {href ? (
            <Link href={href} aria-current={current ? 'page' : undefined}>
              <Icon size={ROW_ICON_SIZE} /> {title}
            </Link>
          ) : (
            <>
              <Icon size={ROW_ICON_SIZE} /> {title}
            </>
          )}
        </h2>
        {tools && <span className="side-group-tools">{tools}</span>}
      </div>
      <div className="side-group-rows">{children}</div>
    </section>
  );
}

function ShortcutRow({ shortcut }: { shortcut: Shortcut }) {
  const { t } = useTranslation();
  if (shortcut.target.startsWith('/')) {
    return (
      <Link href={shortcut.target} className="side-link">
        <IconLink size={ROW_ICON_SIZE} /> <span className="side-link-text">{shortcut.label}</span>
      </Link>
    );
  }
  return (
    <a href={shortcut.target} className="side-link" target="_blank" rel="noopener noreferrer">
      <IconExternal size={ROW_ICON_SIZE} /> <span className="side-link-text">{shortcut.label}</span>
      <span className="visually-hidden">{t('space.opensNewTab')}</span>
    </a>
  );
}

/** Shortcuts, shown when there are any or the person may add one (SPC-100, D11). */
function ShortcutsGroup({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  const q = useQuery(shortcutsQuery(space));
  const list = q.data?.shortcuts ?? [];
  const mayChange = space.can?.['space.look'] ?? false;
  if (list.length === 0 && !mayChange) return <ErrorNote error={q.error} />;
  return (
    <SideGroup
      id="shortcuts"
      icon={IconLink}
      title={t('shortcutLinks.title')}
      tools={
        mayChange && (
          <button
            type="button"
            className="icon"
            aria-label={t('space.addShortcut')}
            title={t('space.addShortcut')}
            onClick={() => setAdding(true)}
          >
            <IconPlus size={ROW_ICON_SIZE} />
          </button>
        )
      }
    >
      <ErrorNote error={q.error} />
      {q.data && list.length === 0 && <p className="muted side-empty">{t('space.noShortcuts')}</p>}
      {list.map((s) => (
        <ShortcutRow key={s.id ?? s.label} shortcut={s} />
      ))}
      {adding && (
        <Dialog title={t('shortcutLinks.title')} onClose={() => setAdding(false)}>
          <ShortcutsForm space={space} />
        </Dialog>
      )}
    </SideGroup>
  );
}

/** Blogs: the heading leads to the list, the newest posts follow (SPC-103, D12). */
function BlogsGroup({ space }: { space: Space }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const [writing, setWriting] = useState(false);
  const path = `/spaces/${space.id}/posts`;
  const q = useQuery({
    queryKey: ['posts', path, 'newest'],
    queryFn: () => api.get<{ posts: BlogPost[] }>(`${path}?limit=${POSTS_SHOWN}&offset=0`),
  });
  const posts = q.data?.posts ?? [];
  const list = `/spaces/${space.key}/blog`;
  return (
    <SideGroup
      id="blogs"
      icon={IconBlog}
      title={t('space.blogs')}
      href={list}
      current={pathname === list}
      tools={
        mayCreate(space.can, 'post') && (
          <button
            type="button"
            className="icon"
            aria-label={t('blog.newPost')}
            title={t('blog.newPost')}
            onClick={() => setWriting(true)}
          >
            <IconPlus size={ROW_ICON_SIZE} />
          </button>
        )
      }
    >
      <ErrorNote error={q.error} />
      {q.data && posts.length === 0 && <p className="muted side-empty">{t('blog.none')}</p>}
      {posts.map((post) => (
        <Link key={post.id} href={`/pages/${post.id}`} className="side-link">
          <IconBlog size={ROW_ICON_SIZE} /> <span className="side-link-text">{pageTitle(t, post.title)}</span>
        </Link>
      ))}
      {writing && <NewPostDialog space={space} onClose={() => setWriting(false)} />}
    </SideGroup>
  );
}

/** The space's own optional parts, such as releasing a draft space (SPC-160). */
function SpaceAppsGroup({ space }: { space: Space }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { publishing } = useCapabilities();
  const release = `/spaces/${space.key}/release`;
  const releases = publishing && space.pair_role === 'draft' && (space.can?.['space.release'] ?? false);
  if (!releases) return null;
  return (
    <SideGroup id="space-apps" icon={IconApps} title={t('space.apps')}>
      <Link href={release} className="side-link" aria-current={pathname === release ? 'page' : undefined}>
        <IconUpload size={ROW_ICON_SIZE} />{' '}
        <span className="side-link-text">{t('publishing.screenLink')}</span>
      </Link>
    </SideGroup>
  );
}

function RowDialogs({
  open,
  onClose,
}: {
  open: { page: PageSummary; dialog: RowDialog };
  onClose: () => void;
}) {
  const { page, dialog } = open;
  if (dialog === 'rename') return <RenameDialog page={page} onClose={onClose} />;
  if (dialog === 'move') return <MoveDialog page={page} onClose={onClose} />;
  if (dialog === 'copy') return <CopyDialog page={page} onClose={onClose} />;
  if (dialog === 'archive') return <ArchiveDialog page={page} onClose={onClose} />;
  if (dialog === 'sort') return <SortChildrenDialog page={page} onClose={onClose} />;
  if (dialog === 'folder' || dialog === 'page') {
    return <KindDialog page={page} kind={dialog} onClose={onClose} />;
  }
  return <DeleteDialog page={page} onClose={onClose} />;
}

type Making = 'page' | 'database' | 'link' | null;

/** Content: what can be made at the top of the tree, and the tree itself (SPC-101, D13). */
function ContentGroup({ space, current, parent }: { space: Space; current?: string; parent: string | null }) {
  const { t } = useTranslation();
  const { databases, publishing } = useCapabilities();
  const [making, setMaking] = useState<Making>(null);
  const [adding, setAdding] = useState<string | null>(null);
  const [rowDialog, setRowDialog] = useState<{ page: PageSummary; dialog: RowDialog } | null>(null);
  const home = space.home_page_id;
  const canCreate = mayCreate(space.can, 'page');
  const canMakeDatabase = databases && mayCreate(space.can, 'database');
  const under = parent ?? home;
  const close = () => setMaking(null);
  const createItems =
    canCreate || canMakeDatabase ? (
      <>
        {canCreate && (
          <MenuItem onClick={() => setMaking('page')}>
            <IconPage size={MENU_ICON_SIZE} /> {t('space.newPage')}
          </MenuItem>
        )}
        {canMakeDatabase && (
          <MenuItem onClick={() => setMaking('database')}>
            <IconSheet size={MENU_ICON_SIZE} /> {t('sheet.new')}
          </MenuItem>
        )}
        {canCreate && (
          <MenuItem onClick={() => setMaking('link')}>
            <IconLink size={MENU_ICON_SIZE} /> {t('tree.addLink')}
          </MenuItem>
        )}
      </>
    ) : undefined;
  return (
    <>
      <PagesSection
        key={space.id}
        spaceId={space.id}
        home={home}
        current={current === home ? undefined : current}
        canMove={canCreate}
        onAdd={canCreate ? (id) => setAdding(id) : undefined}
        createItems={createItems}
        onDialog={(page, dialog) => setRowDialog({ page, dialog })}
        draftSpace={publishing && space.pair_role === 'draft'}
      />
      {making === 'page' && <NewPageDialog space={space} parent={under} onClose={close} />}
      {making === 'database' && <NewDatabaseDialog space={space} parent={under} onClose={close} />}
      {making === 'link' && <NewLinkDialog space={space} parent={under} onClose={close} />}
      {adding && <NewPageDialog space={space} parent={adding} onClose={() => setAdding(null)} />}
      {rowDialog && <RowDialogs open={rowDialog} onClose={() => setRowDialog(null)} />}
    </>
  );
}

/**
 * Once the sidebar has scrolled, a control floats over its top that brings it back and puts the
 * focus on the space's header row (SPC-102).
 */
function BackToTop() {
  const { t } = useTranslation();
  const anchor = useRef<HTMLSpanElement>(null);
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const sidebar = anchor.current?.closest('.sidebar');
    if (!sidebar) return;
    const follow = () => setScrolled(sidebar.scrollTop > 0);
    sidebar.addEventListener('scroll', follow, { passive: true });
    return () => sidebar.removeEventListener('scroll', follow);
  }, []);
  const toTop = () => {
    const sidebar = anchor.current?.closest('.sidebar');
    const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    sidebar?.scrollTo({ top: 0, behavior: still ? 'auto' : 'smooth' });
    document.getElementById(HEAD_LINK_ID)?.focus({ preventScroll: true });
  };
  return (
    <span ref={anchor} className="back-to-top-place">
      {scrolled && (
        <button type="button" className="back-to-top" onClick={toTop}>
          <IconArrowUp size={ROW_ICON_SIZE} /> {t('space.backToTop')}
        </button>
      )}
    </span>
  );
}

/** The block of a space in the sidebar, after the global part. */
export function SpaceBlock({
  space,
  current,
  parent,
}: {
  space: Space;
  current?: string;
  parent: string | null;
}) {
  const home = space.home_page_id;
  const blogOn = space.features?.blog ?? true;
  return (
    <div className="space-block">
      <BackToTop />
      <div className="space-head">
        <Link
          id={HEAD_LINK_ID}
          href={home ? `/pages/${home}` : `/spaces/${space.key}`}
          className="side-link space-head-link"
          aria-current={home && current === home ? 'page' : undefined}
        >
          <SpaceBadge space={space} size="small" />
          <span className="side-link-text">{space.name}</span>
        </Link>
        <SpaceMenu space={space} />
      </div>
      <ShortcutsGroup space={space} />
      <ContentGroup space={space} current={current} parent={parent} />
      {blogOn && <BlogsGroup space={space} />}
      <SpaceAppsGroup space={space} />
    </div>
  );
}
