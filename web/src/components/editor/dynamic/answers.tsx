// SPDX-License-Identifier: Apache-2.0
// How the answer of a block looks. The server has already left out what this reader may
// not see, so everything here only draws.
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { Json } from '../../../api';
import { useDates } from '../../../dates';
import { Link } from '../../../router';
import { IconChevronDown, IconChevronLeft, IconChevronRight } from '../../icons';
import { Avatar } from '../../ui';
import { DocumentView } from '../editor';

const CARD_TURN_MS = 6000;
const CLOUD_WEIGHTS = 4;
const NAV_ICON_SIZE = 16;

export interface BlockRow {
  id: string;
  parent_id?: string;
  title: string;
  emoji?: string;
  space_key?: string;
  space_name?: string;
  at?: string;
  by?: string;
  detail?: string;
  count?: number;
  cells?: string[];
  done?: boolean;
  image?: string;
}

export interface BlockPerson {
  id: string;
  name: string;
  job_title?: string;
  department?: string;
  location?: string;
  email?: string;
  has_picture: boolean;
}

export interface BlockAnswer {
  rows: BlockRow[];
  columns?: string[];
  column_keys?: string[];
  document?: Json;
  source?: { id: string; title: string; can_edit?: boolean };
  people?: BlockPerson[];
}

function RowLink({ row }: { row: BlockRow }) {
  return (
    <Link href={`/pages/${row.id}`}>
      {row.emoji && <span aria-hidden="true">{row.emoji} </span>}
      {row.title}
    </Link>
  );
}

function PageRows({ rows }: { rows: BlockRow[] }) {
  return (
    <ul className="block-list">
      {rows.map((row) => (
        <li key={row.id}>
          <RowLink row={row} />
          {row.space_name && <span className="muted"> · {row.space_name}</span>}
          {row.detail && <p className="muted block-preview">{row.detail}</p>}
        </li>
      ))}
    </ul>
  );
}

function Card({ row }: { row: BlockRow }) {
  return (
    <>
      {row.image && (
        <img className="block-card-image" src={`/api/v1/attachments/${row.image}/content`} alt="" />
      )}
      <RowLink row={row} />
      {row.detail && <p className="muted">{row.detail}</p>}
    </>
  );
}

function LinkCards({ rows }: { rows: BlockRow[] }) {
  return (
    <ul className="block-cards">
      {rows.map((row) => (
        <li key={row.id} className="block-card">
          <Card row={row} />
        </li>
      ))}
    </ul>
  );
}

function asksForStillness(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** One card at a time. It never moves for someone who asked for no motion (BLK-062). */
function RotatingCards({ rows }: { rows: BlockRow[] }) {
  const { t } = useTranslation();
  const [shown, setShown] = useState(0);
  const [resting, setResting] = useState(false);
  const count = rows.length;
  useEffect(() => {
    if (resting || count < 2 || asksForStillness()) return;
    const timer = setInterval(() => setShown((n) => (n + 1) % count), CARD_TURN_MS);
    return () => clearInterval(timer);
  }, [resting, count]);
  const row = rows[shown % count];
  if (!row) return null;
  return (
    <div
      className="block-rotating"
      role="group"
      aria-roledescription={t('blocks.rotating')}
      onMouseEnter={() => setResting(true)}
      onMouseLeave={() => setResting(false)}
      onFocus={() => setResting(true)}
      onBlur={() => setResting(false)}
    >
      <div className="block-card">
        <Card row={row} />
      </div>
      {count > 1 && (
        <div className="row">
          <button
            type="button"
            className="icon"
            aria-label={t('blocks.previousCard')}
            onClick={() => setShown((n) => (n + count - 1) % count)}
          >
            <IconChevronLeft size={NAV_ICON_SIZE} />
          </button>
          <span className="muted">{t('blocks.cardOf', { shown: (shown % count) + 1, count })}</span>
          <button
            type="button"
            className="icon"
            aria-label={t('blocks.nextCard')}
            onClick={() => setShown((n) => (n + 1) % count)}
          >
            <IconChevronRight size={NAV_ICON_SIZE} />
          </button>
        </div>
      )}
    </div>
  );
}

function FactsTable({ answer }: { answer: BlockAnswer }) {
  const { t } = useTranslation();
  return (
    <div className="block-frame">
      <table>
        <thead>
          <tr>
            <th>{t('blocks.page')}</th>
            {answer.columns?.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {answer.rows.map((row) => (
            <tr key={row.id}>
              <td>
                <RowLink row={row} />
              </td>
              {row.cells?.map((cell, i) => (
                <td key={answer.columns?.[i] ?? i}>{cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** A table whose columns the server names by key, so the words come from here. */
function KeyedTable({ answer, first }: { answer: BlockAnswer; first: ReactNode }) {
  const { t } = useTranslation();
  const dates = useDates();
  const keys = answer.column_keys ?? [];
  return (
    <div className="block-frame">
      <table>
        <thead>
          <tr>
            <th>{first}</th>
            {keys.map((key) => (
              <th key={key}>{t(`blocks.column_${key}`)}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {answer.rows.map((row) => (
            <tr key={row.id}>
              <td>{answer.rows[0]?.space_key ? <RowLink row={row} /> : row.title}</td>
              {row.cells?.map((cell, i) => (
                <td key={keys[i] ?? i}>{keys[i] === 'changed' && cell ? dates.day(cell) : cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ContributionTable({ answer, byPage }: { answer: BlockAnswer; byPage: boolean }) {
  const { t } = useTranslation();
  if (!byPage) return <KeyedTable answer={answer} first={t('blocks.column_person')} />;
  const keys = answer.column_keys ?? [];
  return (
    <div className="block-frame">
      <table>
        <thead>
          <tr>
            <th>{t('blocks.page')}</th>
            {keys.map((key) => (
              <th key={key}>{t(`blocks.column_${key}`)}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {answer.rows.map((row) => (
            <tr key={row.id}>
              <td>
                <RowLink row={row} />
              </td>
              {row.cells?.map((cell, i) => (
                <td key={keys[i] ?? i}>{cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function firstLetter(text: string): string {
  return text.trim().charAt(0).toLocaleUpperCase() || '#';
}

/** Filters in the browser: the block has already loaded at most 500 rows (spec 026, D15). */
function useLetters(rows: BlockRow[]) {
  const [letter, setLetter] = useState('');
  const letters = useMemo(() => [...new Set(rows.map((r) => firstLetter(r.title)))].sort(), [rows]);
  const shown = letter ? rows.filter((r) => firstLetter(r.title) === letter) : rows;
  return { letter, setLetter, letters, shown };
}

function LetterBar({
  letters,
  letter,
  onPick,
}: {
  letters: string[];
  letter: string;
  onPick: (next: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="block-letters" role="group" aria-label={t('blocks.byLetter')}>
      <button type="button" className="ghost" aria-pressed={letter === ''} onClick={() => onPick('')}>
        {t('blocks.allLetters')}
      </button>
      {letters.map((item) => (
        <button
          key={item}
          type="button"
          className="ghost"
          aria-pressed={letter === item}
          onClick={() => onPick(item)}
        >
          {item}
        </button>
      ))}
    </div>
  );
}

function SpaceIndex({ rows }: { rows: BlockRow[] }) {
  const { letter, setLetter, letters, shown } = useLetters(rows);
  return (
    <>
      <LetterBar letters={letters} letter={letter} onPick={setLetter} />
      <PageRows rows={shown} />
    </>
  );
}

function LabelRows({ rows, cloud }: { rows: BlockRow[]; cloud?: boolean }) {
  const most = Math.max(1, ...rows.map((row) => row.count ?? 1));
  if (cloud) {
    return (
      <ul className="block-cloud">
        {rows.map((row) => (
          <li key={row.title} data-weight={Math.ceil(((row.count ?? 1) / most) * CLOUD_WEIGHTS)}>
            <Link href={`/labels/${encodeURIComponent(row.title)}`}>{row.title}</Link>
          </li>
        ))}
      </ul>
    );
  }
  return (
    <ul className="block-list">
      {rows.map((row) => (
        <li key={row.title}>
          <Link href={`/labels/${encodeURIComponent(row.title)}`}>{row.title}</Link>
          <span className="muted"> · {row.count}</span>
        </li>
      ))}
    </ul>
  );
}

function LabelIndex({ rows }: { rows: BlockRow[] }) {
  const { letter, setLetter, letters, shown } = useLetters(rows);
  return (
    <>
      <LetterBar letters={letters} letter={letter} onPick={setLetter} />
      <LabelRows rows={shown} />
    </>
  );
}

interface TreeNode {
  row: BlockRow;
  children: TreeNode[];
}

function treeOf(rows: BlockRow[]): TreeNode[] {
  const nodes = new Map(rows.map((row) => [row.id, { row, children: [] as TreeNode[] }]));
  const roots: TreeNode[] = [];
  for (const node of nodes.values()) {
    const parent = node.row.parent_id ? nodes.get(node.row.parent_id) : undefined;
    (parent ? parent.children : roots).push(node);
  }
  return roots;
}

function TreeBranch({
  node,
  open,
  onToggle,
}: {
  node: TreeNode;
  open: Set<string>;
  onToggle: (id: string) => void;
}) {
  const { t } = useTranslation();
  const isOpen = open.has(node.row.id);
  return (
    <li>
      {node.children.length > 0 && (
        <button
          type="button"
          className="ghost"
          aria-expanded={isOpen}
          aria-label={t(isOpen ? 'blocks.collapse' : 'blocks.expand', { title: node.row.title })}
          onClick={() => onToggle(node.row.id)}
        >
          {isOpen ? <IconChevronDown size={NAV_ICON_SIZE} /> : <IconChevronRight size={NAV_ICON_SIZE} />}
        </button>
      )}
      <RowLink row={node.row} />
      {isOpen && node.children.length > 0 && (
        <ul className="block-list">
          {node.children.map((child) => (
            <TreeBranch key={child.row.id} node={child} open={open} onToggle={onToggle} />
          ))}
        </ul>
      )}
    </li>
  );
}

function PageTree({ rows }: { rows: BlockRow[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState<Set<string>>(new Set());
  const roots = useMemo(() => treeOf(rows), [rows]);
  const toggle = (id: string) =>
    setOpen((before) => {
      const next = new Set(before);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  return (
    <>
      <div className="row">
        <button type="button" className="ghost" onClick={() => setOpen(new Set(rows.map((r) => r.id)))}>
          {t('blocks.expandAll')}
        </button>
        <button type="button" className="ghost" onClick={() => setOpen(new Set())}>
          {t('blocks.collapseAll')}
        </button>
      </div>
      <ul className="block-list">
        {roots.map((node) => (
          <TreeBranch key={node.row.id} node={node} open={open} onToggle={toggle} />
        ))}
      </ul>
    </>
  );
}

function PersonBox({ person }: { person: BlockPerson }) {
  return (
    <>
      {person.has_picture ? (
        <img className="avatar-picture" src={`/api/v1/people/${person.id}/picture`} alt="" />
      ) : (
        <Avatar name={person.name} />
      )}
      <span className="person-card-text">
        <Link href={`/people/${person.id}`}>{person.name}</Link>
        {person.job_title && <span className="muted">{person.job_title}</span>}
        {person.department && <span className="muted">{person.department}</span>}
        {person.location && <span className="muted">{person.location}</span>}
        {person.email && <a href={`mailto:${person.email}`}>{person.email}</a>}
      </span>
    </>
  );
}

function PeopleBlock({ people, grid }: { people: BlockPerson[]; grid: boolean }) {
  return (
    <ul className={grid ? 'block-people grid' : 'block-people'}>
      {people.map((person) => (
        <li key={person.id}>
          <PersonBox person={person} />
        </li>
      ))}
    </ul>
  );
}

function SpaceCards({ rows }: { rows: BlockRow[] }) {
  return (
    <ul className="block-cards">
      {rows.map((row) => (
        <li key={row.id} className="block-card">
          <Link href={`/spaces/${row.space_key}`}>{row.title}</Link>
        </li>
      ))}
    </ul>
  );
}

function PlainRows({ answer, kind }: { answer: BlockAnswer; kind: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <ul className="block-list">
      {answer.rows.map((row, i) => (
        <li key={`${row.id}-${i}`}>
          {kind === 'attachments_here' || kind === 'space_files' ? (
            <a href={`/api/v1/attachments/${row.id}/content?download=1`}>{row.title}</a>
          ) : kind === 'decision_report' || kind === 'popular_pages' ? (
            <Link href={`/pages/${row.id}`}>{row.title}</Link>
          ) : kind === 'task_report' || kind === 'my_tasks' ? (
            <Link href={`/pages/${row.parent_id}`}>{row.title}</Link>
          ) : kind === 'spaces_list' ? (
            <Link href={`/spaces/${row.space_key}`}>{row.title}</Link>
          ) : (
            <span>{row.title || t('blocks.noSummary')}</span>
          )}
          {row.detail && <span className="muted"> · {row.detail}</span>}
          {row.by && <span className="muted"> · {row.by}</span>}
          {row.at && <span className="muted"> · {dates.day(row.at)}</span>}
          {kind === 'contributors' && (
            <span className="muted"> · {t('blocks.contributions', { count: row.count ?? 0 })}</span>
          )}
          {kind === 'popular_pages' && (
            <span className="muted"> · {t('blocks.views', { count: row.count ?? 0 })}</span>
          )}
        </li>
      ))}
    </ul>
  );
}

const PAGE_LISTS = new Set([
  'child_pages',
  'recent_changes',
  'activity_tabs',
  'my_favourites',
  'my_drafts',
  'filter_list',
  'blog_posts',
  'breadcrumbs',
]);

type Settings = Record<string, unknown>;

function Include({ answer, frame, synced }: { answer: BlockAnswer; frame: boolean; synced: boolean }) {
  const { t } = useTranslation();
  const source = answer.source;
  return (
    <div className={frame || synced ? 'block-include framed' : 'block-include'}>
      {answer.document !== undefined && <DocumentView document={answer.document} plain />}
      {source?.title && (
        <span className="row">
          <Link className="block-source" href={`/pages/${source.id}`}>
            {t(synced ? 'blocks.syncedFrom' : 'blocks.source', { title: source.title })}
          </Link>
          {synced && source.can_edit && (
            <Link className="block-source" href={`/pages/${source.id}/edit`}>
              {t('blocks.editSource')}
            </Link>
          )}
        </span>
      )}
    </div>
  );
}

/** Picks the way one kind of block looks. */
export function Answer({
  kind,
  answer,
  settings,
}: {
  kind: string;
  answer: BlockAnswer;
  settings: Settings;
}) {
  if (answer.document) {
    return <Include answer={answer} frame={settings.frame === true} synced={kind === 'synced_block'} />;
  }
  if (answer.people) return <PeopleBlock people={answer.people} grid={settings.style === 'grid'} />;
  switch (kind) {
    case 'facts_report':
      return <FactsTable answer={answer} />;
    case 'report_table':
      return <ReportTable answer={answer} />;
    case 'contribution_table':
      return <ContributionTable answer={answer} byPage={settings.by === 'page'} />;
    case 'link_cards':
      return <LinkCards rows={answer.rows} />;
    case 'rotating_cards':
      return <RotatingCards rows={answer.rows} />;
    case 'page_tree':
      return <PageTree rows={answer.rows} />;
    case 'space_index':
      return <SpaceIndex rows={answer.rows} />;
    case 'labels_list':
      return <LabelIndex rows={answer.rows} />;
    case 'popular_labels':
      return <LabelRows rows={answer.rows} cloud={settings.style === 'cloud'} />;
    case 'related_labels':
    case 'recent_labels':
      return <LabelRows rows={answer.rows} />;
    case 'spaces_list':
      if (settings.style === 'cards') return <SpaceCards rows={answer.rows} />;
  }
  if (PAGE_LISTS.has(kind)) return <PageRows rows={answer.rows} />;
  return <PlainRows answer={answer} kind={kind} />;
}

function ReportTable({ answer }: { answer: BlockAnswer }) {
  const { t } = useTranslation();
  return <KeyedTable answer={answer} first={t('blocks.page')} />;
}
