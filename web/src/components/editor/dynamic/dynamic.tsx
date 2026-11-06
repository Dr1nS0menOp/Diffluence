// SPDX-License-Identifier: Apache-2.0
// A block that shows something by itself. A document stores settings, and every
// reader asks the server for their own answer (spec 012, E1).
import { Component, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Node as DocumentNode } from '@tiptap/core';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import { api, type Json } from '../../../api';
import { useDates } from '../../../dates';
import { Link, match, navigate, useLocation } from '../../../router';
import { IconSliders } from '../../icons';
import { Dialog, ErrorNote, Field, Loading } from '../../ui';
import { DocumentView } from '../editor';
import { contentsOf, jumpToHeading } from '../outline';
import { DYNAMIC_KINDS, listFromText, LOCAL_KINDS, matchesPattern, type SettingField } from './kinds';

const NEAR_SCREEN = '300px';
const TYPING_PAUSE_MS = 300;
const SETTINGS_ICON_SIZE = 16;

type Settings = Record<string, unknown>;

interface BlockRow {
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
}

interface BlockAnswer {
  rows: BlockRow[];
  columns?: string[];
  document?: Json;
  source?: { id: string; title: string };
}

function usePageId(): string | undefined {
  const { pathname } = useLocation();
  return (
    match('/pages/:id', pathname)?.id ??
    match('/pages/:id/edit', pathname)?.id ??
    match('/pages/:id/history', pathname)?.id
  );
}

/** Asks only when the block comes near the screen, so text never waits for it. */
function useNearScreen(): [React.RefObject<HTMLDivElement | null>, boolean] {
  const box = useRef<HTMLDivElement>(null);
  const [near, setNear] = useState(false);
  useEffect(() => {
    if (!box.current) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) setNear(true);
      },
      { rootMargin: NEAR_SCREEN },
    );
    observer.observe(box.current);
    return () => observer.disconnect();
  }, []);
  return [box, near];
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
        </li>
      ))}
    </ul>
  );
}

function LinkCards({ rows }: { rows: BlockRow[] }) {
  return (
    <ul className="block-cards">
      {rows.map((row) => (
        <li key={row.id} className="block-card">
          <RowLink row={row} />
          {row.detail && <p className="muted">{row.detail}</p>}
        </li>
      ))}
    </ul>
  );
}

function Report({ answer }: { answer: BlockAnswer }) {
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

function PlainRows({ answer, kind }: { answer: BlockAnswer; kind: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <ul className="block-list">
      {answer.rows.map((row, i) => (
        <li key={`${row.id}-${i}`}>
          {kind === 'attachments_here' ? (
            <a href={`/api/v1/attachments/${row.id}/content?download=1`}>{row.title}</a>
          ) : kind === 'decision_report' ? (
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
            <span className="muted"> · {t('details.versions', { count: row.count ?? 0 })}</span>
          )}
          {kind === 'labels_list' && <span className="muted"> · {row.count}</span>}
        </li>
      ))}
    </ul>
  );
}

const PAGE_LISTS = new Set([
  'child_pages',
  'page_tree',
  'space_index',
  'recent_changes',
  'my_favourites',
  'my_drafts',
  'filter_list',
  'blog_posts',
  'breadcrumbs',
]);

function Answer({ kind, answer, frame }: { kind: string; answer: BlockAnswer; frame: boolean }) {
  const { t } = useTranslation();
  if (answer.document) {
    return (
      <div className={frame ? 'block-include framed' : 'block-include'}>
        <DocumentView document={answer.document} plain />
        {answer.source?.title && (
          <Link className="block-source" href={`/pages/${answer.source.id}`}>
            {t('blocks.source', { title: answer.source.title })}
          </Link>
        )}
      </div>
    );
  }
  if (kind === 'facts_report' || kind === 'report_table') return <Report answer={answer} />;
  if (kind === 'link_cards') return <LinkCards rows={answer.rows} />;
  if (PAGE_LISTS.has(kind)) return <PageRows rows={answer.rows} />;
  return <PlainRows answer={answer} kind={kind} />;
}

function Asked({ kind, settings }: { kind: string; settings: Settings }) {
  const { t } = useTranslation();
  const pageId = usePageId();
  const [box, near] = useNearScreen();
  const q = useQuery({
    queryKey: ['block', kind, pageId, settings],
    queryFn: () => api.post<BlockAnswer>(`/blocks/${kind}`, { page_id: pageId ?? null, settings }),
    enabled: near,
  });
  const empty = q.data && q.data.rows.length === 0 && !q.data.document;
  return (
    <div ref={box} className="dynamic-body">
      {(!near || q.isPending) && <Loading />}
      {q.error && (
        <div className="row">
          <ErrorNote error={q.error} />
          <button type="button" onClick={() => void q.refetch()}>
            {t('blocks.tryAgain')}
          </button>
        </div>
      )}
      {empty && !settings.hide_empty && <p className="muted">{t('blocks.empty')}</p>}
      {q.data && !empty && <Answer kind={kind} answer={q.data} frame={settings.frame === true} />}
    </div>
  );
}

function Contents({ document: content, settings }: { document: Json; settings: Settings }) {
  const { t } = useTranslation();
  const from = typeof settings.from === 'number' ? settings.from : 1;
  const to = typeof settings.to === 'number' ? settings.to : 3;
  const exclude = typeof settings.exclude === 'string' ? settings.exclude : '';
  const headings = contentsOf(content).filter(
    (h) => h.level >= from && h.level <= to && !(exclude && matchesPattern(h.text, exclude)),
  );
  if (headings.length === 0) return <p className="muted">{t('blocks.noHeadings')}</p>;
  const List = settings.numbered ? 'ol' : 'ul';
  return (
    <List className="block-list">
      {headings.map((h) => (
        <li key={h.slug} data-level={h.level}>
          <a
            href={`#${h.slug}`}
            onClick={(e) => {
              e.preventDefault();
              jumpToHeading(h.slug, true);
            }}
          >
            {h.text}
          </a>
        </li>
      ))}
    </List>
  );
}

function SearchBox({ settings }: { settings: Settings }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const pageId = usePageId();
  const page = useQuery({
    queryKey: ['block-search-page', pageId],
    queryFn: () => api.get<{ space_id: string }>(`/pages/${pageId}`),
    enabled: settings.this_space === true && !!pageId,
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const address = new URLSearchParams({ q: query });
    if (page.data) address.set('space_id', page.data.space_id);
    if (typeof settings.below === 'string') address.set('below', settings.below);
    navigate(`/search?${address.toString()}`);
  };
  const hint = typeof settings.placeholder === 'string' ? settings.placeholder : t('blocks.searchHint');
  return (
    <form role="search" onSubmit={submit}>
      <input
        type="search"
        aria-label={hint}
        placeholder={hint}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
    </form>
  );
}

function Local({ kind, settings, content }: { kind: string; settings: Settings; content: Json }) {
  const { t } = useTranslation();
  if (kind === 'toc') return <Contents document={content} settings={settings} />;
  if (kind === 'search_box') return <SearchBox settings={settings} />;
  return (
    <a
      href="#main"
      onClick={(e) => {
        e.preventDefault();
        window.scrollTo(0, 0);
      }}
    >
      {t('blocks.backToTop')}
    </a>
  );
}

/** One failing block must leave the rest of the page working (BLK-008). */
class BlockBoundary extends Component<{ message: string; children: ReactNode }, { failed: boolean }> {
  override state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  override render() {
    return this.state.failed ? <p className="muted">{this.props.message}</p> : this.props.children;
  }
}

function FieldInput({
  field,
  value,
  onChange,
}: {
  field: SettingField;
  value: unknown;
  onChange: (next: unknown) => void;
}) {
  const { t } = useTranslation();
  const label = t(`blocks.field_${field.key}`);
  if (field.type === 'switch') {
    return (
      <label className="check">
        <input type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} />
        {label}
      </label>
    );
  }
  if (field.type === 'sort' || field.type === 'style') {
    const options = field.type === 'sort' ? ['tree', 'title', 'created', 'changed'] : ['list', 'cards'];
    return (
      <Field label={label}>
        <select value={typeof value === 'string' ? value : ''} onChange={(e) => onChange(e.target.value)}>
          <option value="">{t('common.none')}</option>
          {options.map((option) => (
            <option key={option} value={option}>
              {t(`blocks.option_${option}`)}
            </option>
          ))}
        </select>
      </Field>
    );
  }
  if (field.type === 'number') {
    return (
      <Field label={label}>
        <input
          type="number"
          min={1}
          value={typeof value === 'number' ? value : ''}
          onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
        />
      </Field>
    );
  }
  if (field.type === 'pages') {
    return (
      <Field label={label} help={t('blocks.pagesHelp')}>
        <input
          defaultValue={Array.isArray(value) ? value.join(', ') : ''}
          onChange={(e) => onChange(listFromText(e.target.value).map((part) => pageOrText(field, part)))}
        />
      </Field>
    );
  }
  if (field.type === 'list') {
    return (
      <Field label={label} help={t('blocks.listHelp')}>
        <input
          defaultValue={Array.isArray(value) ? value.join(', ') : ''}
          onChange={(e) => onChange(listFromText(e.target.value))}
        />
      </Field>
    );
  }
  return (
    <Field label={label} help={field.type === 'page' ? t('blocks.pageHelp') : undefined}>
      <input
        value={typeof value === 'string' ? value : ''}
        onChange={(e) => onChange(pageOrText(field, e.target.value))}
      />
    </Field>
  );
}

const PAGE_ADDRESS = /\/pages\/([0-9a-f-]{36})/i;

/** An address pasted into a page field is as good as an id. */
function pageOrText(field: SettingField, value: string): string {
  return field.type === 'page' || field.type === 'pages'
    ? (PAGE_ADDRESS.exec(value)?.[1] ?? value.trim())
    : value;
}

function cleaned(settings: Settings): Settings {
  return Object.fromEntries(
    Object.entries(settings).filter(
      ([, v]) => v !== undefined && v !== '' && v !== false && !(Array.isArray(v) && v.length === 0),
    ),
  );
}

function SettingsDialog({
  kind,
  settings,
  onChange,
  onClose,
}: {
  kind: string;
  settings: Settings;
  onChange: (next: Settings) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState(settings);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const change = (key: string, value: unknown) => {
    const next = { ...form, [key]: value };
    setForm(next);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => onChange(cleaned(next)), TYPING_PAUSE_MS);
  };
  return (
    <Dialog title={t(`insert.block_${kind}.name`)} onClose={onClose}>
      {(DYNAMIC_KINDS[kind] ?? []).map((field) => (
        <FieldInput
          key={field.key}
          field={field}
          value={form[field.key]}
          onChange={(value) => change(field.key, value)}
        />
      ))}
      {(DYNAMIC_KINDS[kind] ?? []).length === 0 && <p className="muted">{t('blocks.noSettings')}</p>}
    </Dialog>
  );
}

function DynamicView({ node, editor, updateAttributes, selected }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const { kind, settings: stored } = node.attrs as { kind: string; settings: Settings | null };
  const settings = stored ?? {};
  const [open, setOpen] = useState(false);
  const known = kind in DYNAMIC_KINDS;
  const name = known ? t(`insert.block_${kind}.name`) : t('blocks.unknown');
  return (
    <NodeViewWrapper
      as="section"
      className={selected ? 'dynamic selected' : 'dynamic'}
      aria-label={name}
      data-kind={kind}
      contentEditable={false}
    >
      {editor.isEditable && (
        <div className="dynamic-head">
          <span>{name}</span>
          <button type="button" className="ghost" onClick={() => setOpen(true)}>
            <IconSliders size={SETTINGS_ICON_SIZE} /> {t('blocks.settings')}
          </button>
        </div>
      )}
      <BlockBoundary message={t('blocks.failed')}>
        {!known && <p className="muted">{t('blocks.unknown')}</p>}
        {known && LOCAL_KINDS.has(kind) && (
          <Local kind={kind} settings={settings} content={editor.getJSON() as Json} />
        )}
        {known && !LOCAL_KINDS.has(kind) && <Asked kind={kind} settings={settings} />}
      </BlockBoundary>
      {open && (
        <SettingsDialog
          kind={kind}
          settings={settings}
          onChange={(next) => updateAttributes({ settings: next })}
          onClose={() => setOpen(false)}
        />
      )}
    </NodeViewWrapper>
  );
}

const jsonAttribute = (name: string) => ({
  default: null,
  parseHTML: (el: HTMLElement) => JSON.parse(el.getAttribute(`data-${name}`) ?? 'null') as unknown,
  renderHTML: (attrs: Record<string, unknown>) =>
    attrs[name] ? { [`data-${name}`]: JSON.stringify(attrs[name]) } : {},
});

export const Dynamic = DocumentNode.create({
  name: 'dynamic',
  group: 'block',
  atom: true,
  draggable: true,
  selectable: true,
  addAttributes() {
    return {
      kind: {
        default: 'toc',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-dynamic'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-dynamic': attrs.kind }),
      },
      settings: jsonAttribute('settings'),
      width: { default: null, rendered: false },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-dynamic]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', HTMLAttributes];
  },
  addNodeView() {
    return ReactNodeViewRenderer(DynamicView);
  },
});
