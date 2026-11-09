// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Space } from '../api';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { ConfirmButton, Dialog, ErrorNote, Field, FilePicker, Loading, RelativeTime } from '../components/ui';
import { Link, navigate } from '../router';
import { TableBox } from '../components/table-box';

export interface RuleAction {
  type: string;
  text?: string;
  label?: string;
  to?: string;
  target_id?: string;
  title?: string;
  older_days?: number;
  address?: string;
  body?: string;
  restricted?: string;
}
interface Condition {
  kind: string;
  value?: string;
  days?: number;
  items?: Condition[];
}
export interface Definition {
  trigger: { kind: string; every?: string; at?: string };
  conditions?: Condition;
  actions: RuleAction[];
  /** For a rule of the instance: the spaces it acts in, or none for every space. */
  spaces?: string[];
}
interface Problem {
  step: number;
  message: string;
}
interface Rule {
  id: string;
  space_id: string | null;
  name: string;
  enabled: boolean;
  definition: Definition;
  owner_name: string;
  paused_at: string | null;
  last_run_at: string | null;
  last_state: string;
  has_hook: boolean;
  problems: Problem[];
}
interface Step {
  key: string;
  action: string;
  state: string;
  note?: string;
}
interface Run {
  id: string;
  state: string;
  report: string;
  page_title: string | null;
  steps: Step[];
  created_at: string;
}

const TRIGGERS = [
  'page.published',
  'page.edited',
  'page.moved',
  'page.copied',
  'page.archived',
  'page.deleted',
  'page.commented',
  'page.labelled',
  'page.status_changed',
  'page.owner_changed',
  'blog.published',
  'blog.commented',
  'blog.labelled',
  'attachment.added',
  'attachment.removed',
  'task.created',
  'task.ticked',
  'person.mentioned',
  'group.member_added',
  'group.member_removed',
  'space.created',
  'space.archived',
  'schedule',
  'manual',
  'incoming',
];
const ACTIONS = [
  'find_pages',
  'add_label',
  'remove_label',
  'set_status',
  'add_comment',
  'notify',
  'email',
  'assign_task',
  'add_watcher',
  'remove_watcher',
  'archive',
  'delete',
  'copy',
  'webhook',
  'chat',
];
const CONDITION_KINDS = ['has_label', 'has_status', 'title_contains', 'text_contains', 'older_than'];
const SCHEDULES = ['hourly', 'daily', 'weekly', 'monthly'];
const VALUES = [
  '{page.title}',
  '{page.url}',
  '{page.author}',
  '{page.owner}',
  '{found.title}',
  '{rule.name}',
];
const RECIPIENTS = ['owner', 'author'];
const TRIGGER_STEP = -1;
const CONDITION_STEP = -2;

/** Ready-made rules (PLT-033): people want a start more than an empty form. */
export const RULE_TEMPLATES: { key: string; definition: Definition }[] = [
  {
    key: 'remindOwner',
    definition: {
      trigger: { kind: 'schedule', every: 'weekly', at: '09:00' },
      actions: [
        { type: 'find_pages', older_days: 365 },
        { type: 'notify', to: 'owner', text: '{found.title} has not changed for a year.' },
      ],
    },
  },
  {
    key: 'labelNew',
    definition: { trigger: { kind: 'page.published' }, actions: [{ type: 'add_label', label: 'new' }] },
  },
  {
    key: 'tellOnPost',
    definition: {
      trigger: { kind: 'blog.published' },
      actions: [{ type: 'notify', to: 'owner', text: 'New blog post: {page.title}' }],
    },
  },
  {
    key: 'askReview',
    definition: {
      trigger: { kind: 'page.status_changed' },
      actions: [{ type: 'notify', to: 'owner', text: 'Please review {page.title}.' }],
    },
  },
  {
    key: 'manyComments',
    definition: {
      trigger: { kind: 'page.commented' },
      actions: [{ type: 'notify', to: 'owner', text: '{page.title} has a new comment.' }],
    },
  },
];

const emptyDefinition: Definition = {
  trigger: { kind: 'page.published' },
  actions: [{ type: 'add_label', label: '' }],
};

function ruleList(spaceId: string | null) {
  return spaceId ? `/spaces/${spaceId}/rules` : '/admin/rules';
}

/** Moves a step, for those who do not drag (WCAG 2.5.7). */
export function moveStep<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length) return list;
  const next = [...list];
  const [item] = next.splice(from, 1);
  if (item !== undefined) next.splice(to, 0, item);
  return next;
}

function saveExport(rule: Rule) {
  const link = document.createElement('a');
  const raw = JSON.stringify({ name: rule.name, definition: rule.definition }, null, 2);
  link.href = URL.createObjectURL(new Blob([raw], { type: 'application/json' }));
  link.download = `${rule.name}.json`;
  link.click();
  URL.revokeObjectURL(link.href);
}

function RuleRow({ rule, spaceId }: { rule: Rule; spaceId: string | null }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const refresh = () => client.invalidateQueries({ queryKey: ['rules', spaceId] });
  const toggle = useMutation({
    mutationFn: () =>
      api.put(`/rules/${rule.id}`, { name: rule.name, enabled: !rule.enabled, definition: rule.definition }),
    onSettled: refresh,
  });
  const copy = useMutation({
    mutationFn: () =>
      api.post(ruleList(spaceId), {
        name: t('rules2.copyName', { name: rule.name }),
        enabled: false,
        definition: rule.definition,
      }),
    onSettled: refresh,
  });
  const remove = useMutation({ mutationFn: () => api.del(`/rules/${rule.id}`), onSettled: refresh });
  return (
    <tr>
      <td>
        <Link href={`/rules/${rule.id}`}>{rule.name}</Link>
      </td>
      <td>{t(`rules2.trigger_${rule.definition.trigger.kind.replace('.', '_')}`)}</td>
      <td>
        {rule.paused_at ? (
          <span className="lozenge">{t('rules2.paused')}</span>
        ) : rule.enabled ? (
          t('rules2.on')
        ) : (
          t('rules2.off')
        )}
      </td>
      <td>{rule.last_run_at ? <RelativeTime value={rule.last_run_at} /> : t('rules2.never')}</td>
      <td>{rule.owner_name}</td>
      <td>
        <span className="row">
          <button type="button" className="ghost" onClick={() => toggle.mutate()}>
            {rule.enabled ? t('rules2.turnOff') : t('rules2.turnOn')}
          </button>
          <button type="button" className="ghost" onClick={() => copy.mutate()}>
            {t('rules2.copy')}
          </button>
          <button type="button" className="ghost" onClick={() => saveExport(rule)}>
            {t('rules2.export')}
          </button>
          <ConfirmButton
            label={t('common.delete')}
            message={t('rules2.deleteConfirm', { name: rule.name })}
            className="ghost"
            onConfirm={() => remove.mutate()}
          />
        </span>
        <ErrorNote error={toggle.error ?? copy.error ?? remove.error} />
      </td>
    </tr>
  );
}

function NewRule({ spaceId }: { spaceId: string | null }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const make = useMutation({
    mutationFn: (v: { name: string; definition: Definition }) =>
      api.post<Rule>(ruleList(spaceId), { name: v.name, enabled: false, definition: v.definition }),
    onSuccess: (rule) => navigate(`/rules/${rule.id}`),
  });
  return (
    <>
      <button type="button" className="primary" onClick={() => setOpen(true)}>
        {t('rules2.new')}
      </button>
      {open && (
        <Dialog title={t('rules2.new')} onClose={() => setOpen(false)}>
          <ErrorNote error={make.error} />
          <ul className="plain-list">
            <li>
              <button
                type="button"
                className="ghost"
                onClick={() => make.mutate({ name: t('rules2.newName'), definition: emptyDefinition })}
              >
                {t('rules2.fromEmpty')}
              </button>
            </li>
            {RULE_TEMPLATES.map((template) => (
              <li key={template.key}>
                <button
                  type="button"
                  className="ghost"
                  onClick={() =>
                    make.mutate({
                      name: t(`rules2.template_${template.key}`),
                      definition: template.definition,
                    })
                  }
                >
                  {t(`rules2.template_${template.key}`)}
                </button>
              </li>
            ))}
          </ul>
        </Dialog>
      )}
    </>
  );
}

function ImportRule({ spaceId }: { spaceId: string | null }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const load = useMutation({
    mutationFn: async (file: File) => {
      const parsed = JSON.parse(await file.text()) as { name: string; definition: Definition };
      return api.post(ruleList(spaceId), {
        name: parsed.name,
        enabled: false,
        definition: parsed.definition,
      });
    },
    onSettled: () => client.invalidateQueries({ queryKey: ['rules', spaceId] }),
  });
  return (
    <>
      <FilePicker label={t('rules2.import')} accept="application/json" onPick={(file) => load.mutate(file)} />
      <ErrorNote error={load.error} />
    </>
  );
}

/** The rules of a space, or of the instance. */
export function RuleList({ spaceId }: { spaceId: string | null }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['rules', spaceId],
    queryFn: () => api.get<{ rules: Rule[] }>(ruleList(spaceId)),
  });
  return (
    <>
      <div className="row">
        <NewRule spaceId={spaceId} />
        <ImportRule spaceId={spaceId} />
      </div>
      <ErrorNote error={q.error} />
      {q.isPending && <Loading />}
      {q.data?.rules.length === 0 && <p className="muted">{t('rules2.none')}</p>}
      {(q.data?.rules.length ?? 0) > 0 && (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('common.name')}</th>
                <th>{t('rules2.when')}</th>
                <th>{t('rules2.state')}</th>
                <th>{t('rules2.lastRun')}</th>
                <th>{t('rules2.owner')}</th>
                <th>{t('rules2.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.rules.map((r) => (
                <RuleRow key={r.id} rule={r} spaceId={spaceId} />
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
    </>
  );
}

/** Space settings, Automation. */
export function RulesView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!q.data) return <Loading />;
  return (
    <Frame space={q.data}>
      <h1>{t('rules2.title')}</h1>
      <RuleList spaceId={q.data.id} />
    </Frame>
  );
}

function StepMessage({ problems, step }: { problems: Problem[]; step: number }) {
  return (
    <>
      {problems
        .filter((p) => p.step === step)
        .map((p) => (
          <p key={p.message} className="note note-error">
            {p.message}
          </p>
        ))}
    </>
  );
}

function ValuePicker({ onPick }: { onPick: (value: string) => void }) {
  const { t } = useTranslation();
  return (
    <Field label={t('rules2.insertValue')}>
      <select
        value=""
        onChange={(e) => {
          if (e.target.value) onPick(e.target.value);
        }}
      >
        <option value="">{t('rules2.chooseValue')}</option>
        {VALUES.map((v) => (
          <option key={v} value={v}>
            {v}
          </option>
        ))}
      </select>
    </Field>
  );
}

function ActionFields({ action, onChange }: { action: RuleAction; onChange: (next: RuleAction) => void }) {
  const { t } = useTranslation();
  const set = (part: Partial<RuleAction>) => onChange({ ...action, ...part });
  const hasRecipient = ['notify', 'email', 'assign_task', 'add_watcher', 'remove_watcher'].includes(
    action.type,
  );
  const hasText = ['set_status', 'add_comment', 'notify', 'email', 'assign_task', 'chat'].includes(
    action.type,
  );
  const hasAddress = action.type === 'webhook' || action.type === 'chat';
  return (
    <>
      {action.type === 'find_pages' && (
        <Field label={t('rules2.olderDays')}>
          <input
            type="number"
            min={0}
            value={action.older_days ?? 0}
            onChange={(e) => set({ older_days: Number(e.target.value) })}
          />
        </Field>
      )}
      {(action.type === 'add_label' || action.type === 'remove_label' || action.type === 'find_pages') && (
        <Field label={t('rules2.label')}>
          <input value={action.label ?? ''} onChange={(e) => set({ label: e.target.value })} />
        </Field>
      )}
      {hasRecipient && (
        <Field label={t('rules2.to')}>
          <select value={action.to ?? ''} onChange={(e) => set({ to: e.target.value })}>
            <option value="">{t('rules2.chooseWho')}</option>
            {RECIPIENTS.map((r) => (
              <option key={r} value={r}>
                {t(`rules2.to_${r}`)}
              </option>
            ))}
          </select>
        </Field>
      )}
      {hasAddress && (
        <Field label={t('rules2.address')} help={t('rules2.addressHelp')}>
          <input type="url" value={action.address ?? ''} onChange={(e) => set({ address: e.target.value })} />
        </Field>
      )}
      {action.type === 'webhook' && (
        <Field label={t('rules2.body')}>
          <textarea value={action.body ?? ''} onChange={(e) => set({ body: e.target.value })} />
        </Field>
      )}
      {hasText && (
        <>
          <Field label={t('rules2.text')}>
            <textarea value={action.text ?? ''} onChange={(e) => set({ text: e.target.value })} />
          </Field>
          <ValuePicker onPick={(v) => set({ text: `${action.text ?? ''}${v}` })} />
        </>
      )}
    </>
  );
}

/** Where a rule of the instance acts: in every space, or in the spaces chosen (spec 033). */
function ScopePart({
  definition,
  onChange,
}: {
  definition: Definition;
  onChange: (next: Definition) => void;
}) {
  const { t } = useTranslation();
  const spaces = useQuery({
    queryKey: ['admin-spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/admin/spaces'),
  });
  const chosen = definition.spaces ?? [];
  const [choosing, setChoosing] = useState(chosen.length > 0);
  const set = (next: string[]) => onChange({ ...definition, spaces: next });
  const nameOf = (id: string) => spaces.data?.spaces.find((s) => s.id === id)?.name ?? id;
  const left = (spaces.data?.spaces ?? []).filter((s) => !chosen.includes(s.id));
  return (
    <section>
      <h2>{t('rules2.where')}</h2>
      <fieldset className="choice-row">
        <legend className="visually-hidden">{t('rules2.where')}</legend>
        <label className="check">
          <input
            type="radio"
            name="rule-scope"
            checked={!choosing}
            onChange={() => {
              setChoosing(false);
              set([]);
            }}
          />
          {t('rules2.everySpace')}
        </label>
        <label className="check">
          <input type="radio" name="rule-scope" checked={choosing} onChange={() => setChoosing(true)} />
          {t('rules2.chosenSpaces')}
        </label>
      </fieldset>
      {choosing && (
        <>
          <ul className="chip-list">
            {chosen.map((id) => (
              <li key={id} className="chip">
                {nameOf(id)}
                <button
                  type="button"
                  className="ghost"
                  aria-label={t('rules2.removeSpace', { name: nameOf(id) })}
                  onClick={() => set(chosen.filter((s) => s !== id))}
                >
                  {t('common.remove')}
                </button>
              </li>
            ))}
          </ul>
          <Field label={t('rules2.addSpace')}>
            <select value="" onChange={(e) => e.target.value && set([...chosen, e.target.value])}>
              <option value="">{t('rules2.pickSpace')}</option>
              {left.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>
          {chosen.length === 0 && <p className="muted">{t('rules2.noSpaceYet')}</p>}
        </>
      )}
    </section>
  );
}

function TriggerPart({
  definition,
  problems,
  onChange,
}: {
  definition: Definition;
  problems: Problem[];
  onChange: (next: Definition) => void;
}) {
  const { t } = useTranslation();
  const trigger = definition.trigger;
  const set = (part: Partial<Definition['trigger']>) =>
    onChange({ ...definition, trigger: { ...trigger, ...part } });
  return (
    <section>
      <h2>{t('rules2.when')}</h2>
      <StepMessage problems={problems} step={TRIGGER_STEP} />
      <Field label={t('rules2.trigger')}>
        <select value={trigger.kind} onChange={(e) => set({ kind: e.target.value })}>
          {TRIGGERS.map((k) => (
            <option key={k} value={k}>
              {t(`rules2.trigger_${k.replace('.', '_')}`)}
            </option>
          ))}
        </select>
      </Field>
      {trigger.kind === 'schedule' && (
        <>
          <Field label={t('rules2.every')}>
            <select value={trigger.every ?? ''} onChange={(e) => set({ every: e.target.value })}>
              <option value="">{t('rules2.chooseEvery')}</option>
              {SCHEDULES.map((s) => (
                <option key={s} value={s}>
                  {t(`rules2.every_${s}`)}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t('rules2.at')} help={t('rules2.atHelp')}>
            <input type="time" value={trigger.at ?? ''} onChange={(e) => set({ at: e.target.value })} />
          </Field>
        </>
      )}
    </section>
  );
}

function ConditionPart({
  definition,
  problems,
  onChange,
}: {
  definition: Definition;
  problems: Problem[];
  onChange: (next: Definition) => void;
}) {
  const { t } = useTranslation();
  const group = definition.conditions ?? { kind: 'all', items: [] };
  const items = group.items ?? [];
  const setGroup = (next: Condition) =>
    onChange({ ...definition, conditions: (next.items ?? []).length > 0 ? next : undefined });
  const setItem = (index: number, next: Condition) =>
    setGroup({ ...group, items: items.map((item, i) => (i === index ? next : item)) });
  return (
    <section>
      <h2>{t('rules2.if')}</h2>
      <StepMessage problems={problems} step={CONDITION_STEP} />
      <Field label={t('rules2.group')}>
        <select value={group.kind} onChange={(e) => setGroup({ ...group, kind: e.target.value })}>
          <option value="all">{t('rules2.group_all')}</option>
          <option value="any">{t('rules2.group_any')}</option>
        </select>
      </Field>
      {items.map((item, i) => (
        <fieldset key={i} className="rule-step">
          <legend>{t('rules2.conditionNumber', { number: i + 1 })}</legend>
          <Field label={t('rules2.condition')}>
            <select value={item.kind} onChange={(e) => setItem(i, { ...item, kind: e.target.value })}>
              {CONDITION_KINDS.map((k) => (
                <option key={k} value={k}>
                  {t(`rules2.condition_${k}`)}
                </option>
              ))}
            </select>
          </Field>
          {item.kind === 'older_than' ? (
            <Field label={t('rules2.days')}>
              <input
                type="number"
                min={1}
                value={item.days ?? 1}
                onChange={(e) => setItem(i, { ...item, days: Number(e.target.value) })}
              />
            </Field>
          ) : (
            <Field label={t('rules2.value')}>
              <input
                value={item.value ?? ''}
                onChange={(e) => setItem(i, { ...item, value: e.target.value })}
              />
            </Field>
          )}
          <button
            type="button"
            onClick={() => setGroup({ ...group, items: items.filter((_, k) => k !== i) })}
          >
            {t('common.remove')}
          </button>
        </fieldset>
      ))}
      <button
        type="button"
        onClick={() => setGroup({ ...group, items: [...items, { kind: 'has_label', value: '' }] })}
      >
        {t('rules2.addCondition')}
      </button>
    </section>
  );
}

function ActionPart({
  definition,
  problems,
  onChange,
}: {
  definition: Definition;
  problems: Problem[];
  onChange: (next: Definition) => void;
}) {
  const { t } = useTranslation();
  const actions = definition.actions;
  const setActions = (next: RuleAction[]) => onChange({ ...definition, actions: next });
  return (
    <section>
      <h2>{t('rules2.then')}</h2>
      {actions.map((action, i) => (
        <fieldset key={i} className="rule-step">
          <legend>{t('rules2.stepNumber', { number: i + 1 })}</legend>
          <StepMessage problems={problems} step={i} />
          <Field label={t('rules2.action')}>
            <select
              value={action.type}
              onChange={(e) => setActions(actions.map((a, k) => (k === i ? { type: e.target.value } : a)))}
            >
              {ACTIONS.map((a) => (
                <option key={a} value={a}>
                  {t(`rules2.action_${a}`)}
                </option>
              ))}
            </select>
          </Field>
          <ActionFields
            action={action}
            onChange={(next) => setActions(actions.map((a, k) => (k === i ? next : a)))}
          />
          <div className="row">
            <button type="button" disabled={i === 0} onClick={() => setActions(moveStep(actions, i, i - 1))}>
              {t('rules2.moveUp')}
            </button>
            <button
              type="button"
              disabled={i === actions.length - 1}
              onClick={() => setActions(moveStep(actions, i, i + 1))}
            >
              {t('rules2.moveDown')}
            </button>
            <button type="button" onClick={() => setActions(actions.filter((_, k) => k !== i))}>
              {t('common.remove')}
            </button>
          </div>
        </fieldset>
      ))}
      <button type="button" onClick={() => setActions([...actions, { type: 'add_label', label: '' }])}>
        {t('rules2.addStep')}
      </button>
    </section>
  );
}

function TestRun({ rule }: { rule: Rule }) {
  const { t } = useTranslation();
  const [pageId, setPageId] = useState('');
  const run = useMutation({
    mutationFn: () =>
      api.post<{ outcome: string; steps: Step[]; warnings: string[] }>(`/rules/${rule.id}/test`, {
        page_id: pageId || null,
      }),
  });
  return (
    <section>
      <h2>{t('rules2.testTitle')}</h2>
      <Field label={t('rules2.testPage')} help={t('rules2.testHelp')}>
        <input value={pageId} onChange={(e) => setPageId(e.target.value.trim())} />
      </Field>
      <button type="button" onClick={() => run.mutate()}>
        {t('rules2.test')}
      </button>
      <ErrorNote error={run.error} />
      {run.data && (
        <div role="status">
          <p>{t(`rules2.outcome_${run.data.outcome}`)}</p>
          <ul className="plain-list">
            {run.data.steps.map((s) => (
              <li key={s.key}>
                {t(`rules2.action_${s.action}`)}: {t(`rules2.note_${s.note ?? 'none'}`)}
              </li>
            ))}
          </ul>
          {run.data.warnings.map((w) => (
            <p key={w} className="note">
              {t(`rules2.warning_${w}`)}
            </p>
          ))}
        </div>
      )}
    </section>
  );
}

function RunLog({ rule }: { rule: Rule }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['rule-runs', rule.id],
    queryFn: () => api.get<{ runs: Run[] }>(`/rules/${rule.id}/runs`),
  });
  return (
    <section>
      <h2>{t('rules2.log')}</h2>
      <ErrorNote error={q.error} />
      {q.data?.runs.length === 0 && <p className="muted">{t('rules2.noRuns')}</p>}
      <ul className="plain-list">
        {q.data?.runs.map((r) => (
          <li key={r.id}>
            <RelativeTime value={r.created_at} />: {t(`rules2.run_${r.state}`)}
            {r.report && ` (${t(`rules2.report_${r.report}`, { defaultValue: r.report })})`}
            {r.page_title && `, ${r.page_title}`}
          </li>
        ))}
      </ul>
    </section>
  );
}

function HookAddress({ rule }: { rule: Rule }) {
  const { t } = useTranslation();
  const make = useMutation({ mutationFn: () => api.post<{ address: string }>(`/rules/${rule.id}/hook`) });
  return (
    <section>
      <h2>{t('rules2.hookTitle')}</h2>
      <p className="muted">{t('rules2.hookHelp')}</p>
      <ErrorNote error={make.error} />
      {make.data && (
        <p>
          <code>{make.data.address}</code>
        </p>
      )}
      <button type="button" onClick={() => make.mutate()}>
        {rule.has_hook ? t('rules2.hookRenew') : t('rules2.hookMake')}
      </button>
    </section>
  );
}

function RuleForm({ rule }: { rule: Rule }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState(rule.name);
  const [enabled, setEnabled] = useState(rule.enabled);
  const [definition, setDefinition] = useState(rule.definition);
  useEffect(() => {
    setName(rule.name);
    setEnabled(rule.enabled);
    setDefinition(rule.definition);
  }, [rule]);
  const save = useMutation({
    mutationFn: () => api.put(`/rules/${rule.id}`, { name, enabled, definition }),
    onSettled: () => client.invalidateQueries({ queryKey: ['rule', rule.id] }),
  });
  return (
    <>
      <h1>{rule.name}</h1>
      {rule.paused_at && <p className="note">{t('rules2.pausedHelp')}</p>}
      {rule.problems.length > 0 && (
        <div className="note note-error" role="alert">
          <p>{t('rules2.problems', { count: rule.problems.length })}</p>
          <ul>
            {rule.problems.map((p) => (
              <li key={`${p.step}-${p.message}`}>{p.message}</li>
            ))}
          </ul>
        </div>
      )}
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        <Field label={t('common.name')}>
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        {!rule.space_id && <ScopePart definition={definition} onChange={setDefinition} />}
        <TriggerPart definition={definition} problems={rule.problems} onChange={setDefinition} />
        <ConditionPart definition={definition} problems={rule.problems} onChange={setDefinition} />
        <ActionPart definition={definition} problems={rule.problems} onChange={setDefinition} />
        <label className="check">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          {t('rules2.enabled')}
        </label>
        <button type="submit" className="primary" disabled={save.isPending}>
          {t('common.save')}
        </button>
      </form>
      {rule.definition.trigger.kind === 'incoming' && <HookAddress rule={rule} />}
      <TestRun rule={rule} />
      <RunLog rule={rule} />
    </>
  );
}

function SpaceFrame({ spaceId, children }: { spaceId: string; children: React.ReactNode }) {
  const q = useQuery({
    queryKey: ['space-id', spaceId],
    queryFn: () => api.get<Space>(`/spaces/${spaceId}`),
  });
  if (!q.data) return <Loading />;
  return <Frame space={q.data}>{children}</Frame>;
}

/** One rule: When, If, Then, a test run and the log. */
export function RuleView({ ruleId }: { ruleId: string }) {
  const q = useQuery({ queryKey: ['rule', ruleId], queryFn: () => api.get<Rule>(`/rules/${ruleId}`) });
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!q.data) return <Loading />;
  if (q.data.space_id) {
    return (
      <SpaceFrame spaceId={q.data.space_id}>
        <RuleForm rule={q.data} />
      </SpaceFrame>
    );
  }
  return (
    <main id="main" className="plain reading">
      <RuleForm rule={q.data} />
    </main>
  );
}
