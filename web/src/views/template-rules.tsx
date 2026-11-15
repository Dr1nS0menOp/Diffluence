// SPDX-License-Identifier: Apache-2.0
// Conditions in the settings of a template (spec 074, wave GT3): what a condition may test
// where it stands, its sentence in a row, and the Rules section, which holds the label rules
// (TPL-423, TPL-1010).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  api,
  type LabelRule,
  type Space,
  type TemplateEntries,
  type TemplateField,
  type TemplateStep,
  type TemplateSummary,
} from '../api';
import { useCapabilities } from '../capabilities';
import {
  ConditionDialog,
  sentenceWords,
  type ConditionScope,
  type NamedField,
} from '../components/condition-editor';
import { TableBox } from '../components/table-box';
import { Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { conditionSentence, type Condition } from '../template-conditions';
import { fieldSpecOf, type FieldSpec } from '../template-fields';

/** A question as a condition tests it. */
const named = (f: FieldSpec): NamedField => ({ name: f.name, kind: f.kind, options: f.options });

/** The questions in the order the form asks them, each with its step (stepsOf in the form). */
export function formOrder(fields: FieldSpec[], steps: TemplateStep[]): { field: FieldSpec; step: number }[] {
  const out: { field: FieldSpec; step: number }[] = [];
  const placed = new Set<string>();
  steps.forEach((s, i) => {
    for (const f of fields) {
      if (s.fields.includes(f.name) && !placed.has(f.name)) {
        out.push({ field: f, step: i });
        placed.add(f.name);
      }
    }
  });
  for (const f of fields) if (!placed.has(f.name)) out.push({ field: f, step: steps.length });
  return out;
}

/** What a condition may test, with every other question named as later (TPL-421). */
export function scopeWith(
  base: ConditionScope,
  all: FieldSpec[],
  allowed: (f: FieldSpec) => boolean,
): ConditionScope {
  return {
    ...base,
    fields: all.filter(allowed).map(named),
    later: all.filter((f) => !allowed(f)).map((f) => f.name),
  };
}

/**
 * The scope of the conditions of a template that test every question: those of its pages,
 * sections and rules. Undefined while guided templates are off or the template is loading.
 */
export function useConditionScope(templateId: string): ConditionScope | undefined {
  const guidedOn = useCapabilities().guided_templates;
  const template = useQuery({
    queryKey: ['template', templateId],
    queryFn: () => api.get<TemplateSummary>(`/templates/${templateId}`),
    enabled: guidedOn,
  });
  const entries = useQuery({
    queryKey: ['template-entries', templateId],
    queryFn: () => api.get<TemplateEntries>(`/templates/${templateId}/entries`),
    enabled: guidedOn,
  });
  const allSpaces = template.data?.space_id === null;
  const spaces = useQuery({
    queryKey: ['spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/spaces'),
    enabled: guidedOn && allSpaces,
  });
  if (!guidedOn || !template.data || !entries.data) return undefined;
  return {
    fields: (template.data.fields ?? []).map(fieldSpecOf).map(named),
    later: [],
    space: allSpaces,
    names: entries.data.names,
    spaces: (spaces.data?.spaces ?? []).map((s) => ({ id: s.id, name: s.name })),
  };
}

/** A condition in a row: its sentence, or "Always", and a button that changes it. */
export function ConditionCell({
  condition,
  scope,
  title,
  onSave,
  disabled,
}: {
  condition: Condition | undefined;
  scope: ConditionScope;
  title: string;
  onSave: (next: Condition | undefined) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <span className={condition ? undefined : 'muted'}>
        {condition ? conditionSentence(condition, sentenceWords(scope)) : t('templates.condition.always')}
      </span>{' '}
      <button type="button" className="ghost" disabled={disabled} onClick={() => setOpen(true)}>
        {condition ? t('templates.condition.change') : t('templates.condition.add')}
      </button>
      {open && (
        <ConditionDialog
          title={t('templates.condition.dialogTitle', { name: title })}
          start={condition}
          scope={scope}
          onClose={() => setOpen(false)}
          onSave={(next) => {
            setOpen(false);
            onSave(next);
          }}
        />
      )}
    </>
  );
}

type RuleKind = 'label' | 'field';

/** The label rules of a template (TPL-423): each adds a label to a page, always or when its condition holds. */
export function RulesSection({ templateId }: { templateId: string }) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const scope = useConditionScope(templateId);
  const entries = useQuery({
    queryKey: ['template-entries', templateId],
    queryFn: () => api.get<TemplateEntries>(`/templates/${templateId}/entries`),
    enabled: guidedOn,
  });
  if (!guidedOn) return <p className="muted">{t('templates.rulesNeedGuided')}</p>;
  if (entries.error) return <ErrorNote error={entries.error} />;
  if (!entries.data || !scope) return <Loading />;
  return (
    <RulesForm key={entries.data.version} templateId={templateId} entries={entries.data} scope={scope} />
  );
}

function RulesForm({
  templateId,
  entries,
  scope,
}: {
  templateId: string;
  entries: TemplateEntries;
  scope: ConditionScope;
}) {
  const { t } = useTranslation();
  const [rules, setRules] = useState<LabelRule[]>(entries.label_rules);
  const save = useMutation({
    mutationFn: () =>
      api.put<{ label_rules: LabelRule[] }>(`/templates/${templateId}/rules`, { label_rules: rules }),
  });
  const labelQuestions = scope.fields.filter((f) => f.kind === 'label');
  const nameOf = (key: string) =>
    entries.entries.find((e) => e.key === key)?.name || t('templates.firstPage');
  const set = (i: number, next: LabelRule) => setRules(rules.map((r, j) => (j === i ? next : r)));
  return (
    <form
      className="settings-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <p className="muted">{t('templates.rulesHelp')}</p>
      <ErrorNote error={save.error} />
      <h2>{t('templates.labelRules')}</h2>
      {rules.length === 0 && <p className="muted">{t('templates.noRules')}</p>}
      {rules.length > 0 && (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('templates.rulePage')}</th>
                <th>{t('templates.ruleAdds')}</th>
                <th>{t('templates.ruleWhen')}</th>
                <th>{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((rule, i) => {
                const kind: RuleKind = rule.field !== undefined ? 'field' : 'label';
                return (
                  <tr key={i}>
                    <td>
                      <select
                        aria-label={t('templates.rulePageOf', { at: i + 1 })}
                        value={rule.entry}
                        onChange={(e) => set(i, { ...rule, entry: e.target.value })}
                      >
                        {entries.entries.map((e) => (
                          <option key={e.key} value={e.key}>
                            {nameOf(e.key)}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>
                      <div className="row">
                        <select
                          aria-label={t('templates.ruleKind', { at: i + 1 })}
                          value={kind}
                          onChange={(e) =>
                            set(
                              i,
                              e.target.value === 'field'
                                ? { entry: rule.entry, when: rule.when, field: labelQuestions[0]?.name ?? '' }
                                : { entry: rule.entry, when: rule.when, label: '' },
                            )
                          }
                        >
                          <option value="label">{t('templates.ruleKindFixed')}</option>
                          <option value="field" disabled={labelQuestions.length === 0}>
                            {t('templates.ruleKindField')}
                          </option>
                        </select>
                        {kind === 'label' ? (
                          <Field label={t('templates.ruleLabel', { at: i + 1 })}>
                            <input
                              value={rule.label ?? ''}
                              required
                              onChange={(e) => set(i, { ...rule, label: e.target.value })}
                            />
                          </Field>
                        ) : (
                          <Field label={t('templates.ruleField', { at: i + 1 })}>
                            <select
                              value={rule.field ?? ''}
                              onChange={(e) => set(i, { ...rule, field: e.target.value })}
                            >
                              {labelQuestions.map((f) => (
                                <option key={f.name} value={f.name}>
                                  {f.name}
                                </option>
                              ))}
                            </select>
                          </Field>
                        )}
                      </div>
                    </td>
                    <td>
                      <ConditionCell
                        condition={rule.when}
                        scope={scope}
                        title={t('templates.ruleName', { at: i + 1 })}
                        onSave={(when) => set(i, { ...rule, when })}
                      />
                    </td>
                    <td>
                      <button
                        type="button"
                        className="ghost"
                        onClick={() => setRules(rules.filter((_, j) => j !== i))}
                      >
                        {t('templates.removeRule', { at: i + 1 })}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </TableBox>
      )}
      {labelQuestions.length === 0 && <p className="muted">{t('templates.ruleNoLabelQuestion')}</p>}
      <p>
        <button
          type="button"
          disabled={rules.length >= 50}
          onClick={() => setRules([...rules, { entry: 'root', label: '' }])}
        >
          {t('templates.addRule')}
        </button>
      </p>
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

/** The form of a template as the server gives it after a change. */
interface FormAnswer {
  fields: TemplateField[];
  steps: TemplateStep[];
}

/**
 * Renames a question everywhere the template uses it, as one new version (TPL-230): in every
 * text, title pattern, step and condition. The server refuses a name in use or kept.
 */
export function RenameDialog({
  templateId,
  name,
  onRenamed,
  onClose,
}: {
  templateId: string;
  name: string;
  onRenamed: (form: FormAnswer) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [to, setTo] = useState(name);
  const rename = useMutation({
    mutationFn: () => api.post<FormAnswer>(`/templates/${templateId}/fields/rename`, { from: name, to }),
    onSuccess: onRenamed,
  });
  return (
    <Dialog title={t('templates.renameTitle', { name })} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          e.stopPropagation();
          rename.mutate();
        }}
      >
        <ErrorNote error={rename.error} />
        <Field label={t('templates.renameTo')} help={t('templates.renameHelp')}>
          <input value={to} required autoFocus onChange={(e) => setTo(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="primary" disabled={rename.isPending}>
            {t('templates.renameDone')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
