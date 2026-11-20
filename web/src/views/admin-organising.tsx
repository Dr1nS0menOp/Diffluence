// SPDX-License-Identifier: Apache-2.0
// Administration: what holds for all spaces. Company home, old drafts, features, templates.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, NIL_ID, type Space } from '../api';
import { PagePicker } from '../components/picker';
import { ErrorNote, Field, Loading } from '../components/ui';
import { TemplateManager } from './templates';
import { TableBox } from '../components/table-box';
import { differs, SaveReset } from '../components/save-reset';
import { Categories } from './admin-rules';

const PRESETS = ['team_space', 'read_only_kb'];

interface Organising {
  company_home: string;
  draft_days: number;
  default_preset: string;
}

interface InstanceFeatures {
  off: string[];
  new_off: string[];
  all: string[];
}

const ORGANISING_KEY = ['admin-organising'];

/** The organising settings as stored: Organising and Defaults for new spaces each change a part. */
function useOrganising() {
  return useQuery({ queryKey: ORGANISING_KEY, queryFn: () => api.get<Organising>('/admin/organising') });
}

/**
 * Edits a part of the organising settings and saves them whole, as the single screen did before
 * the two were apart (spec 081, D17).
 */
function useOrganisingForm(start: Organising) {
  const client = useQueryClient();
  const [form, setForm] = useState(start);
  const save = useMutation({
    mutationFn: () => api.put('/admin/organising', { ...form, company_home: form.company_home || NIL_ID }),
    onSuccess: () => client.invalidateQueries({ queryKey: ORGANISING_KEY }),
  });
  return { form, setForm, save, changed: differs(start, form), reset: () => setForm(start) };
}

function OrganisingForm({ start }: { start: Organising }) {
  const { t } = useTranslation();
  const { form, setForm, save, changed, reset } = useOrganisingForm(start);
  const home = form.company_home === NIL_ID ? '' : form.company_home;
  return (
    <form
      className="settings-unit"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <PagePicker
        label={t('adminOrganise.companyHome')}
        help={t('adminOrganise.companyHomeHelp')}
        value={home}
        onChange={(id) => setForm({ ...form, company_home: id })}
      />
      <Field label={t('adminOrganise.draftDays')} help={t('adminOrganise.draftDaysHelp')}>
        <input
          type="number"
          min={1}
          value={form.draft_days}
          onChange={(e) => setForm({ ...form, draft_days: Number(e.target.value) })}
        />
      </Field>
      <SaveReset changed={changed} busy={save.isPending} onReset={reset} />
    </form>
  );
}

function SpaceDefaultsForm({ start }: { start: Organising }) {
  const { t } = useTranslation();
  const { form, setForm, save, changed, reset } = useOrganisingForm(start);
  return (
    <form
      className="settings-unit"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <Field label={t('adminOrganise.defaultPreset')} help={t('adminOrganise.defaultPresetHelp')}>
        <select
          value={form.default_preset}
          onChange={(e) => setForm({ ...form, default_preset: e.target.value })}
        >
          <option value="">{t('spaceSettings.preset_private')}</option>
          {PRESETS.map((p) => (
            <option key={p} value={p}>
              {t(`permissions.${p}`)}
            </option>
          ))}
        </select>
      </Field>
      <SaveReset changed={changed} busy={save.isPending} onReset={reset} />
    </form>
  );
}

/** Defaults for new spaces (ADM-073): the permissions a new space starts with. */
export function SpaceDefaultsTab() {
  const settings = useOrganising();
  if (settings.error) return <ErrorNote error={settings.error} />;
  if (!settings.data) return <Loading />;
  return <SpaceDefaultsForm key={JSON.stringify(settings.data)} start={settings.data} />;
}

function FeatureTable({ start }: { start: InstanceFeatures }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const save = useMutation({
    mutationFn: (next: { off: string[]; new_off: string[] }) => api.put('/admin/features', next),
    onSettled: () => client.invalidateQueries({ queryKey: ['admin-features'] }),
  });
  const flip = (list: string[], name: string, on: boolean) =>
    on ? list.filter((x) => x !== name) : [...list, name];
  return (
    <section className="settings-unit">
      <h2>{t('spaceSettings.features')}</h2>
      <ErrorNote error={save.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('adminOrganise.onForAll')}</th>
              <th>{t('adminOrganise.onForNew')}</th>
            </tr>
          </thead>
          <tbody>
            {start.all.map((name) => {
              const label = t(`spaceSettings.feature_${name}`);
              return (
                <tr key={name}>
                  <td>{label}</td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t('adminOrganise.onForAllNamed', { name: label })}
                      checked={!start.off.includes(name)}
                      onChange={(e) =>
                        save.mutate({ ...start, off: flip(start.off, name, e.target.checked) })
                      }
                    />
                  </td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t('adminOrganise.onForNewNamed', { name: label })}
                      checked={!start.new_off.includes(name)}
                      onChange={(e) =>
                        save.mutate({ ...start, new_off: flip(start.new_off, name, e.target.checked) })
                      }
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </TableBox>
    </section>
  );
}

interface ExperimentalParts {
  on: string[];
  all: string[];
}

/** The experimental parts (spec 069): off on a new installation, on here one by one. */
function ExperimentalTable({ start }: { start: ExperimentalParts }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const save = useMutation({
    mutationFn: (on: string[]) => api.put('/admin/experimental', { on }),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['admin-experimental'] });
      await client.invalidateQueries({ queryKey: ['capabilities'] });
    },
  });
  return (
    <section className="settings-unit">
      <h2>{t('experimental.title')}</h2>
      <p className="muted">{t('experimental.help')}</p>
      <ErrorNote error={save.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('experimental.on')}</th>
            </tr>
          </thead>
          <tbody>
            {start.all.map((name) => {
              const label = t(`experimental.part_${name}`);
              return (
                <tr key={name}>
                  <td>{label}</td>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t('experimental.onNamed', { name: label })}
                      checked={start.on.includes(name)}
                      onChange={(e) =>
                        save.mutate(
                          e.target.checked ? [...start.on, name] : start.on.filter((x) => x !== name),
                        )
                      }
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </TableBox>
    </section>
  );
}

function SaveSpaceAsTemplate() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  const [spaceId, setSpaceId] = useState('');
  const [name, setName] = useState('');
  const save = useMutation({
    mutationFn: () => api.post('/admin/space-templates', { space_id: spaceId, name, description: '' }),
    onSuccess: async () => {
      setName('');
      await client.invalidateQueries({ queryKey: ['space-templates'] });
    },
  });
  return (
    <section className="settings-unit">
      <h2>{t('adminOrganise.spaceTemplates')}</h2>
      <p className="muted">{t('adminOrganise.spaceTemplatesHelp')}</p>
      <ErrorNote error={save.error} />
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <select
          required
          aria-label={t('move.space')}
          value={spaceId}
          onChange={(e) => setSpaceId(e.target.value)}
        >
          <option value="">{t('adminOrganise.chooseSpace')}</option>
          {spaces.data?.spaces.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
        <input
          required
          value={name}
          aria-label={t('common.name')}
          placeholder={t('common.name')}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit">{t('common.save')}</button>
      </form>
    </section>
  );
}

export function OrganisingTab() {
  const settings = useOrganising();
  const features = useQuery({
    queryKey: ['admin-features'],
    queryFn: () => api.get<InstanceFeatures>('/admin/features'),
  });
  const experimental = useQuery({
    queryKey: ['admin-experimental'],
    queryFn: () => api.get<ExperimentalParts>('/admin/experimental'),
  });
  if (settings.isPending || features.isPending || experimental.isPending) return <Loading />;
  if (settings.error || features.error || experimental.error) {
    return <ErrorNote error={settings.error ?? features.error ?? experimental.error} />;
  }
  return (
    <>
      <OrganisingForm key={JSON.stringify(settings.data)} start={settings.data} />
      <FeatureTable start={features.data} />
      <ExperimentalTable start={experimental.data} />
      <Categories />
    </>
  );
}

/** Templates of the instance, and saving a space as a template for new spaces (D19). */
export function TemplatesTab() {
  return (
    <>
      <TemplateManager
        listPath="/admin/templates"
        base="/admin/templates"
        reviewPath="/admin/template-review"
        canSetReview
      />
      <SaveSpaceAsTemplate />
    </>
  );
}
