// SPDX-License-Identifier: Apache-2.0
// The parts of Space settings that shape how a space is organised.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type DirectoryEntry,
  type LabelCount,
  type PageRef,
  type Space,
  type SpaceOverview,
  type Stage,
} from '../api';
import { Link, navigate } from '../router';
import { SubjectPicker } from '../components/picker';
import { Dialog, ErrorNote, Field, FilePicker } from '../components/ui';

const FEATURES = [
  'blog',
  'comments',
  'reactions',
  'access_requests',
  'stages',
  'analytics',
  'automation',
  'calendar',
];
const MARK_COLOURS = ['grey', 'yellow', 'green', 'blue', 'purple', 'red'];

function useSpaceRefresh(space: Space) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['space', space.key] });
    await client.invalidateQueries({ queryKey: ['space-id', space.id] });
    await client.invalidateQueries({ queryKey: ['overview', space.id] });
  };
}

export function SpaceLinksSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  return (
    <section>
      <h2>{t('spaceSettings.keepTidy')}</h2>
      <ul className="plain-list">
        <li>
          <Link href={`/spaces/${space.key}/content`}>{t('content.title')}</Link>
        </li>
        <li>
          <Link href={`/spaces/${space.key}/templates`}>{t('templates.title')}</Link>
        </li>
        <li>
          <Link href={`/spaces/${space.key}/archive`}>{t('organise.archive')}</Link>
        </li>
        <li>
          <Link href={`/spaces/${space.key}/labels`}>{t('organise.labels')}</Link>
        </li>
        <li>
          <Link href={`/spaces/${space.key}/insights`}>{t('insights.spaceTitle')}</Link>
        </li>
        <li>
          <Link href={`/spaces/${space.key}/settings/automation`}>{t('rules2.title')}</Link>
        </li>
        <li>
          <Link href={`/spaces/${space.key}/settings/webhooks`}>{t('webhooks.title')}</Link>
        </li>
      </ul>
    </section>
  );
}

export function KeySection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [key, setKey] = useState(space.key);
  const save = useMutation({
    mutationFn: () => api.put<Space>(`/spaces/${space.id}/key`, { key }),
    onSuccess: async (s) => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      navigate(`/spaces/${s.key}/settings`, true);
    },
  });
  const old = useQuery({
    queryKey: ['old-keys', space.id],
    queryFn: () => api.get<{ keys: string[] }>(`/spaces/${space.id}/old-keys`),
  });
  const release = useMutation({
    mutationFn: (alias: string) => api.del(`/spaces/${space.id}/old-keys/${encodeURIComponent(alias)}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['old-keys', space.id] }),
  });
  return (
    <section>
      <h2>{t('spaceSettings.key')}</h2>
      <p className="muted">{t('spaceSettings.keyHelp')}</p>
      <ErrorNote error={save.error ?? release.error} />
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <input value={key} aria-label={t('space.key')} onChange={(e) => setKey(e.target.value)} />
        <button type="submit">{t('common.save')}</button>
      </form>
      <ul className="plain-list">
        {old.data?.keys.map((alias) => (
          <li key={alias} className="row">
            {alias}
            <button type="button" className="ghost" onClick={() => release.mutate(alias)}>
              {t('spaceSettings.release')}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function PeopleSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const overview = useQuery({
    queryKey: ['overview', space.id],
    queryFn: () => api.get<SpaceOverview>(`/spaces/${space.id}/overview`),
  });
  const owners = overview.data?.owners ?? [];
  const save = useMutation({
    mutationFn: (ids: string[]) => api.put(`/spaces/${space.id}/owners`, { user_ids: ids }),
    onSettled: refresh,
  });
  const [home, setHome] = useState('');
  const setHomePage = useMutation({
    mutationFn: () => api.put(`/spaces/${space.id}/home`, { page_id: home }),
    onSuccess: refresh,
  });
  const pages = useQuery({
    queryKey: ['tree-pages', space.id],
    queryFn: () => api.get<{ pages: PageRef[] }>(`/spaces/${space.id}/tree`),
  });
  return (
    <section>
      <h2>{t('spaceSettings.owners')}</h2>
      <p className="muted">{t('spaceSettings.ownersHelp')}</p>
      <ErrorNote error={save.error ?? setHomePage.error} />
      <ul className="plain-list">
        {owners.map((o) => (
          <li key={o.id} className="row">
            {o.name}
            <button
              type="button"
              className="ghost"
              onClick={() => save.mutate(owners.filter((x) => x.id !== o.id).map((x) => x.id))}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <SubjectPicker
        label={t('spaceSettings.addOwner')}
        usersOnly
        onPick={(e: DirectoryEntry) => save.mutate([...owners.map((o) => o.id), e.id])}
      />
      <h2>{t('spaceSettings.home')}</h2>
      <div className="row">
        <select
          aria-label={t('spaceSettings.home')}
          value={home || (space.home_page_id ?? '')}
          onChange={(e) => setHome(e.target.value)}
        >
          {pages.data?.pages.map((p) => (
            <option key={p.id} value={p.id}>
              {p.title}
            </option>
          ))}
        </select>
        <button type="button" disabled={!home} onClick={() => setHomePage.mutate()}>
          {t('common.save')}
        </button>
      </div>
    </section>
  );
}

export function FeaturesSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const save = useMutation({
    mutationFn: (values: Record<string, boolean>) => api.put(`/spaces/${space.id}/features`, values),
    onSettled: refresh,
  });
  return (
    <section>
      <h2>{t('spaceSettings.features')}</h2>
      <p className="muted">{t('spaceSettings.featuresHelp')}</p>
      <ErrorNote error={save.error} />
      {FEATURES.map((f) => (
        <label key={f} className="check">
          <input
            type="checkbox"
            checked={space.features?.[f] ?? true}
            onChange={(e) => save.mutate({ [f]: e.target.checked })}
          />
          {t(`spaceSettings.feature_${f}`)}
        </label>
      ))}
    </section>
  );
}

export function StatusesSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['statuses', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ statuses: Stage[]; free: boolean }>(`/spaces/${space.id}/statuses`),
  });
  const save = useMutation({
    mutationFn: (v: { statuses: Stage[]; free: boolean }) => api.put(`/spaces/${space.id}/statuses`, v),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const [word, setWord] = useState('');
  const [color, setColor] = useState('blue');
  const list = q.data?.statuses ?? [];
  const free = q.data?.free ?? false;
  return (
    <section>
      <h2>{t('spaceSettings.statuses')}</h2>
      <ErrorNote error={q.error ?? save.error} />
      <ul className="plain-list">
        {list.map((s) => (
          <li key={s.text} className="row">
            <span className={`status-mark mark-${s.color}`}>{s.text}</span>
            <button
              type="button"
              className="ghost"
              onClick={() => save.mutate({ statuses: list.filter((x) => x.text !== s.text), free })}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate({ statuses: [...list, { text: word, color }], free });
          setWord('');
        }}
      >
        <input
          required
          maxLength={30}
          value={word}
          aria-label={t('organise.statusWord')}
          placeholder={t('organise.statusWord')}
          onChange={(e) => setWord(e.target.value)}
        />
        <select
          aria-label={t('spaceSettings.colour')}
          value={color}
          onChange={(e) => setColor(e.target.value)}
        >
          {MARK_COLOURS.map((c) => (
            <option key={c} value={c}>
              {t(`editor.colors.${c}`)}
            </option>
          ))}
        </select>
        <button type="submit">{t('common.add')}</button>
      </form>
      <label className="check">
        <input
          type="checkbox"
          checked={free}
          onChange={(e) => save.mutate({ statuses: list, free: e.target.checked })}
        />
        {t('spaceSettings.freeStatus')}
      </label>
    </section>
  );
}

/** A fixed list, and renaming, which merges when the new name already exists. */
export function LabelsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['space-labels', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () =>
      api.get<{ labels: LabelCount[]; fixed: boolean; fixed_labels: string[] }>(`/spaces/${space.id}/labels`),
  });
  const refresh = () => client.invalidateQueries({ queryKey: key });
  const fixed = useMutation({
    mutationFn: (v: { fixed: boolean; labels: string[] }) => api.put(`/spaces/${space.id}/labels`, v),
    onSettled: refresh,
  });
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const rename = useMutation({
    mutationFn: () => api.post(`/spaces/${space.id}/labels/rename`, { from, to }),
    onSuccess: () => {
      setFrom('');
      setTo('');
    },
    onSettled: refresh,
  });
  const used = q.data?.labels ?? [];
  const list = q.data?.fixed_labels ?? [];
  return (
    <section>
      <h2>{t('organise.labels')}</h2>
      <ErrorNote error={q.error ?? fixed.error ?? rename.error} />
      <label className="check">
        <input
          type="checkbox"
          checked={q.data?.fixed ?? false}
          onChange={(e) =>
            fixed.mutate({
              fixed: e.target.checked,
              labels: list.length > 0 ? list : used.map((l) => l.label),
            })
          }
        />
        {t('spaceSettings.fixedLabels')}
      </label>
      {q.data?.fixed && <p className="muted">{list.join(', ')}</p>}
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          rename.mutate();
        }}
      >
        <select
          required
          aria-label={t('spaceSettings.renameFrom')}
          value={from}
          onChange={(e) => setFrom(e.target.value)}
        >
          <option value="">{t('spaceSettings.renameFrom')}</option>
          {used.map((l) => (
            <option key={l.label} value={l.label}>
              {l.label}
            </option>
          ))}
        </select>
        <input
          required
          value={to}
          aria-label={t('spaceSettings.renameTo')}
          placeholder={t('spaceSettings.renameTo')}
          onChange={(e) => setTo(e.target.value)}
        />
        <button type="submit">{t('spaceSettings.rename')}</button>
      </form>
    </section>
  );
}

export function RulesSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceRefresh(space);
  const [months, setMonths] = useState(space.review_months ?? 0);
  const review = useMutation({
    mutationFn: () => api.put(`/spaces/${space.id}/review`, { months }),
    onSuccess: refresh,
  });
  const client = useQueryClient();
  const ruleKey = ['approval-rule', space.id];
  const rule = useQuery({
    queryKey: ruleKey,
    queryFn: () =>
      api.get<{ required: boolean; approvers: { id: string; name: string }[] }>(
        `/spaces/${space.id}/approvals`,
      ),
  });
  const required = rule.data?.required ?? false;
  const approvers = rule.data?.approvers ?? [];
  const approval = useMutation({
    mutationFn: (v: { required: boolean; approvers: string[] }) =>
      api.put(`/spaces/${space.id}/approvals`, v),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ruleKey });
      await refresh();
    },
  });
  const approverIds = approvers.map((a) => a.id);
  const align = useMutation({
    mutationFn: (value: string) => api.put(`/spaces/${space.id}/title-align`, { align: value }),
    onSuccess: refresh,
  });
  const upload = useMutation({
    mutationFn: (file: File) => api.upload(`/spaces/${space.id}/icon`, file),
    onSuccess: refresh,
  });
  const removeIcon = useMutation({
    mutationFn: () => api.del(`/spaces/${space.id}/icon`),
    onSuccess: refresh,
  });
  return (
    <section>
      <h2>{t('spaceSettings.rules')}</h2>
      <ErrorNote error={review.error ?? approval.error ?? align.error ?? upload.error ?? removeIcon.error} />
      <Field label={t('spaceSettings.reviewMonths')} help={t('spaceSettings.reviewHelp')}>
        <input
          type="number"
          min={0}
          max={120}
          value={months}
          onChange={(e) => setMonths(Number(e.target.value))}
          onBlur={() => review.mutate()}
        />
      </Field>
      <Field label={t('spaceSettings.titleAlign')}>
        <select value={space.title_align ?? 'start'} onChange={(e) => align.mutate(e.target.value)}>
          <option value="start">{t('look.alignStart')}</option>
          <option value="center">{t('look.alignCenter')}</option>
        </select>
      </Field>
      <label className="check">
        <input
          type="checkbox"
          checked={required}
          onChange={(e) => approval.mutate({ required: e.target.checked, approvers: approverIds })}
        />
        {t('spaceSettings.requireApproval')}
      </label>
      <ul className="plain-list">
        {approvers.map((a) => (
          <li key={a.id} className="row">
            {a.name}
            <button
              type="button"
              className="ghost"
              onClick={() =>
                approval.mutate({ required, approvers: approverIds.filter((id) => id !== a.id) })
              }
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <SubjectPicker
        label={t('spaceSettings.fixedApprover')}
        usersOnly
        onPick={(e) => approval.mutate({ required, approvers: [...approverIds, e.id] })}
      />
      <div className="row">
        <FilePicker
          label={t('spaceSettings.picture')}
          accept="image/png,image/jpeg,image/gif,image/webp"
          onPick={(f) => upload.mutate(f)}
        />
        {space.has_icon && (
          <button type="button" className="quiet" onClick={() => removeIcon.mutate()}>
            {t('spaceSettings.removePicture')}
          </button>
        )}
      </div>
    </section>
  );
}

/** Deleting a space asks for its key, and offers something milder first (SPC-073). */
export function DeleteSpaceButton({ space, onDelete }: { space: Space; onDelete: () => void }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  return (
    <>
      <button type="button" className="danger" onClick={() => setOpen(true)}>
        {t('space.deleteSpace')}
      </button>
      {open && (
        <Dialog title={t('space.deleteSpace')} onClose={() => setOpen(false)}>
          <p>{t('space.confirmDelete')}</p>
          <p className="muted">{t('spaceSettings.deleteInstead')}</p>
          <Field label={t('spaceSettings.typeKey', { key: space.key })}>
            <input value={typed} autoFocus onChange={(e) => setTyped(e.target.value)} />
          </Field>
          <div className="dialog-actions">
            <button type="button" className="quiet" onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </button>
            <button
              type="button"
              className="danger"
              disabled={typed !== space.key}
              onClick={() => {
                setOpen(false);
                onDelete();
              }}
            >
              {t('space.deleteSpace')}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}
