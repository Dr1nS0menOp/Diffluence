// SPDX-License-Identifier: Apache-2.0
// Managing templates, of a space or of all spaces, and writing one.
import { useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type Json,
  type Space,
  type TemplateImportNotes,
  type TemplateSummary,
  type Workflow,
} from '../api';
import { Link, navigate } from '../router';
import { DocumentEditor } from '../components/editor/editor';
import { IconArrowBack } from '../components/icons';
import { spaceByKeyQuery } from '../components/frame';
import { SpaceToolFrame } from './space-settings-frame';
import { templateFileAddress } from '../components/template-gallery';
import { ErrorNote, Field, FilePicker, Loading } from '../components/ui';
import { TableBox } from '../components/table-box';
import { useCapabilities } from '../capabilities';
import { TemplatePages, useTemplateValues } from './template-pages';
import { useConditionScope } from './template-rules';
import { ImportNote } from './template-history';
import {
  FourEyesSetting,
  LIST_STATES,
  MyTemplates,
  TemplateStateMark,
  WaitingForApproval,
} from './template-access';

function TemplateRows({ list, onChanged }: { list: TemplateSummary[]; onChanged: () => void }) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const update = useMutation({
    mutationFn: (x: TemplateSummary & { hidden: boolean; promoted: boolean }) =>
      api.patch(`/templates/${x.id}`, {
        name: x.name,
        description: x.description,
        category: x.category,
        labels: x.labels,
        title_pattern: x.title_pattern,
        steps: x.steps,
        default_parent: x.default_parent,
        hidden: x.hidden,
        promoted: x.promoted,
        workflow_id: x.workflow_id,
      }),
    onSettled: onChanged,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/templates/${id}`),
    onSettled: onChanged,
  });
  // The state of each template, and a filter of states, come with approval (TPL-1001).
  const [state, setState] = useState('');
  const shown = state ? list.filter((x) => x.state === state) : list;
  // An empty list says what will appear, not an empty table (design standard, States).
  if (list.length === 0) return <p className="muted">{t('templates.noneYet')}</p>;
  return (
    <>
      <ErrorNote error={update.error ?? remove.error} />
      {guidedOn && list.length > 0 && (
        <Field label={t('templates.filterState')}>
          <select value={state} onChange={(e) => setState(e.target.value)}>
            <option value="">{t('templates.anyState')}</option>
            {LIST_STATES.map((s) => (
              <option key={s} value={s}>
                {t(`templates.state_${s}`)}
              </option>
            ))}
          </select>
        </Field>
      )}
      {guidedOn && shown.length === 0 && list.length > 0 && (
        <p className="muted">{t('templates.noneInState')}</p>
      )}
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              {guidedOn && <th>{t('templates.stateColumn')}</th>}
              <th>{t('templates.category')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((x) => (
              <tr key={x.id}>
                <td>
                  <Link href={`/templates/${x.id}/settings`}>{x.name}</Link>
                  {x.hidden && <span className="muted"> {t('templates.hidden')}</span>}
                  {x.guided && !guidedOn && <span className="muted"> ({t('templates.needsGuided')})</span>}
                  {x.groups_gone && <p className="note-error">{t('templates.groupsGone')}</p>}
                </td>
                {guidedOn && (
                  <td>
                    <TemplateStateMark state={x.state} />
                  </td>
                )}
                <td>{x.category}</td>
                <td className="row">
                  <button
                    type="button"
                    className="ghost"
                    onClick={() => update.mutate({ ...x, hidden: !x.hidden, promoted: x.promoted })}
                  >
                    {x.hidden ? t('templates.show') : t('templates.hide')}
                  </button>
                  <button
                    type="button"
                    className="ghost"
                    onClick={() => update.mutate({ ...x, hidden: x.hidden, promoted: !x.promoted })}
                  >
                    {x.promoted ? t('templates.unpromote') : t('templates.promote')}
                  </button>
                  <a className="ghost" href={`/api/v1/templates/${x.id}/export`}>
                    {t('templates.export')}
                  </a>
                  {!x.starter && (
                    <button type="button" className="ghost" onClick={() => remove.mutate(x.id)}>
                      {t('common.delete')}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </>
  );
}

/**
 * The list and the ways to add one. `base` is where new ones go on the server, and
 * `reviewPath` the setting of a second person for the scope (spec 074, TPL-511).
 */
export function TemplateManager({
  listPath,
  base,
  reviewPath,
  canSetReview,
}: {
  listPath: string;
  base: string;
  reviewPath: string;
  canSetReview: boolean;
}) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['templates', listPath],
    queryFn: () => api.get<{ templates: TemplateSummary[] }>(listPath),
  });
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['templates', listPath] });
  };
  const [name, setName] = useState('');
  const create = useMutation({
    mutationFn: () => api.post<TemplateSummary>(base, { name }),
    onSuccess: (x) => navigate(`/templates/${x.id}/edit`),
  });
  const upload = useMutation({
    mutationFn: async (file: File) =>
      api.post<TemplateSummary & { import_notes?: TemplateImportNotes }>(
        `${base}/import`,
        JSON.parse(await file.text()),
      ),
    onSettled: refresh,
  });
  return (
    <>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? create.error ?? upload.error} />
      {upload.data?.import_notes && <ImportNote name={upload.data.name} notes={upload.data.import_notes} />}
      {guidedOn && q.data && <WaitingForApproval list={q.data.templates} />}
      {q.data && <TemplateRows list={q.data.templates} onChanged={refresh} />}
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
        <button type="submit">{t('templates.create')}</button>
        <FilePicker label={t('templates.import')} accept=".json" onPick={(f) => upload.mutate(f)} />
      </form>
      {guidedOn && <FourEyesSetting path={reviewPath} canChange={canSetReview} />}
    </>
  );
}

function InSpace({ spaceKey, children }: { spaceKey: string; children: (space: Space) => ReactNode }) {
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  return (
    <SpaceToolFrame space={q.data} entry="templates">
      {children(q.data)}
    </SpaceToolFrame>
  );
}

export function TemplatesView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  return (
    <InSpace spaceKey={spaceKey}>
      {(space) => (
        <>
          <h1>{t('templates.title')}</h1>
          {space.can?.['space.templates'] && (
            <TemplateManager
              listPath={`/spaces/${space.id}/manage-templates`}
              base={`/spaces/${space.id}/templates`}
              reviewPath={`/spaces/${space.id}/template-review`}
              canSetReview={!!space.can['space.administer']}
            />
          )}
          {/* One who may propose templates and not manage them finds only their own here (TPL-1002). */}
          {!space.can?.['space.templates'] && guidedOn && space.can?.['template.propose'] && (
            <MyTemplates space={space} />
          )}
        </>
      )}
    </InSpace>
  );
}

function TemplateForm({ template }: { template: TemplateSummary }) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const client = useQueryClient();
  const [form, setForm] = useState({
    name: template.name,
    description: template.description,
    category: template.category,
    labels: template.labels.join(', '),
    title_pattern: template.title_pattern,
    workflow_id: template.workflow_id ?? '',
  });
  const doc = useRef<Json>(template.document ?? { type: 'doc', content: [{ type: 'paragraph' }] });
  const values = useTemplateValues(template.id);
  const conditions = useConditionScope(template.id);
  const save = useMutation({
    mutationFn: () =>
      api.patch(`/templates/${template.id}`, {
        ...form,
        labels: form.labels
          .split(',')
          .map((l) => l.trim())
          .filter(Boolean),
        steps: template.steps,
        default_parent: template.default_parent,
        hidden: template.hidden,
        promoted: template.promoted,
        workflow_id: form.workflow_id || null,
        document: doc.current,
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: ['template', template.id] }),
  });
  const set = (key: keyof typeof form) => (e: { target: { value: string } }) =>
    setForm({ ...form, [key]: e.target.value });
  return (
    <>
      <div className="edit-bar">
        <button type="button" className="quiet edit-close" onClick={() => window.history.back()}>
          <IconArrowBack size={18} /> {t('editor.close')}
        </button>
        <span role="status" className="muted edit-status">
          {save.isSuccess && t('editor.saved')}
        </span>
        <button type="button" className="primary" onClick={() => save.mutate()}>
          {t('common.save')}
        </button>
      </div>
      <ErrorNote error={save.error} />
      <div className="template-details">
        <Field label={t('common.name')}>
          <input value={form.name} required onChange={set('name')} />
        </Field>
        <Field label={t('space.description')}>
          <input value={form.description} onChange={set('description')} />
        </Field>
        <Field label={t('templates.category')}>
          <input value={form.category} onChange={set('category')} />
        </Field>
        <Field label={t('organise.labels')} help={t('templates.labelsHelp')}>
          <input value={form.labels} onChange={set('labels')} />
        </Field>
        <Field label={t('templates.titlePattern')} help={t('templates.titlePatternHelp')}>
          <input value={form.title_pattern} onChange={set('title_pattern')} />
        </Field>
        {template.space_id && (
          <TemplateWorkflow
            spaceId={template.space_id}
            value={form.workflow_id}
            onChange={set('workflow_id')}
          />
        )}
      </div>
      <DocumentEditor
        pageId=""
        initial={doc.current}
        canAttach={false}
        inTemplate
        templateValues={values}
        templateConditions={conditions}
        fileAddress={templateFileAddress(template.id)}
        onChange={(next) => {
          doc.current = next;
        }}
        onPublish={() => save.mutate()}
        onSave={() => save.mutate()}
        header={null}
      />
      <p className="template-details">
        <Link href={`/templates/${template.id}/settings`}>{t('templates.openSettings')}</Link>
      </p>
      {guidedOn && <TemplatePages templateId={template.id} />}
    </>
  );
}

/** The workflow a page made from a template of a space enters (spec 064, DOC-560). */
function TemplateWorkflow({
  spaceId,
  value,
  onChange,
}: {
  spaceId: string;
  value: string;
  onChange: (e: { target: { value: string } }) => void;
}) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['workflows', spaceId],
    queryFn: () => api.get<{ workflows: Workflow[] }>(`/spaces/${spaceId}/workflows`),
  });
  const active = (q.data?.workflows ?? []).filter((w) => !w.archived);
  if (!q.data || (active.length === 0 && !value)) return null;
  return (
    <Field label={t('templates.workflow')} help={t('templates.workflowHelp')}>
      <select value={value} onChange={onChange}>
        <option value="">{t('templates.noWorkflow')}</option>
        {active.map((w) => (
          <option key={w.id} value={w.id}>
            {w.name}
          </option>
        ))}
      </select>
    </Field>
  );
}

export function TemplateView({ templateId }: { templateId: string }) {
  const q = useQuery({
    queryKey: ['template', templateId],
    // The editor changes the newest version, which may wait while another is in use (TPL-523).
    queryFn: () => api.get<TemplateSummary>(`/templates/${templateId}/newest`),
    staleTime: Infinity,
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return <TemplateForm key={templateId} template={q.data} />;
}
