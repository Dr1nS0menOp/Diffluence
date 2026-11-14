// SPDX-License-Identifier: Apache-2.0
// The pages a guided template makes, in the template's own editor (spec 074, wave GT1).
// A minimal list: add a page below another, write its text, remove it, and choose
// whether the pages are published as they are made. The editor's full sections come later.
import { useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Json, type TemplateEntries, type TemplateEntry } from '../api';
import { DocumentEditor } from '../components/editor/editor';
import { templateFileAddress } from '../components/template-gallery';
import { ErrorNote, Field, Loading } from '../components/ui';
import { TableBox } from '../components/table-box';

/** How deep an entry sits below the first page, for indenting its name. */
function depthOf(entry: TemplateEntry, all: TemplateEntry[]): number {
  let depth = 0;
  let parent = entry.parent;
  while (parent) {
    depth += 1;
    parent = all.find((e) => e.key === parent)?.parent ?? '';
  }
  return depth;
}

function EntryText({
  templateId,
  entry,
  onDone,
}: {
  templateId: string;
  entry: TemplateEntry;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const full = useQuery({
    queryKey: ['template-entry', templateId, entry.key],
    queryFn: () => api.get<TemplateEntry>(`/templates/${templateId}/entries/${entry.key}`),
  });
  const [name, setName] = useState(entry.name);
  const [pattern, setPattern] = useState(entry.title_pattern);
  const doc = useRef<Json | null>(null);
  const save = useMutation({
    mutationFn: () =>
      api.patch(`/templates/${templateId}/entries/${entry.key}`, {
        name,
        kind: entry.kind,
        parent: entry.parent,
        title_pattern: pattern,
        labels: entry.labels,
        restrictions: entry.restrictions,
        workflow_id: entry.workflow_id,
        document: doc.current ?? full.data?.document,
      }),
    onSuccess: onDone,
  });
  if (full.error) return <ErrorNote error={full.error} />;
  if (!full.data) return <Loading />;
  return (
    <section aria-label={t('templates.writing', { name: entry.name })}>
      <h3>{t('templates.writing', { name: entry.name })}</h3>
      <ErrorNote error={save.error} />
      <div className="template-details">
        <Field label={t('templates.pageName')}>
          <input value={name} required onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={t('templates.titlePattern')} help={t('templates.titlePatternHelp')}>
          <input value={pattern} onChange={(e) => setPattern(e.target.value)} />
        </Field>
      </div>
      <DocumentEditor
        pageId=""
        initial={full.data.document ?? { type: 'doc', content: [{ type: 'paragraph' }] }}
        canAttach={false}
        inTemplate
        fileAddress={templateFileAddress(templateId)}
        onChange={(next) => {
          doc.current = next;
        }}
        onPublish={() => save.mutate()}
        onSave={() => save.mutate()}
        header={null}
      />
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onDone}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" disabled={save.isPending} onClick={() => save.mutate()}>
          {t('common.save')}
        </button>
      </div>
    </section>
  );
}

/** The list of pages a template makes, shown while guided templates are on. */
export function TemplatePages({ templateId }: { templateId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const queryKey = ['template-entries', templateId];
  const list = useQuery({
    queryKey,
    queryFn: () => api.get<TemplateEntries>(`/templates/${templateId}/entries`),
  });
  const refresh = () => client.invalidateQueries({ queryKey });
  const [name, setName] = useState('');
  const [below, setBelow] = useState('root');
  const [writing, setWriting] = useState<TemplateEntry | null>(null);
  const add = useMutation({
    mutationFn: () => api.post(`/templates/${templateId}/entries`, { name, parent: below, kind: 'page' }),
    onSuccess: () => {
      setName('');
      void refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (key: string) => api.del(`/templates/${templateId}/entries/${key}`),
    onSettled: refresh,
  });
  const publishing = useMutation({
    mutationFn: (publish: boolean) => api.put(`/templates/${templateId}/publishing`, { publish }),
    onSettled: refresh,
  });
  const entries = list.data?.entries ?? [];
  const nameOf = (entry: TemplateEntry) => entry.name || t('templates.firstPage');
  return (
    <section className="template-pages" aria-labelledby="template-pages-title">
      <h2 id="template-pages-title">{t('templates.pages')}</h2>
      <p className="muted">{t('templates.pagesHelp')}</p>
      {list.isPending && <Loading />}
      <ErrorNote error={list.error ?? add.error ?? remove.error ?? publishing.error} />
      {list.data && (
        <>
          <TableBox>
            <table>
              <thead>
                <tr>
                  <th>{t('common.name')}</th>
                  <th>{t('templates.below')}</th>
                  <th>{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => {
                  const parent = entries.find((e) => e.key === entry.parent);
                  return (
                    <tr key={entry.key}>
                      <td data-depth={depthOf(entry, entries)}>{nameOf(entry)}</td>
                      <td>{parent ? nameOf(parent) : ''}</td>
                      <td className="row">
                        {entry.key !== 'root' && (
                          <>
                            <button type="button" className="ghost" onClick={() => setWriting(entry)}>
                              {t('templates.editText')}
                            </button>
                            <button type="button" className="ghost" onClick={() => remove.mutate(entry.key)}>
                              {t('templates.removePage')}
                            </button>
                          </>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableBox>
          <form
            className="row"
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              add.mutate();
            }}
          >
            <input
              required
              value={name}
              aria-label={t('templates.pageName')}
              placeholder={t('templates.pageName')}
              onChange={(e) => setName(e.target.value)}
            />
            <select
              aria-label={t('templates.below')}
              value={below}
              onChange={(e) => setBelow(e.target.value)}
            >
              {entries.map((entry) => (
                <option key={entry.key} value={entry.key}>
                  {nameOf(entry)}
                </option>
              ))}
            </select>
            <button type="submit" disabled={add.isPending}>
              {t('templates.addPage')}
            </button>
          </form>
          <label className="check">
            <input
              type="checkbox"
              checked={list.data.publish}
              disabled={publishing.isPending}
              onChange={(e) => publishing.mutate(e.target.checked)}
            />
            {t('templates.publishAtOnce')}
          </label>
          <p className="muted">{t('templates.publishAtOnceHelp')}</p>
        </>
      )}
      {writing && (
        <EntryText
          key={writing.key}
          templateId={templateId}
          entry={writing}
          onDone={() => {
            setWriting(null);
            void refresh();
          }}
        />
      )}
    </section>
  );
}
