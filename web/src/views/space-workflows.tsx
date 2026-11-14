// SPDX-License-Identifier: Apache-2.0
// The workflows of a space, in its settings (spec 064, DOC-900, DOC-910 to DOC-913, DOC-130):
// the list, a new workflow from scratch, from an example or from an instance template, the
// editor, and taking a newer version of a template by choice.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type Space,
  type Workflow,
  type WorkflowChange,
  type WorkflowDefinition,
  type WorkflowExample,
  type WorkflowTemplate,
} from '../api';
import { TableBox } from '../components/table-box';
import { Dialog, ErrorNote, Loading, RelativeTime } from '../components/ui';
import { WorkflowEditor, type WorkflowDraft } from '../components/workflow-editor';

const BLANK: WorkflowDraft = { name: '', description: '', definition: { states: [], transitions: [] } };

/** The words of an example in the reader's language; a copy keeps whatever is saved. */
export function useTranslatedExample() {
  const { t } = useTranslation();
  return (ex: WorkflowExample): WorkflowDraft => {
    const base = `workflowExamples.${ex.key}`;
    const definition: WorkflowDefinition = {
      states: ex.definition.states.map((s) => ({
        ...s,
        name: t(`${base}.states.${s.id}`, { defaultValue: s.name }),
      })),
      transitions: ex.definition.transitions.map((tr) => ({
        ...tr,
        name: t(`${base}.transitions.${tr.id}`, { defaultValue: tr.name }),
      })),
    };
    return {
      name: t(`${base}.name`, { defaultValue: ex.name }),
      description: t(`${base}.description`, { defaultValue: ex.description }),
      definition,
    };
  };
}

export interface Starts {
  templates: WorkflowTemplate[];
  examples: WorkflowExample[];
}

export const startsQuery = {
  queryKey: ['workflow-starts'],
  queryFn: () => api.get<Starts>('/workflow-templates'),
};

type Editing = { kind: 'new'; draft: WorkflowDraft } | { kind: 'edit'; workflow: Workflow };

export function WorkflowsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [editing, setEditing] = useState<Editing | null>(null);
  const [choosing, setChoosing] = useState(false);
  const key = ['workflows', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ workflows: Workflow[] }>(`/spaces/${space.id}/workflows`),
  });
  const done = async () => {
    await client.invalidateQueries({ queryKey: key });
    setEditing(null);
  };
  const save = useMutation({
    mutationFn: (draft: WorkflowDraft) =>
      editing?.kind === 'edit'
        ? api.put(`/workflows/${editing.workflow.id}`, { ...draft, base_version: editing.workflow.version })
        : api.post(`/spaces/${space.id}/workflows`, draft),
    onSuccess: done,
  });
  const copy = useMutation({
    mutationFn: (template: WorkflowTemplate) =>
      api.post<Workflow>(`/spaces/${space.id}/workflows/from-template`, { template_id: template.id }),
    onSuccess: async (made) => {
      await client.invalidateQueries({ queryKey: key });
      setChoosing(false);
      setEditing({ kind: 'edit', workflow: made });
    },
  });
  if (q.isPending) return <Loading />;
  if (editing) {
    const initial =
      editing.kind === 'new'
        ? editing.draft
        : {
            name: editing.workflow.name,
            description: editing.workflow.description,
            definition: editing.workflow.definition,
          };
    return (
      <section>
        <h2>
          {editing.kind === 'new'
            ? t('workflowEditor.newTitle')
            : t('workflowEditor.editTitle', { name: editing.workflow.name })}
        </h2>
        <WorkflowEditor
          key={editing.kind === 'edit' ? editing.workflow.id + editing.workflow.version : 'new'}
          initial={initial}
          existing={editing.kind === 'edit' ? editing.workflow.definition : undefined}
          names={editing.kind === 'edit' ? editing.workflow.names : undefined}
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
  const list = q.data?.workflows ?? [];
  return (
    <section>
      <p className="muted">{t('control.settingsIntro')}</p>
      <ErrorNote error={q.error} />
      <p>
        <button type="button" className="primary" onClick={() => setChoosing(true)}>
          {t('workflowEditor.new')}
        </button>
      </p>
      {list.length === 0 ? (
        <p className="muted">{t('control.none')}</p>
      ) : (
        <WorkflowTable list={list} onEdit={(workflow) => setEditing({ kind: 'edit', workflow })} />
      )}
      {choosing && (
        <StartDialog
          busy={copy.isPending}
          error={copy.error}
          onClose={() => setChoosing(false)}
          onTemplate={(template) => copy.mutate(template)}
          onDraft={(draft) => {
            setChoosing(false);
            setEditing({ kind: 'new', draft });
          }}
        />
      )}
    </section>
  );
}

function WorkflowTable({ list, onEdit }: { list: Workflow[]; onEdit: (w: Workflow) => void }) {
  const { t } = useTranslation();
  const [comparing, setComparing] = useState<Workflow | null>(null);
  return (
    <>
      <TableBox>
        <table>
          <thead>
            <tr>
              <th scope="col">{t('control.columnName')}</th>
              <th scope="col">{t('control.columnStates')}</th>
              <th scope="col">{t('control.columnVersion')}</th>
              <th scope="col">{t('control.columnPages')}</th>
              <th scope="col">
                <span className="visually-hidden">{t('workflowEditor.columnActions')}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {list.map((w) => (
              <tr key={w.id}>
                <td>
                  {w.name} {w.space_workflow && <span className="lozenge">{t('control.spaceWorkflow')}</span>}
                  {w.archived && <span className="lozenge">{t('workflowEditor.archived')}</span>}
                  {w.template_latest !== null &&
                    w.template_version !== null &&
                    w.template_latest > w.template_version && (
                      <p className="muted">
                        {t('workflowEditor.templateNewer', { version: w.template_latest })}{' '}
                        <button type="button" className="ghost" onClick={() => setComparing(w)}>
                          {t('workflowEditor.templateCompare')}
                        </button>
                      </p>
                    )}
                </td>
                <td>{w.definition.states.map((s) => s.name).join(', ')}</td>
                <td>
                  {w.version}{' '}
                  <span className="muted">{w.saved_at && <RelativeTime value={w.saved_at} />}</span>
                </td>
                <td>{w.pages}</td>
                <td>
                  {!w.archived && (
                    <button type="button" className="ghost" onClick={() => onEdit(w)}>
                      {t('workflowEditor.edit', { name: w.name })}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      {comparing && <TemplateDialog workflow={comparing} onClose={() => setComparing(null)} />}
    </>
  );
}

function StartDialog({
  busy,
  error,
  onClose,
  onTemplate,
  onDraft,
}: {
  busy: boolean;
  error: unknown;
  onClose: () => void;
  onTemplate: (t: WorkflowTemplate) => void;
  onDraft: (d: WorkflowDraft) => void;
}) {
  const { t } = useTranslation();
  const q = useQuery(startsQuery);
  const translate = useTranslatedExample();
  return (
    <Dialog title={t('workflowEditor.startTitle')} onClose={onClose}>
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
        {q.data?.templates.map((tpl) => (
          <li key={tpl.id}>
            <button type="button" disabled={busy} onClick={() => onTemplate(tpl)}>
              {t('workflowEditor.startTemplate', { name: tpl.name })}
            </button>
            {tpl.description && <p className="muted">{tpl.description}</p>}
          </li>
        ))}
      </ul>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? error} />
    </Dialog>
  );
}

/** A newer version of the template, compared before the space takes it (DOC-130). */
function TemplateDialog({ workflow, onClose }: { workflow: Workflow; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['template-changes', workflow.id],
    queryFn: () =>
      api.get<{ changes: WorkflowChange[]; template_version: number }>(
        `/workflows/${workflow.id}/template-changes`,
      ),
  });
  const take = useMutation({
    mutationFn: () => api.post(`/workflows/${workflow.id}/take-template`, { base_version: workflow.version }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['workflows', workflow.space_id] });
      onClose();
    },
  });
  return (
    <Dialog title={t('workflowEditor.templateTitle', { name: workflow.name })} onClose={onClose}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? take.error} />
      {q.data && (
        <>
          <ul>
            {q.data.changes.map((c) => (
              <li key={c.kind + c.id}>{t(`workflowEditor.change_${c.kind}`, { from: c.from, to: c.to })}</li>
            ))}
          </ul>
          <p>{t('workflowEditor.runningRounds')}</p>
          <div className="dialog-actions">
            <button type="button" className="quiet" onClick={onClose}>
              {t('common.cancel')}
            </button>
            <button type="button" className="primary" disabled={take.isPending} onClick={() => take.mutate()}>
              {t('workflowEditor.templateTake', { version: q.data.template_version })}
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}
