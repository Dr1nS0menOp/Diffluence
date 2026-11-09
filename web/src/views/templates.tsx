// SPDX-License-Identifier: Apache-2.0
// Managing templates, of a space or of all spaces, and writing one.
import { useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Json, type Space, type TemplateSummary } from '../api';
import { Link, navigate } from '../router';
import { DocumentEditor } from '../components/editor/editor';
import { IconArrowBack } from '../components/icons';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { templateFileAddress } from '../components/template-gallery';
import { ErrorNote, Field, FilePicker, Loading } from '../components/ui';
import { TableBox } from '../components/table-box';

function TemplateRows({ list, onChanged }: { list: TemplateSummary[]; onChanged: () => void }) {
  const { t } = useTranslation();
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
      }),
    onSettled: onChanged,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/templates/${id}`),
    onSettled: onChanged,
  });
  return (
    <>
      <ErrorNote error={update.error ?? remove.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('templates.category')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {list.map((x) => (
              <tr key={x.id}>
                <td>
                  <Link href={`/templates/${x.id}/edit`}>{x.name}</Link>
                  {x.hidden && <span className="muted"> {t('templates.hidden')}</span>}
                </td>
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

/** The list and the ways to add one. `base` is where new ones go on the server. */
export function TemplateManager({ listPath, base }: { listPath: string; base: string }) {
  const { t } = useTranslation();
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
    mutationFn: async (file: File) => api.post(`${base}/import`, JSON.parse(await file.text())),
    onSettled: refresh,
  });
  return (
    <>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? create.error ?? upload.error} />
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
    </>
  );
}

function InSpace({ spaceKey, children }: { spaceKey: string; children: (space: Space) => ReactNode }) {
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  return <Frame space={q.data}>{children(q.data)}</Frame>;
}

export function TemplatesView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  return (
    <InSpace spaceKey={spaceKey}>
      {(space) => (
        <>
          <h1>{t('templates.title')}</h1>
          <TemplateManager
            listPath={`/spaces/${space.id}/manage-templates`}
            base={`/spaces/${space.id}/templates`}
          />
        </>
      )}
    </InSpace>
  );
}

function TemplateForm({ template }: { template: TemplateSummary }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState({
    name: template.name,
    description: template.description,
    category: template.category,
    labels: template.labels.join(', '),
    title_pattern: template.title_pattern,
  });
  const doc = useRef<Json>(template.document ?? { type: 'doc', content: [{ type: 'paragraph' }] });
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
      </div>
      <DocumentEditor
        pageId=""
        initial={doc.current}
        canAttach={false}
        inTemplate
        fileAddress={templateFileAddress(template.id)}
        onChange={(next) => {
          doc.current = next;
        }}
        onPublish={() => save.mutate()}
        onSave={() => save.mutate()}
        header={null}
      />
    </>
  );
}

export function TemplateView({ templateId }: { templateId: string }) {
  const q = useQuery({
    queryKey: ['template', templateId],
    queryFn: () => api.get<TemplateSummary>(`/templates/${templateId}`),
    staleTime: Infinity,
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return <TemplateForm key={templateId} template={q.data} />;
}
