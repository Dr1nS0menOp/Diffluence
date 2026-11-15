// SPDX-License-Identifier: Apache-2.0
// A template's settings in the Settings frame, one section at a time at its own address
// (spec 074, TPL-1010): General, Pages, Form, Rules, Who can use it and Versions. The text of
// each page is written in the editor, the Focus frame.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type Space,
  type TemplateChanges,
  type TemplateHistory,
  type TemplateSummary,
  type TemplateVersionRow,
} from '../api';
import { useCapabilities } from '../capabilities';
import { useDates } from '../dates';
import { Link, useLocation } from '../router';
import { DiffDocument } from '../components/editor/diff-view';
import { IconArrowDown, IconArrowUp, IconChevronLeft } from '../components/icons';
import { PagePicker } from '../components/picker';
import { SettingsFrame } from '../components/settings-frame';
import { TableBox } from '../components/table-box';
import { CloseFormDialog, TemplateForm } from '../components/template-form';
import { Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { fieldSpecOf, type FieldSpec } from '../template-fields';
import { FieldDialog } from './template-field-dialog';
import { TemplatePages } from './template-pages';
import { TemplateAccessSection, TemplateStateMark } from './template-access';
import {
  ConditionCell,
  formOrder,
  RenameDialog,
  RulesSection,
  scopeWith,
  useConditionScope,
} from './template-rules';

export const TEMPLATE_SECTIONS = ['general', 'pages', 'form', 'rules', 'access', 'versions'] as const;
export type TemplateSection = (typeof TEMPLATE_SECTIONS)[number];

const BACK_ICON_SIZE = 16;

type Step = TemplateSummary['steps'][number];

/** What PATCH needs to keep everything of a template but what changed. */
function rowOf(template: TemplateSummary) {
  return {
    name: template.name,
    description: template.description,
    category: template.category,
    labels: template.labels,
    title_pattern: template.title_pattern,
    steps: template.steps,
    default_parent: template.default_parent,
    hidden: template.hidden,
    promoted: template.promoted,
  };
}

function GeneralSection({
  template,
  space,
  onSaved,
}: {
  template: TemplateSummary;
  space: Space | null;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const [form, setForm] = useState({
    name: template.name,
    description: template.description,
    category: template.category,
    labels: template.labels.join(', '),
    title_pattern: template.title_pattern,
    hidden: template.hidden,
    promoted: template.promoted,
    default_parent: template.default_parent ?? '',
  });
  const save = useMutation({
    mutationFn: () =>
      api.patch(`/templates/${template.id}`, {
        ...rowOf(template),
        ...form,
        labels: form.labels
          .split(',')
          .map((l) => l.trim())
          .filter(Boolean),
        default_parent: form.default_parent || null,
      }),
    onSuccess: onSaved,
  });
  const entries = useQuery({
    queryKey: ['template-entries', template.id],
    queryFn: () => api.get<{ publish: boolean }>(`/templates/${template.id}/entries`),
    enabled: guidedOn,
  });
  const publishing = useMutation({
    mutationFn: (publish: boolean) => api.put(`/templates/${template.id}/publishing`, { publish }),
    onSettled: () => entries.refetch(),
  });
  const text = (key: 'name' | 'description' | 'category' | 'labels' | 'title_pattern') => ({
    value: form[key],
    onChange: (e: { target: { value: string } }) => setForm({ ...form, [key]: e.target.value }),
  });
  return (
    <form
      className="settings-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error ?? publishing.error} />
      <p>
        <Link href={`/templates/${template.id}/edit`}>{t('templates.editFirstText')}</Link>
      </p>
      <Field label={t('common.name')}>
        <input required {...text('name')} />
      </Field>
      <Field label={t('space.description')}>
        <input {...text('description')} />
      </Field>
      <Field label={t('templates.category')}>
        <input {...text('category')} />
      </Field>
      <Field label={t('organise.labels')} help={t('templates.labelsHelp')}>
        <input {...text('labels')} />
      </Field>
      <Field label={t('templates.titlePattern')} help={t('templates.titlePatternHelp')}>
        <input {...text('title_pattern')} />
      </Field>
      {space && (
        // A default parent is a page of the template's own space (ORG-051).
        <PagePicker
          label={t('templates.defaultParent')}
          help={t('templates.defaultParentHelp')}
          value={form.default_parent}
          spaceKey={space.key}
          onChange={(id) => setForm({ ...form, default_parent: id })}
        />
      )}
      <label className="check">
        <input
          type="checkbox"
          checked={form.hidden}
          onChange={(e) => setForm({ ...form, hidden: e.target.checked })}
        />
        {t('templates.hiddenSetting')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={form.promoted}
          onChange={(e) => setForm({ ...form, promoted: e.target.checked })}
        />
        {t('templates.promotedSetting')}
      </label>
      {guidedOn && entries.data && (
        <>
          <label className="check">
            <input
              type="checkbox"
              checked={entries.data.publish}
              disabled={publishing.isPending}
              onChange={(e) => publishing.mutate(e.target.checked)}
            />
            {t('templates.publishAtOnce')}
          </label>
          <p className="muted">{t('templates.publishAtOnceHelp')}</p>
        </>
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

/** Moves one item of a list a place up or down. */
function moved<T>(list: T[], index: number, by: number): T[] {
  const next = [...list];
  const [item] = next.splice(index, 1);
  if (item !== undefined) next.splice(index + by, 0, item);
  return next;
}

function TryForm({
  template,
  space,
  onClose,
}: {
  template: TemplateSummary;
  space: Space;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [dirty, setDirty] = useState(false);
  const [asking, setAsking] = useState(false);
  const leave = () => (dirty ? setAsking(true) : onClose());
  return (
    <>
      <Dialog title={t('templates.tryTitle', { name: template.name })} onClose={leave} onEscape={leave}>
        <TemplateForm
          template={template}
          space={space}
          parent={null}
          guidedOn
          tryOnly
          onBack={leave}
          onClose={onClose}
          onDirty={() => setDirty(true)}
        />
      </Dialog>
      {asking && <CloseFormDialog onConfirm={onClose} onClose={() => setAsking(false)} />}
    </>
  );
}

/**
 * The steps of the form and the questions in them (ORG-061, TPL-1012). Steps are rows that
 * move with Move up and Move down; a question is put in a step from its row. While guided
 * templates are off, only the steps change; the settings of a question are a part of them.
 */
function FormSection({
  template,
  space,
  onSaved,
}: {
  template: TemplateSummary;
  space: Space | null;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const [fields, setFields] = useState<FieldSpec[]>((template.fields ?? []).map(fieldSpecOf));
  const [steps, setSteps] = useState<Step[]>(template.steps);
  const [editing, setEditing] = useState<FieldSpec | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [trying, setTrying] = useState(false);
  // A question may be shown by questions asked before it, a step by the steps before it (TPL-421).
  const base = useConditionScope(template.id);
  const order = formOrder(fields, steps);
  const position = (name: string) => order.findIndex((o) => o.field.name === name);
  const stepAt = (name: string) => order.find((o) => o.field.name === name)?.step ?? steps.length;
  const save = useMutation({
    mutationFn: () =>
      guidedOn
        ? api.put(`/templates/${template.id}/fields`, { fields, steps })
        : api.patch(`/templates/${template.id}`, { ...rowOf(template), steps }),
    onSuccess: onSaved,
  });
  const stepOf = (name: string) => steps.findIndex((s) => s.fields.includes(name));
  const putInStep = (name: string, index: number) =>
    setSteps(
      steps.map((s, i) => ({
        ...s,
        fields:
          i === index ? [...s.fields.filter((f) => f !== name), name] : s.fields.filter((f) => f !== name),
      })),
    );
  // The dialogs stand beside the form, so their own forms never sit inside it.
  return (
    <>
      <form
        className="settings-form"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        <h2>{t('templates.steps')}</h2>
        <p className="muted">{t('templates.stepsHelp')}</p>
        {steps.length === 0 && <p className="muted">{t('templates.noSteps')}</p>}
        {steps.length > 0 && (
          <TableBox>
            <table>
              <thead>
                <tr>
                  <th>{t('templates.stepTitle')}</th>
                  {base && <th>{t('templates.conditionColumn')}</th>}
                  <th>{t('common.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {steps.map((s, i) => (
                  <tr key={i}>
                    <td>
                      <input
                        aria-label={t('templates.stepTitleOf', { at: i + 1 })}
                        value={s.title}
                        required
                        onChange={(e) =>
                          setSteps(steps.map((x, j) => (j === i ? { ...x, title: e.target.value } : x)))
                        }
                      />
                    </td>
                    {base && (
                      <td>
                        <ConditionCell
                          condition={s.condition}
                          scope={scopeWith(base, fields, (f) => stepAt(f.name) < i)}
                          title={s.title || t('templates.stepTitleOf', { at: i + 1 })}
                          onSave={(condition) =>
                            setSteps(steps.map((x, j) => (j === i ? { ...x, condition } : x)))
                          }
                        />
                      </td>
                    )}
                    <td className="row">
                      <button
                        type="button"
                        className="icon"
                        aria-label={t('templates.moveStepUp', { at: i + 1 })}
                        title={t('templates.moveStepUp', { at: i + 1 })}
                        disabled={i === 0}
                        onClick={() => setSteps(moved(steps, i, -1))}
                      >
                        <IconArrowUp size={BACK_ICON_SIZE} />
                      </button>
                      <button
                        type="button"
                        className="icon"
                        aria-label={t('templates.moveStepDown', { at: i + 1 })}
                        title={t('templates.moveStepDown', { at: i + 1 })}
                        disabled={i === steps.length - 1}
                        onClick={() => setSteps(moved(steps, i, 1))}
                      >
                        <IconArrowDown size={BACK_ICON_SIZE} />
                      </button>
                      <button
                        type="button"
                        className="ghost"
                        onClick={() => setSteps(steps.filter((_, j) => j !== i))}
                      >
                        {t('templates.removeStep', { at: i + 1 })}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableBox>
        )}
        <p>
          <button
            type="button"
            disabled={steps.length >= 10}
            onClick={() =>
              setSteps([...steps, { title: t('templates.newStep', { at: steps.length + 1 }), fields: [] }])
            }
          >
            {t('templates.addStep')}
          </button>
        </p>
        <h2>{t('templates.questions')}</h2>
        {fields.length === 0 && <p className="muted">{t('templates.noQuestions')}</p>}
        {fields.length > 0 && (
          <TableBox>
            <table>
              <thead>
                <tr>
                  <th>{t('common.name')}</th>
                  <th>{t('templates.fieldKind')}</th>
                  <th>{t('organise.required')}</th>
                  <th>{t('templates.step')}</th>
                  {base && <th>{t('templates.conditionColumn')}</th>}
                  {guidedOn && <th>{t('common.actions')}</th>}
                </tr>
              </thead>
              <tbody>
                {fields.map((f) => (
                  <tr key={f.name}>
                    <td>{f.name}</td>
                    <td>{t(`templates.kind_${f.kind}`)}</td>
                    <td>{f.required ? t('common.yes') : t('common.no')}</td>
                    <td>
                      <select
                        aria-label={t('templates.stepOfQuestion', { name: f.name })}
                        value={stepOf(f.name)}
                        onChange={(e) => putInStep(f.name, Number(e.target.value))}
                      >
                        <option value={-1}>{t('templates.lastStep')}</option>
                        {steps.map((s, i) => (
                          <option key={i} value={i}>
                            {s.title || t('templates.stepTitleOf', { at: i + 1 })}
                          </option>
                        ))}
                      </select>
                    </td>
                    {base && (
                      <td>
                        <ConditionCell
                          condition={f.condition}
                          scope={scopeWith(base, fields, (g) => position(g.name) < position(f.name))}
                          title={f.name}
                          onSave={(condition) =>
                            setFields(fields.map((g) => (g.name === f.name ? { ...g, condition } : g)))
                          }
                        />
                      </td>
                    )}
                    {guidedOn && (
                      <td className="row">
                        <button type="button" className="ghost" onClick={() => setEditing(f)}>
                          {t('templates.editQuestion', { name: f.name })}
                        </button>
                        <button type="button" className="ghost" onClick={() => setRenaming(f.name)}>
                          {t('templates.renameQuestion', { name: f.name })}
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </TableBox>
        )}
        <div className="dialog-actions">
          {save.isSuccess && (
            <span role="status" className="muted">
              {t('editor.saved')}
            </span>
          )}
          {guidedOn && space && (
            <button type="button" onClick={() => setTrying(true)}>
              {t('templates.tryForm')}
            </button>
          )}
          <button type="submit" className="primary" disabled={save.isPending}>
            {t('common.save')}
          </button>
        </div>
      </form>
      {editing && (
        <FieldDialog
          start={editing}
          spaceId={template.space_id}
          onClose={() => setEditing(null)}
          onSave={(spec) => {
            // The condition is set in its own column, and the dialog keeps it as it was.
            setFields(fields.map((f) => (f.name === spec.name ? { ...spec, condition: f.condition } : f)));
            setEditing(null);
          }}
        />
      )}
      {renaming && (
        <RenameDialog
          templateId={template.id}
          name={renaming}
          onClose={() => setRenaming(null)}
          onRenamed={(form) => {
            setFields(form.fields.map(fieldSpecOf));
            setSteps(form.steps);
            setRenaming(null);
            onSaved();
          }}
        />
      )}
      {trying && space && (
        <TryForm template={{ ...template, guided: true }} space={space} onClose={() => setTrying(false)} />
      )}
    </>
  );
}

function VersionChanges({
  templateId,
  number,
  against,
}: {
  templateId: string;
  number: number;
  against: number;
}) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['template-changes', templateId, number, against],
    queryFn: () =>
      api.get<TemplateChanges>(
        `/templates/${templateId}/versions/${number}/changes${against > 0 ? `?against=${against}` : ''}`,
      ),
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const changed = q.data.entries.filter((e) => e.change !== 'same');
  const heading =
    against > 0 && against !== number - 1
      ? t('templates.changesAgainst', { number, against })
      : t('templates.changesOf', { number });
  return (
    <section aria-label={heading} className="template-changes">
      <h2>{heading}</h2>
      {q.data.settings && <p className="note">{t('templates.settingsChanged')}</p>}
      {changed.length === 0 && !q.data.settings && <p className="muted">{t('templates.nothingChanged')}</p>}
      {changed.map((e) => (
        <article key={e.key}>
          <h3>
            {e.name || t('templates.firstPage')}{' '}
            <span className="lozenge">{t(`templates.change_${e.change}`)}</span>
          </h3>
          <DiffDocument blocks={e.blocks} />
        </article>
      ))}
    </section>
  );
}

/** Who proposed and decided what about a version, and why (TPL-602). */
function DecisionsOf({ row }: { row: TemplateVersionRow }) {
  const { t } = useTranslation();
  const dates = useDates();
  if (row.decisions.length === 0) return null;
  return (
    <ul className="plain-list">
      {row.decisions.map((d) => (
        <li key={d.id}>
          {t(`templates.decided_${d.decision}`, { name: d.person_name })}{' '}
          <time className="muted" dateTime={d.at}>
            {dates.moment(d.at)}
          </time>
          {d.comment && <q className="comment-quote">{d.comment}</q>}
        </li>
      ))}
    </ul>
  );
}

/** Send back, with a reason the author reads (TPL-522, TPL-1030). */
function ReasonDialog({
  title,
  label,
  action,
  tone,
  pending,
  error,
  onSubmit,
  onClose,
}: {
  title: string;
  label: string;
  action: string;
  tone: 'primary' | 'danger';
  pending: boolean;
  error: unknown;
  onSubmit: (reason: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [reason, setReason] = useState('');
  return (
    <Dialog title={title} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          onSubmit(reason);
        }}
      >
        <ErrorNote error={error} />
        <Field label={label}>
          <textarea required value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className={tone} disabled={pending || reason.trim() === ''}>
            {action}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/**
 * The history of a template (ORG-060, TPL-602): each version with its state and decisions,
 * what it changed, and the actions of approval. A waiting version opens with the comparison to
 * the approved one, and Approve and Send back (TPL-1030).
 */
function VersionsSection({ template }: { template: TemplateSummary }) {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const guidedOn = useCapabilities().guided_templates;
  const [open, setOpen] = useState<number | null>(null);
  const [dialog, setDialog] = useState<'send_back' | 'withdraw' | null>(null);
  const key = ['template-versions', template.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<TemplateHistory>(`/templates/${template.id}/versions`),
  });
  const done = (next: TemplateHistory) => {
    client.setQueryData(key, next);
    setDialog(null);
    void client.invalidateQueries({ queryKey: ['template', template.id] });
  };
  const path = (number: number, action: string) => `/templates/${template.id}/versions/${number}/${action}`;
  const propose = useMutation({
    mutationFn: (number: number) => api.post<TemplateHistory>(path(number, 'propose')),
    onSuccess: done,
  });
  const approve = useMutation({
    mutationFn: (number: number) => api.post<TemplateHistory>(path(number, 'approve'), { comment: '' }),
    onSuccess: done,
  });
  const sendBack = useMutation({
    mutationFn: ({ number, reason }: { number: number; reason: string }) =>
      api.post<TemplateHistory>(path(number, 'send-back'), { reason }),
    onSuccess: done,
  });
  const withdraw = useMutation({
    mutationFn: (reason: string) =>
      api.post<TemplateHistory>(`/templates/${template.id}/withdraw`, { reason }),
    onSuccess: done,
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const history = q.data;
  const newest = history.versions[0];
  const waiting = newest?.state === 'waiting' ? newest : undefined;
  // A waiting version shows what it changes against the version in use (TPL-1030).
  const shown = open ?? (waiting && guidedOn ? waiting.number : null);
  const against = shown !== null && shown === waiting?.number ? history.approved : 0;
  const busy = propose.isPending || approve.isPending;
  return (
    <>
      <p className="muted">{t('templates.versionsHelp')}</p>
      <ErrorNote error={propose.error ?? approve.error} />
      {guidedOn && waiting && history.can.decide && (
        <div className="note" role="group" aria-label={t('templates.decisionOf', { number: waiting.number })}>
          <p>{t('templates.waitsForYou', { number: waiting.number, name: waiting.author_name })}</p>
          <div className="dialog-actions">
            <button type="button" onClick={() => setDialog('send_back')}>
              {t('templates.sendBack')}
            </button>
            <button
              type="button"
              className="primary"
              disabled={busy}
              onClick={() => approve.mutate(waiting.number)}
            >
              {t('templates.approve', { number: waiting.number })}
            </button>
          </div>
        </div>
      )}
      {guidedOn && waiting && !history.can.decide && (
        <p className="note">
          {history.can.nobody_else
            ? t('templates.nobodyElse')
            : t('templates.waitsForOthers', { number: waiting.number })}
        </p>
      )}
      {guidedOn && newest && history.can.propose && (
        <p>
          <button
            type="button"
            className="primary"
            disabled={busy}
            onClick={() => propose.mutate(newest.number)}
          >
            {t('templates.propose', { number: newest.number })}
          </button>
        </p>
      )}
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('templates.version')}</th>
              {guidedOn && <th>{t('templates.stateColumn')}</th>}
              <th>{t('templates.author')}</th>
              <th>{t('templates.when')}</th>
              <th>{t('templates.pageCount')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {history.versions.map((v) => (
              <tr key={v.number}>
                <td>{v.number}</td>
                {guidedOn && (
                  <td>
                    <TemplateStateMark state={v.state} />
                    <DecisionsOf row={v} />
                  </td>
                )}
                <td>{v.author_name}</td>
                <td>
                  <time dateTime={v.created_at}>{dates.moment(v.created_at)}</time>
                </td>
                <td>{v.pages}</td>
                <td>
                  <button
                    type="button"
                    className="ghost"
                    aria-pressed={shown === v.number}
                    onClick={() => setOpen(shown === v.number ? -1 : v.number)}
                  >
                    {t('templates.showChanges', { number: v.number })}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      {shown !== null && shown > 0 && (
        <VersionChanges templateId={template.id} number={shown} against={against} />
      )}
      {guidedOn && history.can.withdraw && (
        <section aria-labelledby="withdraw-heading">
          <h2 id="withdraw-heading">{t('templates.withdrawTitle')}</h2>
          <p className="muted">{t('templates.withdrawHelp')}</p>
          <button type="button" className="danger" onClick={() => setDialog('withdraw')}>
            {t('templates.withdraw')}
          </button>
        </section>
      )}
      {dialog === 'send_back' && waiting && (
        <ReasonDialog
          title={t('templates.sendBackTitle', { number: waiting.number })}
          label={t('templates.sendBackReason')}
          action={t('templates.sendBack')}
          tone="primary"
          pending={sendBack.isPending}
          error={sendBack.error}
          onSubmit={(reason) => sendBack.mutate({ number: waiting.number, reason })}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog === 'withdraw' && (
        <ReasonDialog
          title={t('templates.withdrawTitle')}
          label={t('templates.withdrawReason')}
          action={t('templates.withdrawConfirm')}
          tone="danger"
          pending={withdraw.isPending}
          error={withdraw.error}
          onSubmit={(reason) => withdraw.mutate(reason)}
          onClose={() => setDialog(null)}
        />
      )}
    </>
  );
}
function SettingsHead({ template, space }: { template: TemplateSummary; space: Space | null }) {
  const { t } = useTranslation();
  return (
    <>
      <Link
        href={space ? `/spaces/${space.key}/templates` : '/admin/templates'}
        className="side-link settings-back"
      >
        <IconChevronLeft size={BACK_ICON_SIZE} /> {t('templates.backToList')}
      </Link>
      <p className="settings-title">{template.name}</p>
    </>
  );
}

/** The settings of one template, in the Settings frame (TPL-1010). */
export function TemplateSettingsView({ templateId }: { templateId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const asked = useLocation().searchParams.get('section');
  const section: TemplateSection = TEMPLATE_SECTIONS.find((s) => s === asked) ?? 'general';
  const guidedOn = useCapabilities().guided_templates;
  const q = useQuery({
    queryKey: ['template', templateId],
    // The settings change the newest version, which may wait while another is in use (TPL-523).
    queryFn: () => api.get<TemplateSummary>(`/templates/${templateId}/newest`),
  });
  const spaceId = q.data?.space_id ?? null;
  const space = useQuery({
    queryKey: ['space-id', spaceId],
    queryFn: () => api.get<Space>(`/spaces/${spaceId}`),
    enabled: !!spaceId,
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const template = q.data;
  const shownSpace = space.data ?? null;
  const refresh = () => void client.invalidateQueries({ queryKey: ['template', templateId] });
  const item = (id: TemplateSection) => ({
    id,
    label: t(`templates.section_${id}`),
    href: `/templates/${templateId}/settings?section=${id}`,
  });
  return (
    <SettingsFrame
      title={t('templates.settingsOf', { name: template.name })}
      head={<SettingsHead template={template} space={shownSpace} />}
      groups={[
        {
          id: 'template',
          title: template.name,
          // Who can use it is rights per template, a part of guided templates (TPL-050).
          items: TEMPLATE_SECTIONS.filter((s) => guidedOn || s !== 'access').map(item),
        },
      ]}
      current={section}
    >
      <h1>{t(`templates.section_${section}`)}</h1>
      <p className="muted template-narrow-note">{t('templates.largerScreen')}</p>
      {section === 'general' && (spaceId === null || shownSpace) && (
        <GeneralSection key={template.id} template={template} space={shownSpace} onSaved={refresh} />
      )}
      {section === 'pages' && <PagesSection templateId={templateId} />}
      {section === 'form' && (
        <FormSection key={template.id} template={template} space={shownSpace} onSaved={refresh} />
      )}
      {section === 'rules' && <RulesSection templateId={templateId} />}
      {section === 'access' && guidedOn && <TemplateAccessSection templateId={templateId} />}
      {section === 'versions' && <VersionsSection template={template} />}
    </SettingsFrame>
  );
}

function PagesSection({ templateId }: { templateId: string }) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  if (!guidedOn) return <p className="muted">{t('templates.pagesNeedGuided')}</p>;
  return <TemplatePages templateId={templateId} />;
}
