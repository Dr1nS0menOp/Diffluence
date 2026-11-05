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
import { api, type Page, type PageSummary, type TreeNode } from '../api';
import { Link } from '../router';
import { usePreferences } from '../preferences';
import {
  IconChevronDown,
  IconChevronRight,
  IconCollapse,
  IconCopy,
  IconExternal,
  IconLink,
  IconLocate,
  IconLock,
  IconMore,
  IconMove,
  IconPage,
  IconPlus,
  IconRename,
  IconStar,
  IconTrash,
} from './icons';
import { Menu, MenuDownload, MenuItem, MenuSeparator } from './menu';
import { useCopy } from './toast';
import {
  ancestorsOf,
  buildTree,
  collapseBranch,
  collapseEverything,
  filterTree,
  flatPages,
  isBranchOpen,
  matchParts,
  openBranch,
  readBranchMemory,
  revealPage,
  typeAheadRow,
  type Branch,
  type BranchMemory,
  type TreeView,
} from './tree-model';
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

export type RowDialog = 'rename' | 'move' | 'copy' | 'delete';

/** Collapsed branches stay collapsed while the reader moves through the space. */
function useBranchMemory(spaceId: string): [BranchMemory, (next: BranchMemory) => void] {
  const key = `diffluence.tree.${spaceId}`;
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
}

function RowLink({ node, current, filter, tabStop, onFocusRow, card }: RowProps) {
  return (
    <Link
      href={`/pages/${node.id}`}
      draggable={false}
      data-row={node.id}
      tabIndex={tabStop ? 0 : -1}
      className={node.status === 'draft' ? 'unpublished' : undefined}
      aria-current={node.id === current ? 'page' : undefined}
      onFocus={(e) => {
        onFocusRow(node.id);
        card.show(node, e.currentTarget);
      }}
      onBlur={card.hide}
      onMouseEnter={(e) => card.show(node, e.currentTarget)}
      onMouseLeave={card.hide}
    >
      <RowTitle node={node} filter={filter} />
    </Link>
  );
}

function RowTools({ node, tabStop, onAdd, onDialog }: RowProps) {
  const { t } = useTranslation();
  return (
    <>
      <span className={tabStop ? 'node-tools shown' : 'node-tools'}>
        <Menu
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
  return (
    <span className="twist">
      {node.emoji ? (
        <span className="page-emoji" aria-hidden="true">
          {node.emoji}
        </span>
      ) : (
        <IconPage size={ROW_ICON_SIZE} />
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

export function PageTree({
  spaceId,
  nodes,
  current,
  canMove,
  filter,
  view,
  memory,
  onMemory,
  onAdd,
  onDialog,
}: {
  spaceId: string;
  nodes: TreeNode[];
  current?: string;
  canMove: boolean;
  filter: string;
  view: TreeView;
  memory: BranchMemory;
  onMemory: (next: BranchMemory) => void;
  onAdd?: (parent: string) => void;
  onDialog: (page: PageSummary, dialog: RowDialog) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { preferences } = usePreferences();
  const card = useRowCard(preferences.hover_cards);
  const root = useRef<HTMLElement>(null);
  const filtering = filter.trim() !== '';
  const kept = useMemo(() => (filtering ? filterTree(nodes, filter) : null), [nodes, filter, filtering]);
  const tree = useMemo(() => buildTree(kept ? nodes.filter((n) => kept.has(n.id)) : nodes), [nodes, kept]);
  const ancestors = useMemo(() => ancestorsOf(nodes, current), [nodes, current]);
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);
  const [focusRow, setFocusRow] = useState<string | undefined>();
  const openTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const typed = useRef({ text: '', at: 0 });

  // Going to a page reveals it, also in a collapsed branch. Only a new page does:
  // collapsing the branch you are in must stay possible.
  const [revealedPage, setRevealedPage] = useState<string | undefined>();
  if (current !== revealedPage) {
    setRevealedPage(current);
    onMemory(revealPage(memory, ancestors));
  }

  const move = useMutation({
    mutationFn: (v: { id: string; parent_id: string | null; before_id: string | null }) =>
      api.post<Page>(`/pages/${v.id}/move`, { parent_id: v.parent_id, before_id: v.before_id }),
    onSettled: () => client.invalidateQueries({ queryKey: ['tree', spaceId] }),
  });

  const draggable = canMove && !filtering && view === 'tree';
  const tabRow = focusRow && nodes.some((n) => n.id === focusRow) ? focusRow : (current ?? nodes[0]?.id);
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
    if (isOpen || branch.children.length === 0) return;
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
  });

  const render = (list: Branch[], parent: string | null, depth: number): ReactNode => (
    <ul>
      {list.map((b) => {
        const expanded = filtering || isBranchOpen(memory, b.id, depth, ancestors);
        const hasChildren = b.children.length > 0;
        return (
          <li key={b.id}>
            {draggable && (
              <div
                className={target === 'gap:' + b.id ? 'gap active' : 'gap'}
                data-testid={`gap-${b.title}`}
                onDragOver={(e) => allow(e, 'gap:' + b.id)}
                onDragLeave={() => setTarget(null)}
                onDrop={(e) => finish(e, parent, b.id)}
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
            {hasChildren && expanded && render(b.children, b.id, depth + 1)}
          </li>
        );
      })}
    </ul>
  );

  const flat = view === 'tree' ? [] : flatPages(nodes, view).filter((n) => !kept || kept.has(n.id));
  const nothing = view === 'tree' ? tree.length === 0 : flat.length === 0;

  if (nodes.length === 0) return <p className="muted">{t('space.emptyTree')}</p>;
  return (
    <nav
      ref={root}
      className="tree"
      aria-label={t('space.pages')}
      onKeyDown={keys}
      onDragOver={scrollNearEdge}
    >
      {nothing && <p className="muted">{filtering ? t('tree.noMatch') : t('tree.emptyView')}</p>}
      {view === 'tree' ? render(tree, null, 0) : <FlatList nodes={flat} row={rowProps} />}
      {view !== 'tree' && !nothing && <p className="muted tree-hint">{t('tree.viewHint')}</p>}
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

/** The heading, tools, filter and tree of a space's sidebar. */
export function PagesSection({
  spaceId,
  nodes,
  current,
  canMove,
  onAdd,
  onAddRoot,
  onDialog,
}: {
  spaceId: string;
  nodes: TreeNode[];
  current?: string;
  canMove: boolean;
  onAdd?: (parent: string) => void;
  onAddRoot?: () => void;
  onDialog: (page: PageSummary, dialog: RowDialog) => void;
}) {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const [memory, setMemory] = useBranchMemory(spaceId);
  const [filter, setFilter] = useState('');
  const view = TREE_VIEWS.find((v) => v === preferences.tree_view) ?? 'tree';
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
      <PageTree
        spaceId={spaceId}
        nodes={nodes}
        current={current}
        canMove={canMove}
        filter={filter}
        view={view}
        memory={memory}
        onMemory={setMemory}
        onAdd={onAdd}
        onDialog={onDialog}
      />
    </>
  );
}
