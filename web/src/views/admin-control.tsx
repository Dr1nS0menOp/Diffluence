// SPDX-License-Identifier: Apache-2.0
// Administration, Content, Document control (spec 064, DOC-902, DOC-130): the instance
// templates of workflows, which spaces copy, and which copies follow an older version.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type WorkflowTemplate } from '../api';
import { TableBox } from '../components/table-box';
import { ConfirmButton, Dialog, ErrorNote, Loading } from '../components/ui';
import { WorkflowEditor, type WorkflowDraft } from '../components/workflow-editor';
import { startsQuery, useTranslatedExample } from './space-workflows';

const BLANK: WorkflowDraft = { name: '', description: '', definition: { states: [], transitions: [] } };
const KEY = ['admin-workflow-templates'];

type Editing = { kind: 'new'; draft: WorkflowDraft } | { kind: 'edit'; template: WorkflowTemplate };

export function ControlTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [editing, setEditing] = useState<Editing | null>(null);
  const [choosing, setChoosing] = useState(false);
  const q = useQuery({
    queryKey: KEY,
    queryFn: () => api.get<{ templates: WorkflowTemplate[] }>('/admin/workflow-templates'),
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: KEY });
    await client.invalidateQueries({ queryKey: startsQuery.queryKey });
  };
  const save = useMutation({
    mutationFn: (draft: WorkflowDraft) =>
      editing?.kind === 'edit'
        ? api.put(`/admin/workflow-templates/${editing.template.id}`, {
            ...draft,
            base_version: editing.template.version,
          })
        : api.post('/admin/workflow-templates', draft),
    onSuccess: async () => {
      await refresh();
      setEditing(null);
    },
  });
  const archive = useMutation({
    mutationFn: (id: string) => api.post(`/admin/workflow-templates/${id}/archive`),
    onSuccess: refresh,
  });
  if (q.isPending) return <Loading />;
  if (editing) {
    const initial =
      editing.kind === 'new'
        ? editing.draft
        : {
            name: editing.template.name,
            description: editing.template.description,
            definition: editing.template.definition,
          };
    return (
      <section>
        <h2>
          {editing.kind === 'new'
            ? t('controlAdmin.newTitle')
            : t('controlAdmin.editTitle', { name: editing.template.name })}
        </h2>
        <WorkflowEditor
          initial={initial}
          existing={editing.kind === 'edit' ? editing.template.definition : undefined}
          names={editing.kind === 'edit' ? editing.template.names : undefined}
          busy={save.isPending}
          error={save.error}
          onSave={(draft) => save.mutate(draft)}
          onCancel={() => {
            save.reset();
            setEditing(null);
          }}
        />
      </section>
    );
  }
  const list = q.data?.templates ?? [];
  return (
    <section>
      <p className="muted">{t('controlAdmin.intro')}</p>
      <ErrorNote error={q.error ?? archive.error} />
      <p>
        <button type="button" className="primary" onClick={() => setChoosing(true)}>
          {t('controlAdmin.new')}
        </button>
      </p>
      {list.length === 0 ? (
        <p className="muted">{t('controlAdmin.none')}</p>
      ) : (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th scope="col">{t('controlAdmin.columnName')}</th>
                <th scope="col">{t('controlAdmin.columnVersion')}</th>
                <th scope="col">{t('controlAdmin.columnCopies')}</th>
                <th scope="col">
                  <span className="visually-hidden">{t('controlAdmin.columnActions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {list.map((tpl) => (
                <tr key={tpl.id}>
                  <td>{tpl.name}</td>
                  <td>{tpl.version}</td>
                  <td>
                    <Copies template={tpl} />
                  </td>
                  <td>
                    <div className="row">
                      <button
                        type="button"
                        className="ghost"
                        onClick={() => setEditing({ kind: 'edit', template: tpl })}
                      >
                        {t('controlAdmin.edit', { name: tpl.name })}
                      </button>
                      <ConfirmButton
                        className="ghost"
                        label={t('controlAdmin.archive', { name: tpl.name })}
                        message={t('controlAdmin.archiveHelp')}
                        onConfirm={() => archive.mutate(tpl.id)}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      {choosing && (
        <StartDialog
          onClose={() => setChoosing(false)}
          onDraft={(draft) => {
            setChoosing(false);
            setEditing({ kind: 'new', draft });
          }}
        />
      )}
    </section>
  );
}

/** The spaces that copied a template, and which follow an older version. */
function Copies({ template }: { template: WorkflowTemplate }) {
  const { t } = useTranslation();
  const copies = template.copies ?? [];
  if (copies.length === 0) return <span className="muted">{t('controlAdmin.noCopies')}</span>;
  return (
    <ul className="plain-list">
      {copies.map((c) => (
        <li key={c.workflow_id}>
          {t('controlAdmin.copy', { space: c.space_name, workflow: c.workflow_name })}{' '}
          {c.version < template.version ? (
            <span className="lozenge">{t('controlAdmin.older', { version: c.version })}</span>
          ) : (
            <span className="muted">{t('controlAdmin.current')}</span>
          )}
        </li>
      ))}
    </ul>
  );
}

function StartDialog({ onClose, onDraft }: { onClose: () => void; onDraft: (d: WorkflowDraft) => void }) {
  const { t } = useTranslation();
  const q = useQuery(startsQuery);
  const translate = useTranslatedExample();
  return (
    <Dialog title={t('controlAdmin.startTitle')} onClose={onClose}>
      <ul className="plain-list workflow-starts">
        <li>
          <button type="button" onClick={() => onDraft(BLANK)}>
            {t('workflowEditor.startBlank')}
          </button>
        </li>
        {q.data?.examples.map((ex) => {
          const draft = translate(ex);
          return (
            <li key={ex.key}>
              <button type="button" onClick={() => onDraft(draft)}>
                {t('workflowEditor.startExample', { name: draft.name })}
              </button>
              <p className="muted">{draft.description}</p>
            </li>
          );
        })}
      </ul>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
    </Dialog>
  );
}
