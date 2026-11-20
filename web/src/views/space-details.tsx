// SPDX-License-Identifier: Apache-2.0
// Details of a space as one form (spec 083, SPC-091 to SPC-096): the badge, the name, the key with
// its old keys, the owners, the administrators, and a fold with the description, the categories
// and the home page. Save sends the changed parts in order through their own routes (D2).
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Category, type PageRef, type Space, type SpaceOverview } from '../api';
import { Link, navigate } from '../router';
import { Notice } from '../components/notice';
import { SubjectPicker } from '../components/picker';
import { SaveReset } from '../components/save-reset';
import { Avatar, ErrorNote, Field, SpaceBadge } from '../components/ui';
import { pageTitle } from '../page-name';
import {
  changedParts,
  rebased,
  saveInOrder,
  type DetailsPart,
  type DetailsValues,
  type SaveResult,
} from './space-details-save';

/** Up to this many administrators show as pictures (SPC-095). */
const ADMINISTRATORS_SHOWN = 5;

interface Owner {
  id: string;
  name: string;
}

/** What the server holds, as the form compares it. */
function storedValues(space: Space, owners: Owner[], categories: string[] | null): DetailsValues {
  return {
    name: space.name,
    description: space.description,
    owners: owners.map((o) => o.id),
    categories,
    home: space.home_page_id,
    key: space.key,
  };
}

function useDetailsSave(space: Space) {
  const client = useQueryClient();
  const send = (part: DetailsPart, form: DetailsValues) => {
    const base = `/spaces/${space.id}`;
    switch (part) {
      case 'details':
        return api.patch(base, { name: form.name, description: form.description, icon: space.icon });
      case 'owners':
        return api.put(`${base}/owners`, { user_ids: form.owners });
      case 'categories':
        return api.put(`${base}/categories`, { categories: form.categories });
      case 'home':
        return api.put(`${base}/home`, { page_id: form.home });
      case 'key':
        return api.put(`${base}/key`, { key: form.key });
    }
  };
  return useMutation({
    mutationFn: async ({ stored, form }: { stored: DetailsValues; form: DetailsValues }) =>
      saveInOrder(changedParts(stored, form), async (part) => {
        await send(part, form);
      }),
    onSuccess: async (result: SaveResult, { form }) => {
      await client.invalidateQueries({ queryKey: ['overview', space.id] });
      await client.invalidateQueries({ queryKey: ['space-categories', space.id] });
      await client.invalidateQueries({ queryKey: ['spaces'] });
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
      await client.invalidateQueries({ queryKey: ['space-id', space.id] });
      // The address follows the key, so a new key opens Details at its new address.
      if (result.saved.includes('key')) {
        navigate(`/spaces/${form.key}/settings?section=details`, true);
        return;
      }
      await client.invalidateQueries({ queryKey: ['space', space.key] });
    },
  });
}

export function DetailsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const mayAdminister = space.can?.['space.administer'] ?? false;
  const overview = useQuery({
    queryKey: ['overview', space.id],
    queryFn: () => api.get<SpaceOverview>(`/spaces/${space.id}/overview`),
  });
  // Only an administrator changes the categories; one who may only set the look sees the fold open.
  const [foldOpen, setFoldOpen] = useState(!mayAdminister);
  const chosen = useQuery({
    queryKey: ['space-categories', space.id],
    queryFn: () => api.get<{ categories: string[] }>(`/spaces/${space.id}/categories`),
    enabled: foldOpen && mayAdminister,
  });
  const owners = overview.data?.owners ?? [];
  const [known, setKnown] = useState<Owner[]>([]);
  const stored = storedValues(space, owners, chosen.data?.categories ?? null);
  const [form, setForm] = useState<DetailsValues>(stored);
  const storedText = JSON.stringify(stored);
  const previous = useRef(storedText);
  // A new answer from the server moves the fields nobody touched, and keeps what was typed.
  useEffect(() => {
    const before = JSON.parse(previous.current) as DetailsValues;
    previous.current = storedText;
    setForm((typed) => rebased(before, JSON.parse(storedText) as DetailsValues, typed));
  }, [storedText]);
  const save = useDetailsSave(space);
  const changed = changedParts(stored, form).length > 0;
  const nameOf = (id: string) => [...owners, ...known].find((o) => o.id === id)?.name ?? id;
  return (
    <form
      className="space-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate({ stored, form });
      }}
    >
      <SpaceBadge space={space} size="large" />
      <SaveOutcome result={save.data} error={save.error} />
      {mayAdminister && (
        <>
          <Field label={t('common.name')}>
            <input value={form.name} required onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </Field>
          <Field label={t('spaceDetails.key')} help={t('spaceSettings.keyHelp')}>
            <input value={form.key} required onChange={(e) => setForm({ ...form, key: e.target.value })} />
          </Field>
          <OldKeys space={space} />
          <fieldset className="plain-fieldset">
            <legend className="field-legend">{t('spaceDetails.owners')}</legend>
            <small className="muted">{t('spaceDetails.ownersHelp')}</small>
            {form.owners.length > 0 && (
              <ul className="plain-list">
                {form.owners.map((id) => (
                  <li key={id} className="row">
                    <Avatar name={nameOf(id)} size="small" />
                    {nameOf(id)}
                    {/* A space keeps at least one owner (SPC-094). */}
                    {form.owners.length > 1 && (
                      <button
                        type="button"
                        className="ghost"
                        aria-label={t('spaceDetails.removeOwner', { name: nameOf(id) })}
                        onClick={() => setForm({ ...form, owners: form.owners.filter((x) => x !== id) })}
                      >
                        {t('common.remove')}
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            )}
            <SubjectPicker
              label={t('spaceSettings.addOwner')}
              usersOnly
              onPick={(picked) => {
                setKnown([...known, { id: picked.id, name: picked.name }]);
                if (!form.owners.includes(picked.id))
                  setForm({ ...form, owners: [...form.owners, picked.id] });
              }}
            />
          </fieldset>
          <Administrators space={space} names={overview.data?.administrators ?? []} />
        </>
      )}
      <details className="fold" open={foldOpen} onToggle={(e) => setFoldOpen(e.currentTarget.open)}>
        <summary>{t('spaceDetails.more')}</summary>
        {/* The fields stay in the form while it is shut, so typing kept over a reload finds them (spec 043). */}
        <MoreDetails space={space} form={form} onChange={setForm} open={foldOpen} />
      </details>
      <SaveReset changed={changed} busy={save.isPending} onReset={() => setForm(stored)} />
    </form>
  );
}

/** Says which part a Save stopped at, and which parts it saved before it (D2). */
function SaveOutcome({ result, error }: { result: SaveResult | undefined; error: Error | null }) {
  const { t } = useTranslation();
  if (error) return <ErrorNote error={error} />;
  if (!result?.refused) return null;
  const partName = (part: DetailsPart) => t(`spaceDetails.part_${part}`);
  return (
    <Notice kind="error">
      {t('spaceDetails.refused', { part: partName(result.refused.part) })}{' '}
      {result.saved.length > 0 &&
        t('spaceDetails.savedBefore', { parts: result.saved.map(partName).join(', ') })}
      <ErrorNote error={result.refused.error} />
    </Notice>
  );
}

/** The old keys of the space, each freed at once (SPC-093). */
function OldKeys({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const old = useQuery({
    queryKey: ['old-keys', space.id],
    queryFn: () => api.get<{ keys: string[] }>(`/spaces/${space.id}/old-keys`),
  });
  const release = useMutation({
    mutationFn: (alias: string) => api.del(`/spaces/${space.id}/old-keys/${encodeURIComponent(alias)}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['old-keys', space.id] }),
  });
  const keys = old.data?.keys ?? [];
  if (keys.length === 0) return <ErrorNote error={old.error} />;
  return (
    <ul className="plain-list">
      <ErrorNote error={release.error} />
      {keys.map((alias) => (
        <li key={alias} className="row">
          <span className="muted">{t('spaceDetails.oldKey', { key: alias })}</span>
          <button type="button" className="quiet" onClick={() => release.mutate(alias)}>
            {t('spaceDetails.free')}
          </button>
        </li>
      ))}
    </ul>
  );
}

/** The administrators as overlapping pictures, read only, with a way to all of them (SPC-095). */
function Administrators({ space, names }: { space: Space; names: string[] }) {
  const { t } = useTranslation();
  if (names.length === 0) return null;
  return (
    <div className="field">
      <span>{t('spaceDetails.administrators')}</span>
      <div className="row">
        <span className="avatar-stack" title={names.join(', ')}>
          {names.slice(0, ADMINISTRATORS_SHOWN).map((name) => (
            <Avatar key={name} name={name} size="small" />
          ))}
        </span>
        {space.can?.['space.manage_access'] && (
          <Link href={`/spaces/${space.key}/settings?section=people`}>{t('spaceDetails.seeAll')}</Link>
        )}
      </div>
    </div>
  );
}

/** The fold: description and categories for an administrator, the home page for the look (SPC-096). */
function MoreDetails({
  space,
  form,
  onChange,
  open,
}: {
  space: Space;
  form: DetailsValues;
  onChange: (form: DetailsValues) => void;
  /** Whether the fold is open: the lists behind it load only then. */
  open: boolean;
}) {
  const { t } = useTranslation();
  const mayAdminister = space.can?.['space.administer'] ?? false;
  const mayLook = space.can?.['space.look'] ?? false;
  const all = useQuery({
    queryKey: ['categories'],
    queryFn: () => api.get<{ categories: Category[] }>('/categories'),
    enabled: open && mayAdminister,
  });
  const pages = useQuery({
    queryKey: ['tree-pages', space.id],
    queryFn: () => api.get<{ pages: PageRef[] }>(`/spaces/${space.id}/tree`),
    enabled: open && mayLook,
  });
  const categories = all.data?.categories ?? [];
  const picked = form.categories ?? [];
  return (
    <>
      {mayAdminister && (
        <Field label={t('space.description')}>
          <textarea
            value={form.description}
            rows={2}
            onChange={(e) => onChange({ ...form, description: e.target.value })}
          />
        </Field>
      )}
      {mayAdminister && categories.length > 0 && (
        <fieldset className="plain-fieldset">
          <legend className="field-legend">{t('spaceDetails.categories')}</legend>
          {categories.map((c) => (
            <label key={c.id} className="check">
              <input
                type="checkbox"
                checked={picked.includes(c.id)}
                disabled={form.categories === null}
                onChange={(e) =>
                  onChange({
                    ...form,
                    categories: e.target.checked ? [...picked, c.id] : picked.filter((x) => x !== c.id),
                  })
                }
              />
              {c.name}
            </label>
          ))}
        </fieldset>
      )}
      {mayLook && (
        <Field label={t('spaceSettings.home')}>
          <select value={form.home ?? ''} onChange={(e) => onChange({ ...form, home: e.target.value })}>
            {form.home === null && <option value="" />}
            {pages.data?.pages.map((p) => (
              <option key={p.id} value={p.id}>
                {pageTitle(t, p.title)}
              </option>
            ))}
          </select>
        </Field>
      )}
      <ErrorNote error={all.error ?? pages.error} />
    </>
  );
}
