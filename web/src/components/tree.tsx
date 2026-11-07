// SPDX-License-Identifier: Apache-2.0
// The page tree. A nav of links with the keys of the tree pattern on top: a real
// tree item may not hold buttons, and our rows do (spec 009, D1).
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Page, type PageSummary, type TreeFound, type TreeNode } from '../api';
import { BulkMoveDialog } from '../views/move-dialogs';
import { Link } from '../router';
import { usePreferences } from '../preferences';
import {
  IconArchive,
  IconChevronDown,
  IconChevronRight,
  IconCollapse,
  IconCopy,
  IconExternal,
  IconEye,
  IconFolder,
  IconSheet,
  IconLink,
  IconLocate,
  IconLock,
  IconMore,
  IconMove,
  IconPage,
  IconPlus,
  IconRename,
  IconSortDown,
  IconStar,
  IconTask,
  IconTrash,
} from './icons';
import { Menu, MenuDownload, MenuItem, MenuSeparator } from './menu';
import { personalKey, usePerson } from '../personal';
import { useCopy, useToast } from './toast';
import {
  ancestorsOf,
  buildTree,
  collapseBranch,
  collapseEverything,
  matchParts,
  openBranch,
  PAGE_DRAG_TYPE,
  rangeOfRows,
  readBranchMemory,
  revealPage,
  showsBranch,
  toggleRow,
  typeAheadRow,
  type Branch,
  type BranchMemory,
  type TreeView,
} from './tree-model';
import { useDelayed } from './quick-search';
import { useTreeParts, type MoreRows } from './tree-data';
import { ErrorNote, Loading, RelativeTime } from './ui';

const ROW_ICON_SIZE = 16;
const MARK_ICON_SIZE = 14;
const MENU_ICON_SIZE = 18;
const TREE_VIEWS: TreeView[] = ['tree', 'visited', 'updated', 'alphabet'];
const OPEN_WHILE_DRAGGING_MS = 700;
const CARD_DELAY_MS = 600;
const CARD_LEAVE_MS = 200;
const TYPE_AHEAD_MS = 700;
const SCROLL_EDGE_PX = 40;
const SCROLL_STEP_PX = 12;

export type RowDialog = 'rename' | 'move' | 'copy' | 'delete' | 'archive' | 'sort' | 'folder' | 'page';

/** Collapsed branches stay collapsed while the reader moves through the space. */
function useBranchMemory(spaceId: string): [BranchMemory, (next: BranchMemory) => void] {
  const key = personalKey(usePerson(), `tree.${spaceId}`);
  const [memory, setMemory] = useState(() => readBranchMemory(localStorage.getItem(key)));
  useEffect(() => localStorage.setItem(key, JSON.stringify(memory)), [key, memory]);
  return [memory, setMemory];
}

function RowActions({
  node,
  onDialog,
}: {
  node: TreeNode;
  onDialog: (page: PageSummary, dialog: RowDialog) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const copy = useCopy();
  const facts = useQuery({
    queryKey: ['page-facts', node.id],
    queryFn: () => api.get<PageSummary & { favourite: boolean }>(`/pages/${node.id}/summary`),
  });
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${node.id}/favourite`) : api.del(`/pages/${node.id}/favourite`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['page', node.id] });
      await client.invalidateQueries({ queryKey: ['page-facts', node.id] });
    },
  });
  const watching = useQuery({
    queryKey: ['watch', node.id],
    queryFn: () => api.get<{ page: boolean }>(`/pages/${node.id}/watch`),
  });
  const watch = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${node.id}/watch`) : api.del(`/pages/${node.id}/watch`),
    onSettled: () => client.invalidateQueries({ queryKey: ['watch', node.id] }),
  });
  const page = facts.data;
  const published = page?.status === 'published';
  return (
    <>
      <MenuDownload href={`/pages/${node.id}`} target="_blank" rel="noopener">
        <IconExternal size={MENU_ICON_SIZE} /> {t('tree.openNewTab')}
      </MenuDownload>
      <MenuItem onClick={() => void copy(`${window.location.origin}/pages/${node.id}`, t('tree.linkCopied'))}>
        <IconLink size={MENU_ICON_SIZE} /> {t('tree.copyLink')}
      </MenuItem>
      {facts.isPending && <Loading />}
      <ErrorNote error={facts.error} />
      {page && (
        <>
          <MenuItem onClick={() => favourite.mutate(!page.favourite)}>
            <IconStar size={MENU_ICON_SIZE} /> {page.favourite ? t('page.unfavourite') : t('page.favourite')}
          </MenuItem>
          <MenuItem onClick={() => watch.mutate(!watching.data?.page)}>
            <IconEye size={MENU_ICON_SIZE} />{' '}
            {watching.data?.page ? t('watching.stopPage') : t('watching.watchPage')}
          </MenuItem>
          {page.can['page.edit'] && (
            <>
              <MenuItem onClick={() => onDialog(page, 'rename')}>
                <IconRename size={MENU_ICON_SIZE} /> {t('page.rename')}
              </MenuItem>
              <MenuItem onClick={() => onDialog(page, 'move')}>
                <IconMove size={MENU_ICON_SIZE} /> {t('page.move')}
              </MenuItem>
            </>
          )}
          {page.can['page.create'] && published && (
            <MenuItem onClick={() => onDialog(page, 'copy')}>
              <IconCopy size={MENU_ICON_SIZE} /> {t('page.copy')}
            </MenuItem>
          )}
          {page.can['page.edit'] && page.kind === 'page' && (
            <MenuItem onClick={() => onDialog(page, 'folder')}>
              <IconFolder size={MENU_ICON_SIZE} /> {t('organise.turnInto_folder')}
            </MenuItem>
          )}
          {page.can['page.edit'] && page.kind === 'folder' && (
            <MenuItem onClick={() => onDialog(page, 'page')}>
              <IconPage size={MENU_ICON_SIZE} /> {t('organise.turnInto_page')}
            </MenuItem>
          )}
          {page.can['page.edit'] && (
            <MenuItem onClick={() => onDialog(page, 'sort')}>
              <IconSortDown size={MENU_ICON_SIZE} /> {t('organise.sortLevel')}
            </MenuItem>
          )}
          {page.can['page.archive'] && (
            <MenuItem onClick={() => onDialog(page, 'archive')}>
              <IconArchive size={MENU_ICON_SIZE} /> {t('organise.archivePage')}
            </MenuItem>
          )}
          {page.can['page.delete'] && (
            <>
              <MenuSeparator />
              <MenuItem className="danger" onClick={() => onDialog(page, 'delete')}>
                <IconTrash size={MENU_ICON_SIZE} /> {t('page.delete')}
              </MenuItem>
            </>
          )}
        </>
      )}
    </>
  );
}

function RowTitle({ node, filter }: { node: TreeNode; filter: string }) {
  return (
    <>
      {matchParts(node.title, filter).map((part, i) =>
        part.hit ? <mark key={i}>{part.text}</mark> : <span key={i}>{part.text}</span>,
      )}
    </>
  );
}

function RowMarks({ node }: { node: TreeNode }) {
  const { t } = useTranslation();
  return (
    <>
      {node.status === 'draft' && <span className="visually-hidden">{t('tree.unpublished')}</span>}
      {node.kind === 'link' && (
        <span className="node-mark" title={t('space.opensNewTab')}>
          <IconExternal size={MARK_ICON_SIZE} />
          <span className="visually-hidden">{t('space.opensNewTab')}</span>
        </span>
      )}
      {node.restricted && (
        <span className="node-mark" title={t('page.restricted')}>
          <IconLock size={MARK_ICON_SIZE} />
          <span className="visually-hidden">{t('page.restricted')}</span>
        </span>
      )}
      {node.has_draft && (
        <span className="node-mark" title={t('tree.hasDraft')}>
          <span className="node-dot" />
          <span className="visually-hidden">{t('tree.hasDraft')}</span>
        </span>
      )}
    </>
  );
}

/** Follows WCAG 1.4.13: it can be dismissed, hovered, and stays until the pointer leaves. */
function useRowCard(enabled: boolean) {
  const [card, setCard] = useState<{ node: TreeNode; top: number; left: number } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => {
    if (!card) return;
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape') setCard(null);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [card]);
  useEffect(() => () => clearTimeout(timer.current), []);

  const show = (node: TreeNode, row: HTMLElement) => {
    if (!enabled) return;
    clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      const box = row.getBoundingClientRect();
      setCard({ node, top: box.top, left: box.right });
    }, CARD_DELAY_MS);
  };
  const hide = () => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCard(null), CARD_LEAVE_MS);
  };
  const keep = () => clearTimeout(timer.current);
  return { card, show, hide, keep };
}

function RowCard({
  node,
  top,
  left,
  onEnter,
  onLeave,
}: {
  node: TreeNode;
  top: number;
  left: number;
  onEnter: () => void;
  onLeave: () => void;
}) {
  const { t } = useTranslation();
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!box.current) return;
    box.current.style.top = `${top}px`;
    box.current.style.left = `${left}px`;
  }, [top, left]);
  return (
    <div ref={box} role="tooltip" className="row-card" onMouseEnter={onEnter} onMouseLeave={onLeave}>
      <strong>{node.title}</strong>
      <span className="muted">
        {t('tree.changedBy', { name: node.updated_by_name || t('common.unknown') })}{' '}
        <RelativeTime value={node.updated_at} />
      </span>
      {node.owner_name && <span className="muted">{t('tree.ownedBy', { name: node.owner_name })}</span>}
      {node.stage_text && <span className="muted">{node.stage_text}</span>}
    </div>
  );
}

interface RowProps {
  node: TreeNode;
  current?: string;
  filter: string;
  tabStop: boolean;
  onAdd?: (parent: string) => void;
  onDialog: (page: PageSummary, dialog: RowDialog) => void;
  onFocusRow: (id: string) => void;
  card: ReturnType<typeof useRowCard>;
  picking: boolean;
  picked: boolean;
  onPick: (id: string, range: boolean) => void;
}

function RowLink({ node, current, filter, tabStop, onFocusRow, card, picking, picked, onPick }: RowProps) {
  const { t } = useTranslation();
  const shared = {
    draggable: false,
    'data-row': node.id,
    tabIndex: tabStop ? 0 : -1,
    onFocus: (e: { currentTarget: HTMLElement }) => {
      onFocusRow(node.id);
      card.show(node, e.currentTarget);
    },
    onBlur: card.hide,
    onMouseEnter: (e: { currentTarget: HTMLElement }) => card.show(node, e.currentTarget),
    onMouseLeave: card.hide,
    onClick: (e: { preventDefault: () => void; shiftKey: boolean }) => {
      if (!picking) return;
      e.preventDefault();
      onPick(node.id, e.shiftKey);
    },
  };
  if (node.kind === 'link') {
    return (
      <a {...shared} href={node.link_target} target="_blank" rel="noopener noreferrer">
        <RowTitle node={node} filter={filter} />
      </a>
    );
  }
  return (
    <Link
      {...shared}
      href={`/pages/${node.id}`}
      className={node.status === 'draft' ? 'unpublished' : undefined}
      aria-current={node.id === current ? 'page' : undefined}
    >
      <RowTitle node={node} filter={filter} />
      {picked && <span className="visually-hidden"> {t('tree.selected')}</span>}
    </Link>
  );
}

function RowCheck({ node, picked, onPick }: RowProps) {
  const { t } = useTranslation();
  return (
    <input
      type="checkbox"
      className="row-check"
      tabIndex={-1}
      aria-label={t('favourites.pick', { title: node.title })}
      checked={picked}
      onChange={(e) => onPick(node.id, (e.nativeEvent as MouseEvent).shiftKey)}
    />
  );
}

function RowTools({ node, tabStop, onAdd, onDialog }: RowProps) {
  const { t } = useTranslation();
  return (
    <>
      <span className="node-tools">
        <Menu
          tabIndex={tabStop ? 0 : -1}
          label={t('tree.rowMenu', { title: node.title })}
          className="node-add"
          trigger={<IconMore size={ROW_ICON_SIZE} />}
          floating
        >
          <RowActions node={node} onDialog={onDialog} />
        </Menu>
      </span>
      {onAdd && node.status === 'published' && (
        <button
          type="button"
          className="node-add"
          tabIndex={tabStop ? 0 : -1}
          aria-label={t('space.addChild', { title: node.title })}
          title={t('space.addChild', { title: node.title })}
          onClick={() => onAdd(node.id)}
        >
          <IconPlus size={ROW_ICON_SIZE} />
        </button>
      )}
    </>
  );
}

function RowIcon({ node }: { node: TreeNode }) {
  const plain =
    node.kind === 'folder' ? (
      <IconFolder size={ROW_ICON_SIZE} />
    ) : node.kind === 'database' ? (
      <IconSheet size={ROW_ICON_SIZE} />
    ) : (
      <IconPage size={ROW_ICON_SIZE} />
    );
  return (
    <span className="twist">
      {node.emoji ? (
        <span className="page-emoji" aria-hidden="true">
          {node.emoji}
        </span>
      ) : node.kind === 'link' ? (
        <IconLink size={ROW_ICON_SIZE} />
      ) : (
        plain
      )}
    </span>
  );
}

function FlatList({ nodes, row }: { nodes: TreeNode[]; row: (node: TreeNode) => RowProps }) {
  return (
    <ul>
      {nodes.map((node) => {
        const props = row(node);
        return (
          <li key={node.id}>
            <div className={node.id === props.current ? 'node current' : 'node'}>
              {props.picking && <RowCheck {...props} />}
              <RowIcon node={node} />
              <RowLink {...props} />
              <RowMarks node={node} />
              <RowTools {...props} />
            </div>
          </li>
        );
      })}
    </ul>
  );
}

/** The button that brings the next rows of a level. */
function MoreRowsButton({ rows, onMore }: { rows: MoreRows | undefined; onMore: (rows: MoreRows) => void }) {
  const { t } = useTranslation();
  if (!rows) return null;
  return (
    <li>
      <button type="button" className="ghost tree-more" onClick={() => onMore(rows)}>
        {t('tree.showMore')}
      </button>
    </li>
  );
}

export function PageTree({
  spaceId,
  nodes: loaded,
  found,
  more,
  onMore,
  current,
  canMove,
  filter,
  view,
  memory,
  onMemory,
  onAdd,
  onDialog,
  selection,
  onSelection,
}: {
  spaceId: string;
  nodes: TreeNode[];
  /** What the server found, while a filter or a flat view is on. */
  found: TreeFound | undefined;
  more: MoreRows[];
  onMore: (rows: MoreRows) => void;
  current?: string;
  canMove: boolean;
  filter: string;
  view: TreeView;
  memory: BranchMemory;
  onMemory: (next: BranchMemory) => void;
  onAdd?: (parent: string) => void;
  onDialog: (page: PageSummary, dialog: RowDialog) => void;
  /** The picked rows while Select is on. */
  selection: string[] | null;
  onSelection: (next: string[]) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { preferences } = usePreferences();
  const card = useRowCard(preferences.hover_cards);
  const root = useRef<HTMLElement>(null);
  const filtering = filter.trim() !== '';
  const nodes = found?.pages ?? loaded;
  const tree = useMemo(() => buildTree(nodes), [nodes]);
  const ancestors = useMemo(() => ancestorsOf(loaded, current), [loaded, current]);
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);
  const [focusRow, setFocusRow] = useState<string | undefined>();
  const openTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const typed = useRef({ text: '', at: 0 });
  const anchor = useRef('');

  // Going to a page reveals it, also in a collapsed branch. Only a new page does:
  // collapsing the branch you are in must stay possible. The tree comes in parts, so this
  // waits until the row of the page is there: before that its parents are not known.
  const [revealedPage, setRevealedPage] = useState<string | undefined>();
  if (current !== revealedPage && (current === undefined || loaded.some((n) => n.id === current))) {
    setRevealedPage(current);
    onMemory(revealPage(memory, ancestors));
  }

  const move = useMutation({
    mutationFn: (v: { id: string; parent_id: string | null; before_id: string | null }) =>
      api.post<Page>(`/pages/${v.id}/move`, { parent_id: v.parent_id, before_id: v.before_id }),
    onSettled: () => client.invalidateQueries({ queryKey: ['tree', spaceId] }),
  });

  const draggable = canMove && !filtering && view === 'tree';
  const flat = useMemo(() => {
    const byId = new Map(nodes.map((n) => [n.id, n]));
    return view === 'tree' ? [] : (found?.matches ?? []).flatMap((id) => byId.get(id) ?? []);
  }, [nodes, found, view]);
  const firstRow = view === 'tree' ? tree[0]?.id : flat[0]?.id;
  const tabRow = focusRow && nodes.some((n) => n.id === focusRow) ? focusRow : (current ?? firstRow);
  const visibleRows = () =>
    Array.from(root.current?.querySelectorAll<HTMLAnchorElement>('a[data-row]') ?? []);

  const allow = (e: DragEvent, key: string) => {
    if (!dragging) return;
    e.preventDefault();
    setTarget(key);
  };
  const scrollNearEdge = (e: DragEvent) => {
    const scroller = root.current?.closest('.sidebar');
    if (!dragging || !scroller) return;
    const box = scroller.getBoundingClientRect();
    if (e.clientY < box.top + SCROLL_EDGE_PX) scroller.scrollBy(0, -SCROLL_STEP_PX);
    if (e.clientY > box.bottom - SCROLL_EDGE_PX) scroller.scrollBy(0, SCROLL_STEP_PX);
  };
  const openSoon = (branch: Branch, isOpen: boolean) => {
    clearTimeout(openTimer.current);
    if (isOpen || (branch.children.length === 0 && !branch.has_children)) return;
    openTimer.current = setTimeout(() => onMemory(openBranch(memory, branch.id)), OPEN_WHILE_DRAGGING_MS);
  };
  const finish = (e: DragEvent, parent: string | null, before: string | null) => {
    e.preventDefault();
    e.stopPropagation();
    clearTimeout(openTimer.current);
    const id = dragging;
    setDragging(null);
    setTarget(null);
    if (id && id !== parent && id !== before) move.mutate({ id, parent_id: parent, before_id: before });
  };

  const pick = (id: string, range: boolean) => {
    if (!selection) return;
    const order = visibleRows().map((row) => row.dataset.row ?? '');
    onSelection(
      range && anchor.current ? rangeOfRows(order, selection, anchor.current, id) : toggleRow(selection, id),
    );
    anchor.current = id;
  };

  const moveFocus = (row: HTMLAnchorElement | undefined) => row?.focus();
  const typeAhead = (key: string, rows: HTMLAnchorElement[], at: number) => {
    const now = Date.now();
    const text = now - typed.current.at < TYPE_AHEAD_MS ? typed.current.text + key : key;
    typed.current = { text, at: now };
    const titles = rows.map((r) => r.textContent ?? '');
    const found = typeAheadRow(titles, text.length > 1 ? at - 1 : at, text);
    if (found >= 0) moveFocus(rows[found]);
  };
  const keys = (e: KeyboardEvent<HTMLElement>) => {
    const rows = visibleRows();
    const at = rows.indexOf(document.activeElement as HTMLAnchorElement);
    if (at < 0 || e.ctrlKey || e.metaKey || e.altKey) return;
    const here = rows[at];
    const id = here?.dataset.row ?? '';
    const forward = getComputedStyle(e.currentTarget).direction === 'rtl' ? 'ArrowLeft' : 'ArrowRight';
    const backward = forward === 'ArrowLeft' ? 'ArrowRight' : 'ArrowLeft';
    const toggle = here?.closest('.node')?.querySelector<HTMLElement>('button.twist');
    const isOpen = toggle?.getAttribute('aria-expanded') === 'true';
    if (e.key === 'ArrowDown') moveFocus(rows[at + 1]);
    else if (e.key === 'ArrowUp') moveFocus(rows[at - 1]);
    else if (e.key === 'Home') moveFocus(rows[0]);
    else if (e.key === 'End') moveFocus(rows[rows.length - 1]);
    else if (e.key === forward) {
      if (toggle && !isOpen) onMemory(openBranch(memory, id));
      else if (toggle) moveFocus(rows[at + 1]);
    } else if (e.key === backward) {
      const parent = nodes.find((n) => n.id === id)?.parent_id;
      if (toggle && isOpen) onMemory(collapseBranch(memory, id));
      else if (parent) moveFocus(rows.find((r) => r.dataset.row === parent));
    } else if (e.key === ' ' && selection) {
      pick(id, e.shiftKey);
    } else if (e.key.length === 1 && e.key !== ' ' && view === 'tree') {
      typeAhead(e.key, rows, at);
    } else {
      return;
    }
    e.preventDefault();
    // A page shortcut must not fire as well: the letter was for the tree.
    e.stopPropagation();
  };

  const rowProps = (node: TreeNode): RowProps => ({
    node,
    current,
    filter,
    tabStop: node.id === tabRow,
    onAdd,
    onDialog,
    onFocusRow: setFocusRow,
    card,
    picking: selection !== null,
    picked: selection?.includes(node.id) ?? false,
    onPick: pick,
  });

  const render = (list: Branch[], depth: number, parent: string | null): ReactNode => (
    <ul>
      {list.map((b) => {
        const expanded = filtering || showsBranch(memory, b, depth, ancestors);
        const hasChildren = b.children.length > 0 || (b.has_children ?? false);
        return (
          <li key={b.id}>
            {draggable && (
              <div
                className={target === 'gap:' + b.id ? 'gap active' : 'gap'}
                data-testid={`gap-${b.title}`}
                onDragOver={(e) => allow(e, 'gap:' + b.id)}
                onDragLeave={() => setTarget(null)}
                // The real parent: rows at the top can have different ones.
                onDrop={(e) => finish(e, b.parent_id, b.id)}
              />
            )}
            <div
              className={[
                'node',
                b.id === current ? 'current' : '',
                target === 'in:' + b.id ? 'drop' : '',
              ].join(' ')}
              draggable={draggable}
              onDragStart={(e) => {
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', b.title);
                e.dataTransfer.setData(PAGE_DRAG_TYPE, b.id);
                setDragging(b.id);
              }}
              onDragEnd={() => {
                clearTimeout(openTimer.current);
                setDragging(null);
                setTarget(null);
              }}
              onDragEnter={() => {
                if (dragging) openSoon(b, expanded);
              }}
              onDragOver={(e) => allow(e, 'in:' + b.id)}
              onDragLeave={() => {
                clearTimeout(openTimer.current);
                setTarget(null);
              }}
              onDrop={(e) => finish(e, b.id, null)}
            >
              {selection !== null && <RowCheck {...rowProps(b)} />}
              {hasChildren ? (
                <button
                  type="button"
                  className="twist"
                  tabIndex={-1}
                  aria-label={b.title}
                  aria-expanded={expanded}
                  onClick={() => onMemory((expanded ? collapseBranch : openBranch)(memory, b.id))}
                >
                  {expanded ? (
                    <IconChevronDown size={ROW_ICON_SIZE} />
                  ) : (
                    <IconChevronRight size={ROW_ICON_SIZE} />
                  )}
                </button>
              ) : (
                <RowIcon node={b} />
              )}
              <RowLink {...rowProps(b)} />
              <RowMarks node={b} />
              <RowTools {...rowProps(b)} />
            </div>
            {hasChildren && expanded && render(b.children, depth + 1, b.id)}
          </li>
        );
      })}
      {!found && <MoreRowsButton rows={more.find((m) => m.parent === parent)} onMore={onMore} />}
    </ul>
  );

  const nothing = view === 'tree' ? tree.length === 0 : flat.length === 0;

  if (nodes.length === 0 && !found) return <p className="muted">{t('space.emptyTree')}</p>;
  return (
    <nav
      ref={root}
      className="tree"
      aria-label={t('space.pages')}
      onKeyDown={keys}
      onDragOver={scrollNearEdge}
    >
      {nothing && <p className="muted">{filtering ? t('tree.noMatch') : t('tree.emptyView')}</p>}
      {view === 'tree' ? render(tree, 0, null) : <FlatList nodes={flat} row={rowProps} />}
      {view !== 'tree' && !nothing && <p className="muted tree-hint">{t('tree.viewHint')}</p>}
      {found?.capped && <p className="muted tree-hint">{t('tree.capped')}</p>}
      {move.error && <ErrorNote error={move.error} />}
      {card.card && (
        <RowCard
          node={card.card.node}
          top={card.card.top}
          left={card.card.left}
          onEnter={card.keep}
          onLeave={card.hide}
        />
      )}
    </nav>
  );
}

/** What can be done to several rows at once. */
function SelectionBar({
  spaceId,
  picked,
  onDone,
}: {
  spaceId: string;
  picked: string[];
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const [moving, setMoving] = useState(false);
  const archive = useMutation({
    mutationFn: async () => {
      const answers = await Promise.allSettled(
        picked.map((id) => api.post(`/pages/${id}/archive`, { include_children: true })),
      );
      return answers.filter((a) => a.status === 'fulfilled').length;
    },
    onSuccess: async (done) => {
      await client.invalidateQueries({ queryKey: ['tree', spaceId] });
      toast({ message: t('move.archived', { done, failed: picked.length - done }) });
      onDone();
    },
  });
  return (
    <div className="select-bar" role="group" aria-label={t('tree.select')}>
      <span role="status">{t('content.selected', { count: picked.length })}</span>
      <button type="button" disabled={picked.length === 0} onClick={() => setMoving(true)}>
        {t('page.move')}
      </button>
      <button type="button" disabled={picked.length === 0} onClick={() => archive.mutate()}>
        {t('organise.archivePage')}
      </button>
      <ErrorNote error={archive.error} />
      {moving && (
        <BulkMoveDialog ids={picked} spaceId={spaceId} onClose={() => setMoving(false)} onDone={onDone} />
      )}
    </div>
  );
}

/** The heading, tools, filter and tree of a space's sidebar. */
export function PagesSection({
  spaceId,
  home,
  current,
  canMove,
  onAdd,
  onAddRoot,
  onAddLink,
  onDialog,
}: {
  spaceId: string;
  /** The home page has its own row above the tree. */
  home: string | null;
  current?: string;
  canMove: boolean;
  onAdd?: (parent: string) => void;
  onAddRoot?: () => void;
  onAddLink?: () => void;
  onDialog: (page: PageSummary, dialog: RowDialog) => void;
}) {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const [memory, setMemory] = useBranchMemory(spaceId);
  const [filter, setFilter] = useState('');
  const [selection, setSelection] = useState<string[] | null>(null);
  const view = TREE_VIEWS.find((v) => v === preferences.tree_view) ?? 'tree';
  const parts = useTreeParts(spaceId, current, memory.opened);
  const nodes = useMemo(() => parts.nodes.filter((n) => n.id !== home), [parts.nodes, home]);
  const typed = useDelayed(filter.trim());
  const findPages = typed !== '' || view !== 'tree';
  const found = useQuery({
    queryKey: ['tree', spaceId, 'find', typed, view],
    queryFn: () => {
      const params = new URLSearchParams({ q: typed, view });
      return api.get<TreeFound>(`/spaces/${spaceId}/tree/find?${params.toString()}`);
    },
    enabled: findPages,
    // The rows stay while the next ones load.
    placeholderData: (previous) => previous,
  });
  const shown = useMemo(() => {
    if (!findPages || !found.data) return undefined;
    return { ...found.data, pages: found.data.pages.filter((n) => n.id !== home) };
  }, [findPages, found.data, home]);
  const showCurrent = () => {
    setFilter('');
    if (view !== 'tree') update({ tree_view: 'tree' });
    setMemory(revealPage(memory, ancestorsOf(nodes, current)));
    requestAnimationFrame(() => {
      document.querySelector('.tree a[aria-current="page"]')?.scrollIntoView({ block: 'nearest' });
    });
  };
  return (
    <>
      <div className="sidebar-section">
        {t('space.pages')}
        <span className="sidebar-tools">
          <button
            type="button"
            className="node-add always"
            aria-label={t('tree.collapseAll')}
            title={t('tree.collapseAll')}
            onClick={() => setMemory(collapseEverything(nodes))}
          >
            <IconCollapse size={ROW_ICON_SIZE} />
          </button>
          {current && (
            <button
              type="button"
              className="node-add always"
              aria-label={t('tree.showCurrent')}
              title={t('tree.showCurrent')}
              onClick={showCurrent}
            >
              <IconLocate size={ROW_ICON_SIZE} />
            </button>
          )}
          {onAddRoot && (
            <button
              type="button"
              className="node-add always"
              aria-label={t('space.newPage')}
              title={t('space.newPage')}
              onClick={onAddRoot}
            >
              <IconPlus size={ROW_ICON_SIZE} />
            </button>
          )}
          {onAddLink && (
            <button
              type="button"
              className="node-add always"
              aria-label={t('tree.addLink')}
              title={t('tree.addLink')}
              onClick={onAddLink}
            >
              <IconLink size={ROW_ICON_SIZE} />
            </button>
          )}
          {canMove && (
            <button
              type="button"
              className="node-add always"
              aria-label={t('tree.select')}
              title={t('tree.select')}
              aria-pressed={selection !== null}
              onClick={() => setSelection(selection === null ? [] : null)}
            >
              <IconTask size={ROW_ICON_SIZE} />
            </button>
          )}
        </span>
      </div>
      <div className="tree-controls">
        <input
          type="search"
          value={filter}
          aria-label={t('tree.filter')}
          placeholder={t('tree.filter')}
          onChange={(e) => setFilter(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') setFilter('');
          }}
        />
        <select
          aria-label={t('tree.view')}
          value={view}
          onChange={(e) => update({ tree_view: e.target.value })}
        >
          {TREE_VIEWS.map((v) => (
            <option key={v} value={v}>
              {t(`tree.view_${v}`)}
            </option>
          ))}
        </select>
      </div>
      {parts.isPending && <Loading />}
      <ErrorNote error={parts.error ?? found.error} />
      <PageTree
        spaceId={spaceId}
        nodes={nodes}
        found={shown}
        more={parts.more}
        onMore={parts.showMore}
        current={current}
        canMove={canMove}
        filter={filter}
        view={view}
        memory={memory}
        onMemory={setMemory}
        onAdd={onAdd}
        onDialog={onDialog}
        selection={selection}
        onSelection={setSelection}
      />
      {selection !== null && (
        <SelectionBar spaceId={spaceId} picked={selection} onDone={() => setSelection(null)} />
      )}
    </>
  );
}
