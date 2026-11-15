// SPDX-License-Identifier: Apache-2.0
// Rights per template and approval (spec 074, wave GT4): the Who can use it section, the
// state of a template, the setting "every template change needs a second person", and My
// templates for a person who may propose templates without managing them.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type DirectoryEntry,
  type Space,
  type TemplateRights,
  type TemplateState,
  type TemplateSummary,
} from '../api';
import { Link, navigate } from '../router';
import { SubjectPicker } from '../components/picker';
import { TableBox } from '../components/table-box';
import { ErrorNote, Field, Loading } from '../components/ui';

type Subject = TemplateRights['use'][number];

/** The state of a template or a version, as a word in a lozenge (TPL-1001). */
export function TemplateStateMark({ state }: { state: TemplateState | undefined }) {
  const { t } = useTranslation();
  if (!state) return null;
  return <span className="lozenge">{t(`templates.state_${state}`)}</span>;
}

/** The states the list of templates filters by (TPL-1001). */
export const LIST_STATES: TemplateState[] = ['draft', 'waiting', 'approved', 'sent_back', 'withdrawn'];

/** A list of people and groups that only narrows: empty limits nothing (TPL-500). */
function AudienceList({
  title,
  help,
  addLabel,
  subjects,
  names,
  onChange,
}: {
  title: string;
  help: string;
  addLabel: string;
  subjects: Subject[];
  names: Record<string, string>;
  onChange: (next: Subject[], name?: DirectoryEntry) => void;
}) {
  const { t } = useTranslation();
  return (
    <section>
      <h2>{title}</h2>
      <p className="muted">{help}</p>
      {subjects.length === 0 && <p className="muted">{t('templates.listEmpty')}</p>}
      <ul className="plain-list">
        {subjects.map((s) => (
          <li key={s.type + s.id} className="row">
            {names[s.id] ?? t('templates.subjectGone')}
            <button
              type="button"
              className="ghost"
              aria-label={t('templates.removeFrom', {
                name: names[s.id] ?? t('templates.subjectGone'),
                list: title,
              })}
              onClick={() => onChange(subjects.filter((x) => x.id !== s.id))}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <SubjectPicker
        label={addLabel}
        onPick={(e: DirectoryEntry) => {
          if (e.type !== 'user' && e.type !== 'group') return;
          if (subjects.some((s) => s.id === e.id)) return;
          onChange([...subjects, { type: e.type, id: e.id }], e);
        }}
      />
    </section>
  );
}

/** Where a template of all spaces is offered: every space, or the spaces chosen (TPL-502). */
function OfferedIn({
  chosen,
  spaces,
  onChange,
}: {
  chosen: boolean;
  spaces: string[];
  onChange: (chosen: boolean, spaces: string[]) => void;
}) {
  const { t } = useTranslation();
  const all = useQuery({
    queryKey: ['admin-spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/admin/spaces'),
  });
  const nameOf = (id: string) => all.data?.spaces.find((s) => s.id === id)?.name ?? id;
  const left = (all.data?.spaces ?? []).filter((s) => !spaces.includes(s.id));
  return (
    <section>
      <h2>{t('templates.offeredWhere')}</h2>
      <fieldset className="choice-row">
        <legend className="visually-hidden">{t('templates.offeredWhere')}</legend>
        <label className="check">
          <input
            type="radio"
            name="template-offered"
            checked={!chosen}
            onChange={() => onChange(false, [])}
          />
          {t('templates.everySpace')}
        </label>
        <label className="check">
          <input
            type="radio"
            name="template-offered"
            checked={chosen}
            onChange={() => onChange(true, spaces)}
          />
          {t('templates.chosenSpaces')}
        </label>
      </fieldset>
      {chosen && (
        <>
          <ul className="chip-list">
            {spaces.map((id) => (
              <li key={id} className="chip">
                {nameOf(id)}
                <button
                  type="button"
                  className="ghost"
                  aria-label={t('templates.removeSpace', { name: nameOf(id) })}
                  onClick={() =>
                    onChange(
                      true,
                      spaces.filter((s) => s !== id),
                    )
                  }
                >
                  {t('common.remove')}
                </button>
              </li>
            ))}
          </ul>
          <Field label={t('templates.addSpace')}>
            <select value="" onChange={(e) => e.target.value && onChange(true, [...spaces, e.target.value])}>
              <option value="">{t('templates.pickSpace')}</option>
              {left.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>
          {spaces.length === 0 && <p className="note">{t('templates.offeredNowhere')}</p>}
        </>
      )}
    </section>
  );
}

/** The Who can use it section of a template's settings (TPL-500 to TPL-502, TPL-1010). */
export function TemplateAccessSection({ templateId }: { templateId: string }) {
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['template-rights', templateId],
    queryFn: () => api.get<TemplateRights>(`/templates/${templateId}/rights`),
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <AccessForm
      key={JSON.stringify(q.data)}
      start={q.data}
      onSaved={(next) => client.setQueryData(['template-rights', templateId], next)}
      templateId={templateId}
    />
  );
}

function AccessForm({
  start,
  templateId,
  onSaved,
}: {
  start: TemplateRights;
  templateId: string;
  onSaved: (next: TemplateRights) => void;
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState(start);
  const save = useMutation({
    mutationFn: () =>
      api.put<TemplateRights>(`/templates/${templateId}/rights`, {
        use: form.use,
        edit: form.edit,
        chosen: form.chosen,
        spaces: form.spaces,
      }),
    onSuccess: onSaved,
  });
  const named = (e?: DirectoryEntry) => (e ? { ...form.names, [e.id]: e.name } : form.names);
  return (
    <form
      className="settings-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <p className="muted">{t('templates.accessHelp')}</p>
      <ErrorNote error={save.error} />
      <AudienceList
        title={t('templates.whoUses')}
        help={t('templates.whoUsesHelp')}
        addLabel={t('templates.addUser')}
        subjects={form.use}
        names={form.names}
        onChange={(use, e) => setForm({ ...form, use, names: named(e) })}
      />
      <AudienceList
        title={t('templates.whoEdits')}
        help={t('templates.whoEditsHelp')}
        addLabel={t('templates.addEditor')}
        subjects={form.edit}
        names={form.names}
        onChange={(edit, e) => setForm({ ...form, edit, names: named(e) })}
      />
      {form.all_spaces && (
        <OfferedIn
          chosen={form.chosen}
          spaces={form.spaces}
          onChange={(chosen, spaces) => setForm({ ...form, chosen, spaces })}
        />
      )}
      <div className="dialog-actions">
        {save.isSuccess && (
          <span role="status" className="muted">
            {t('editor.saved')}
          </span>
        )}
        <button type="submit" className="primary" disabled={save.isPending}>
          {t('common.save')}
        </button>
      </div>
    </form>
  );
}

/**
 * The setting "every template change needs a second person" of a space, or of the templates of
 * all spaces (TPL-511). Whoever runs the scope changes it; those who manage templates see it.
 */
export function FourEyesSetting({ path, canChange }: { path: string; canChange: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['template-review', path],
    queryFn: () => api.get<{ on: boolean; others_can_approve: boolean }>(path),
  });
  const set = useMutation({
    mutationFn: (on: boolean) => api.put<{ on: boolean; others_can_approve: boolean }>(path, { on }),
    onSuccess: (next) => client.setQueryData(['template-review', path], next),
  });
  if (!q.data) return <ErrorNote error={q.error} />;
  return (
    <section aria-labelledby="four-eyes-heading">
      <h2 id="four-eyes-heading">{t('templates.fourEyesTitle')}</h2>
      <ErrorNote error={set.error} />
      <label className="check">
        <input
          type="checkbox"
          checked={q.data.on}
          disabled={!canChange || set.isPending}
          onChange={(e) => set.mutate(e.target.checked)}
        />
        {t('templates.fourEyes')}
      </label>
      <p className="muted">{t('templates.fourEyesHelp')}</p>
      {q.data.on && !q.data.others_can_approve && <p className="note">{t('templates.fourEyesAlone')}</p>}
    </section>
  );
}

/** The templates waiting for a decision, at the top of the list for those who decide (TPL-1001). */
export function WaitingForApproval({ list }: { list: TemplateSummary[] }) {
  const { t } = useTranslation();
  const waiting = list.filter((x) => x.state === 'waiting' && !x.mine);
  if (waiting.length === 0) return null;
  return (
    <section aria-labelledby="waiting-heading">
      <h2 id="waiting-heading">{t('templates.waitingTitle')}</h2>
      <ul className="plain-list">
        {waiting.map((x) => (
          <li key={x.id}>
            <Link href={`/templates/${x.id}/settings?section=versions`}>{x.name}</Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * My templates: the drafts and proposals of a person who may propose templates in a space and
 * does not manage them. They see only their own (TPL-1002).
 */
export function MyTemplates({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['my-templates', space.id],
    queryFn: () => api.get<{ templates: TemplateSummary[] }>(`/spaces/${space.id}/my-templates`),
  });
  const [name, setName] = useState('');
  const create = useMutation({
    mutationFn: () => api.post<TemplateSummary>(`/spaces/${space.id}/my-templates`, { name }),
    onSuccess: (x) => navigate(`/templates/${x.id}/edit`),
  });
  return (
    <>
      <h2>{t('templates.mine')}</h2>
      <p className="muted">{t('templates.mineHelp')}</p>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? create.error} />
      {q.data?.templates.length === 0 && <p className="muted">{t('templates.mineNone')}</p>}
      {q.data && q.data.templates.length > 0 && (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('common.name')}</th>
                <th>{t('templates.stateColumn')}</th>
              </tr>
            </thead>
            <tbody>
              {q.data.templates.map((x) => (
                <tr key={x.id}>
                  <td>
                    <Link href={`/templates/${x.id}/settings`}>{x.name}</Link>
                  </td>
                  <td>
                    <TemplateStateMark state={x.state} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <input
          required
          value={name}
          aria-label={t('templates.newName')}
          placeholder={t('templates.newName')}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" disabled={create.isPending}>
          {t('templates.createDraft')}
        </button>
      </form>
    </>
  );
}
