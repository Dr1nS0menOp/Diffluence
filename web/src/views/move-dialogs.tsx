// SPDX-License-Identifier: Apache-2.0
// Moving and copying a page: within a space, or to another one (spec 011, D8).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type BulkFailure,
  type CopyImpact,
  type MoveImpact,
  type Page,
  type PageSummary,
  type Space,
  type TreeNode,
} from '../api';
import { navigate } from '../router';
import { useToast } from '../components/toast';
import { Dialog, ErrorNote, Field } from '../components/ui';

const TITLES_IN_PREVIEW = 5;
const MAX_MATCHES = 20;

export interface TitleChange {
  prefix: string;
  suffix: string;
  find: string;
  replace: string;
}

/** What a copied title becomes. The server does the same when it copies. */
export function changedTitle(title: string, change: TitleChange): string {
  const found = change.find ? title.split(change.find).join(change.replace) : title;
  return `${change.prefix}${found}${change.suffix}`;
}

function useSpaces() {
  return useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
}

function useTargets(spaceId: string) {
  return useQuery({
    queryKey: ['tree', spaceId],
    queryFn: () => api.get<{ pages: TreeNode[] }>(`/spaces/${spaceId}/tree`),
    enabled: spaceId !== '',
  });
}

function SpaceField({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const { t } = useTranslation();
  const spaces = useSpaces();
  return (
    <Field label={t('move.space')}>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {spaces.data?.spaces.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}
          </option>
        ))}
      </select>
    </Field>
  );
}

function ParentByTree({
  targets,
  value,
  onChange,
}: {
  targets: TreeNode[];
  value: string;
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Field label={t('page.moveTo')}>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">{t('page.topLevel')}</option>
        {targets.map((n) => (
          <option key={n.id} value={n.id}>
            {n.title}
          </option>
        ))}
      </select>
    </Field>
  );
}

function ParentBySearch({
  targets,
  value,
  onChange,
}: {
  targets: TreeNode[];
  value: string;
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const wanted = text.trim().toLowerCase();
  const found = targets.filter((n) => n.title.toLowerCase().includes(wanted)).slice(0, MAX_MATCHES);
  return (
    <>
      <input
        type="search"
        className="dialog-filter"
        value={text}
        aria-label={t('move.findParent')}
        placeholder={t('move.findParent')}
        onChange={(e) => setText(e.target.value)}
      />
      <fieldset className="pick-list">
        <legend className="visually-hidden">{t('page.moveTo')}</legend>
        {found.map((n) => (
          <label key={n.id} className="check">
            <input type="radio" name="parent" checked={value === n.id} onChange={() => onChange(n.id)} />
            {n.title}
          </label>
        ))}
      </fieldset>
    </>
  );
}

/** Tree and Search are two ways to name the same parent. */
function ParentField({
  targets,
  value,
  onChange,
}: {
  targets: TreeNode[];
  value: string;
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<'tree' | 'search'>('tree');
  return (
    <>
      <div role="tablist" aria-label={t('page.moveTo')} className="tabs">
        {(['tree', 'search'] as const).map((name) => (
          <button
            key={name}
            type="button"
            role="tab"
            aria-selected={tab === name}
            onClick={() => setTab(name)}
          >
            {t(`move.tab_${name}`)}
          </button>
        ))}
      </div>
      <div role="tabpanel" aria-label={t(`move.tab_${tab}`)}>
        {tab === 'tree' ? (
          <ParentByTree targets={targets} value={value} onChange={onChange} />
        ) : (
          <ParentBySearch targets={targets} value={value} onChange={onChange} />
        )}
      </div>
    </>
  );
}

function ImpactNote({ impact }: { impact: MoveImpact }) {
  const { t } = useTranslation();
  return (
    <div role="status" className="note">
      <p>{t('move.pages', { count: impact.pages })}</p>
      {impact.hidden && <p>{t('move.hidden')}</p>}
      {impact.other_space && <p>{t('move.otherSpace')}</p>}
      {impact.restrictions_lost.length > 0 && (
        <p>{t('move.lost', { titles: impact.restrictions_lost.join(', ') })}</p>
      )}
      {impact.restrictions_gained.length > 0 && (
        <p>{t('move.gained', { titles: impact.restrictions_gained.join(', ') })}</p>
      )}
      {impact.needs_manage && <p>{t('move.needsManage')}</p>}
      {!impact.fits && <p>{t('move.noRoom')}</p>}
    </div>
  );
}

export function MoveDialog({
  page,
  toSpace,
  onClose,
}: {
  page: PageSummary;
  toSpace?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [spaceId, setSpaceId] = useState(toSpace ?? page.space_id);
  const [parent, setParent] = useState(toSpace ? '' : (page.parent_id ?? ''));
  const tree = useTargets(spaceId);
  const targets = (tree.data?.pages ?? []).filter((n) => n.id !== page.id && n.status === 'published');
  const impact = useQuery({
    queryKey: ['move-impact', page.id, spaceId, parent],
    queryFn: () => {
      const params = new URLSearchParams({ space_id: spaceId });
      if (parent) params.set('parent_id', parent);
      return api.get<MoveImpact>(`/pages/${page.id}/move-impact?${params.toString()}`);
    },
    staleTime: 0,
  });
  const move = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/move`, { parent_id: parent || null, space_id: spaceId }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
      await client.invalidateQueries({ queryKey: ['tree', spaceId] });
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      onClose();
    },
  });
  return (
    <Dialog title={t('page.move')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          move.mutate();
        }}
      >
        <ErrorNote error={move.error ?? impact.error ?? tree.error} />
        <SpaceField
          value={spaceId}
          onChange={(id) => {
            setSpaceId(id);
            setParent('');
          }}
        />
        <ParentField targets={targets} value={parent} onChange={setParent} />
        {impact.data && <ImpactNote impact={impact.data} />}
        <div className="dialog-actions">
          <button
            type="submit"
            className="primary"
            disabled={(impact.data?.hidden ?? false) || impact.data?.fits === false}
          >
            {t('page.move')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** Moves several rows at once. Each one is checked on its own, and the rest go on. */
export function BulkMoveDialog({
  ids,
  spaceId: startSpace,
  onClose,
  onDone,
}: {
  ids: string[];
  spaceId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const [spaceId, setSpaceId] = useState(startSpace);
  const [parent, setParent] = useState('');
  const tree = useTargets(spaceId);
  const targets = (tree.data?.pages ?? []).filter((n) => !ids.includes(n.id) && n.status === 'published');
  const move = useMutation({
    mutationFn: () =>
      api.post<{ moved: number; failed: BulkFailure[] }>('/pages/bulk-move', {
        page_ids: ids,
        space_id: spaceId,
        parent_id: parent || null,
      }),
    onSuccess: async (r) => {
      await client.invalidateQueries({ queryKey: ['tree', startSpace] });
      await client.invalidateQueries({ queryKey: ['tree', spaceId] });
      toast({ message: t('move.report', { moved: r.moved, failed: r.failed.length }) });
      onDone();
      onClose();
    },
  });
  return (
    <Dialog title={t('move.several', { count: ids.length })} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          move.mutate();
        }}
      >
        <ErrorNote error={move.error ?? tree.error} />
        <SpaceField value={spaceId} onChange={setSpaceId} />
        <ParentByTree targets={targets} value={parent} onChange={setParent} />
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={move.isPending}>
            {t('page.move')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function TitleFields({ change, onChange }: { change: TitleChange; onChange: (next: TitleChange) => void }) {
  const { t } = useTranslation();
  const set = (key: keyof TitleChange) => (e: { target: { value: string } }) =>
    onChange({ ...change, [key]: e.target.value });
  return (
    <fieldset className="title-change">
      <legend>{t('copy.titles')}</legend>
      <Field label={t('copy.prefix')}>
        <input value={change.prefix} onChange={set('prefix')} />
      </Field>
      <Field label={t('copy.suffix')}>
        <input value={change.suffix} onChange={set('suffix')} />
      </Field>
      <Field label={t('copy.find')}>
        <input value={change.find} onChange={set('find')} />
      </Field>
      <Field label={t('copy.replace')}>
        <input value={change.replace} onChange={set('replace')} />
      </Field>
    </fieldset>
  );
}

export function CopyDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const [spaceId, setSpaceId] = useState(page.space_id);
  const [parent, setParent] = useState(page.parent_id ?? '');
  const [children, setChildren] = useState(false);
  const [keepLabels, setKeepLabels] = useState(true);
  const [keepRestrictions, setKeepRestrictions] = useState(true);
  const [change, setChange] = useState<TitleChange>({ prefix: '', suffix: '', find: '', replace: '' });
  const target = useTargets(spaceId);
  const source = useTargets(page.space_id);
  const targets = (target.data?.pages ?? []).filter((n) => n.status === 'published');
  const below = (source.data?.pages ?? []).filter((n) => n.parent_id === page.id).map((n) => n.title);
  const titles = (children ? [page.title, ...below] : [page.title]).slice(0, TITLES_IN_PREVIEW);
  const impact = useQuery({
    queryKey: ['copy-impact', page.id, spaceId, parent],
    queryFn: () => {
      const params = new URLSearchParams({ space_id: spaceId });
      if (parent) params.set('parent_id', parent);
      return api.get<CopyImpact>(`/pages/${page.id}/copy-impact?${params.toString()}`);
    },
    staleTime: 0,
  });
  const lost = impact.data?.restrictions_lost ?? [];
  const refused = impact.data?.needs_manage ?? false;
  const copy = useMutation({
    mutationFn: () =>
      api.post<Page & { job_id?: string }>(`/pages/${page.id}/copy`, {
        parent_id: parent || null,
        space_id: spaceId,
        include_children: children,
        keep_labels: keepLabels,
        keep_restrictions: keepRestrictions,
        ...change,
      }),
    onSuccess: async (p) => {
      await client.invalidateQueries({ queryKey: ['tree', spaceId] });
      if (p.job_id) {
        toast({ message: t('copy.running') });
        onClose();
        return;
      }
      navigate(`/pages/${p.id}`);
    },
  });
  return (
    <Dialog title={t('page.copy')} onClose={onClose} wide>
      <ErrorNote error={copy.error ?? target.error ?? impact.error} />
      <SpaceField
        value={spaceId}
        onChange={(id) => {
          setSpaceId(id);
          setParent('');
        }}
      />
      <ParentByTree targets={targets} value={parent} onChange={setParent} />
      {lost.length > 0 && (
        <div role="status" className="note">
          <p>{t('copy.lost', { titles: lost.join(', ') })}</p>
          {refused && <p>{t('move.needsManage')}</p>}
        </div>
      )}
      {impact.data?.fits === false && (
        <p role="status" className="note">
          {t('copy.noRoom')}
        </p>
      )}
      <label className="check">
        <input type="checkbox" checked={children} onChange={(e) => setChildren(e.target.checked)} />
        {t('page.copyChildren')}
      </label>
      <label className="check">
        <input type="checkbox" checked={keepLabels} onChange={(e) => setKeepLabels(e.target.checked)} />
        {t('copy.keepLabels')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={keepRestrictions}
          onChange={(e) => setKeepRestrictions(e.target.checked)}
        />
        {t('copy.keepRestrictions')}
      </label>
      <TitleFields change={change} onChange={setChange} />
      <ul className="plain-list muted" aria-label={t('copy.preview')}>
        {titles.map((title) => (
          <li key={title}>{changedTitle(title, change)}</li>
        ))}
      </ul>
      <div className="dialog-actions">
        <button
          type="button"
          className="primary"
          disabled={copy.isPending || refused}
          onClick={() => copy.mutate()}
        >
          {t('page.copy')}
        </button>
      </div>
    </Dialog>
  );
}
