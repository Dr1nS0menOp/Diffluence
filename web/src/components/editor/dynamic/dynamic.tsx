// SPDX-License-Identifier: Apache-2.0
// A block that shows something by itself. A document stores settings, and every
// reader asks the server for their own answer (spec 012, E1).
import { Component, useEffect, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Node as DocumentNode } from '@tiptap/core';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import { api, type Json } from '../../../api';
import { match, useLocation } from '../../../router';
import { IconSliders } from '../../icons';
import { Dialog, ErrorNote, Field, Loading } from '../../ui';
import { Answer, type BlockAnswer } from './answers';
import { DYNAMIC_KINDS, linesFromText, listFromText, LOCAL_KINDS, type SettingField } from './kinds';
import { Local } from './local';

const NEAR_SCREEN = '300px';
const TYPING_PAUSE_MS = 300;
const SETTINGS_ICON_SIZE = 16;
const LINES_ROWS = 6;

type Settings = Record<string, unknown>;

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

function Asked({ kind, settings }: { kind: string; settings: Settings }) {
  const { t } = useTranslation();
  const pageId = usePageId();
  const [box, near] = useNearScreen();
  const q = useQuery({
    queryKey: ['block', kind, pageId, settings],
    queryFn: () => api.post<BlockAnswer>(`/blocks/${kind}`, { page_id: pageId ?? null, settings }),
    enabled: near,
  });
  const empty = q.data && q.data.rows.length === 0 && !q.data.document && !q.data.people?.length;
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
      {q.data && !empty && <Answer kind={kind} answer={q.data} settings={settings} />}
    </div>
  );
}

const ACTIVITY_SCOPES = ['all', 'favourites', 'network'];

/** Each tab asks the server again, so each is a list that follows the rules (spec 026, D6). */
function ActivityTabs({ settings }: { settings: Settings }) {
  const { t } = useTranslation();
  const [scope, setScope] = useState('all');
  return (
    <>
      <div className="block-letters" role="group" aria-label={t('blocks.activityScope')}>
        {ACTIVITY_SCOPES.map((item) => (
          <button
            key={item}
            type="button"
            className="ghost"
            aria-pressed={scope === item}
            onClick={() => setScope(item)}
          >
            {t(`blocks.scope_${item}`)}
          </button>
        ))}
      </div>
      <Asked kind="activity_tabs" settings={{ ...settings, scope }} />
    </>
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
  if (field.type === 'sort' || field.type === 'choice') {
    const options = field.options ?? ['tree', 'title', 'created', 'changed'];
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
  if (field.type === 'date') {
    return (
      <Field label={label}>
        <input
          type="date"
          value={typeof value === 'string' ? value : ''}
          onChange={(e) => onChange(e.target.value)}
        />
      </Field>
    );
  }
  if (field.type === 'lines') {
    return (
      <Field label={label} help={t(`blocks.help_${field.key}`)}>
        <textarea
          rows={LINES_ROWS}
          defaultValue={Array.isArray(value) ? value.join('\n') : ''}
          onChange={(e) => onChange(linesFromText(e.target.value))}
        />
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
    <Dialog title={t(`insert.block_${kind}.name`)} onClose={onClose} side>
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
      className={selected && editor.isEditable ? 'dynamic selected' : 'dynamic'}
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
        {known && kind === 'activity_tabs' && <ActivityTabs settings={settings} />}
        {known && !LOCAL_KINDS.has(kind) && kind !== 'activity_tabs' && (
          <Asked kind={kind} settings={settings} />
        )}
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
