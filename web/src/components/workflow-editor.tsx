// SPDX-License-Identifier: Apache-2.0
// The workflow editor (spec 064, DOC-910 to DOC-913): a form, because a form works by keyboard
// and screen reader from the first day. States first, then transitions, each with Move up and
// Move down instead of dragging. The validator runs while the person types and puts its
// message beside the state or transition it is about. A diagram is drawn from the form, with
// the same transitions in words beside it.
import { useDeferredValue, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  api,
  type DirectoryEntry,
  type WorkflowChange,
  type WorkflowDefinition,
  type WorkflowOutcome,
  type WorkflowProblem,
  type WorkflowReview,
  type WorkflowReviewer,
  type WorkflowState,
  type WorkflowStateKind,
  type WorkflowTransition,
} from '../api';
import { useRoles } from '../views/roles';
import { WorkflowDiagram } from './drawing';
import { SubjectPicker } from './picker';
import { Dialog, ErrorNote, Field } from './ui';

const KINDS: WorkflowStateKind[] = ['draft', 'in_review', 'approved', 'rejected', 'obsolete'];
const RULES = ['all', 'quorum', 'any'] as const;
const OUTCOMES: ('' | WorkflowOutcome)[] = ['', 'approved', 'changes'];
const MAX_ID = 40;
const NO_ROLE = '';

/** What the editor hands back to save. */
export interface WorkflowDraft {
  name: string;
  description: string;
  definition: WorkflowDefinition;
}

/** Names of reviewers the editor knows, by id: picked here, or sent along by the server. */
type Names = Record<string, string>;

/** An identifier from a name: lower-case letters, digits and dashes, as the validator asks. */
export function idFromName(name: string, taken: string[], fallback: string): string {
  const base =
    name
      .toLowerCase()
      .normalize('NFKD')
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, MAX_ID - 4) || fallback;
  let id = base;
  for (let n = 2; taken.includes(id); n++) id = `${base}-${n}`;
  return id;
}

function move<T>(list: T[], index: number, step: number): T[] {
  const to = index + step;
  if (to < 0 || to >= list.length) return list;
  const next = [...list];
  const [item] = next.splice(index, 1);
  if (item !== undefined) next.splice(to, 0, item);
  return next;
}

/** The changes a save makes, in the words of the workflow's history (DOC-112, DOC-913). */
export function changesBetween(before: WorkflowDefinition, after: WorkflowDefinition): WorkflowChange[] {
  const out: WorkflowChange[] = [];
  for (const s of before.states) {
    const n = after.states.find((x) => x.id === s.id);
    if (!n) out.push({ kind: 'state_removed', id: s.id, from: s.name });
    else if (n.name !== s.name) out.push({ kind: 'state_renamed', id: s.id, from: s.name, to: n.name });
    else if (JSON.stringify(n) !== JSON.stringify(s))
      out.push({ kind: 'state_changed', id: s.id, to: n.name });
  }
  for (const s of after.states) {
    if (!before.states.some((x) => x.id === s.id)) out.push({ kind: 'state_added', id: s.id, to: s.name });
  }
  for (const tr of before.transitions) {
    const n = after.transitions.find((x) => x.id === tr.id);
    if (!n) out.push({ kind: 'transition_removed', id: tr.id, from: tr.name });
    else if (JSON.stringify(n) !== JSON.stringify(tr))
      out.push({ kind: 'transition_changed', id: tr.id, to: n.name });
  }
  for (const tr of after.transitions) {
    if (!before.transitions.some((x) => x.id === tr.id))
      out.push({ kind: 'transition_added', id: tr.id, to: tr.name });
  }
  return out;
}

export function WorkflowEditor({
  initial,
  names: knownNames = {},
  existing,
  busy,
  error,
  onSave,
  onCancel,
}: {
  initial: WorkflowDraft;
  names?: Names;
  /** The saved definition, when this changes a workflow that exists; saving then shows what changes. */
  existing?: WorkflowDefinition;
  busy: boolean;
  error: unknown;
  onSave: (draft: WorkflowDraft) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const [def, setDef] = useState<WorkflowDefinition>(initial.definition);
  const [fresh, setFresh] = useState<string[]>([]);
  // A new item takes its id from its name as it is typed; its row keeps the key it began with,
  // so the field being typed in is not rebuilt and keeps the focus. A new item gets a key with
  // a colon, which no id may hold, so it never meets the id of another row.
  const [keys, setKeys] = useState<Record<string, string>>({});
  const keyOf = (id: string) => keys[id] ?? id;
  const rekey = (from: string, to: string) => setKeys((k) => ({ ...k, [to]: k[from] ?? from }));
  const keyNew = (id: string) => setKeys((k) => ({ ...k, [id]: `new:${Object.keys(k).length}` }));
  const [names, setNames] = useState<Names>(knownNames);
  const [confirming, setConfirming] = useState(false);
  const checked = useDeferredValue(def);
  const check = useQuery({
    queryKey: ['workflow-check', JSON.stringify(checked)],
    queryFn: () => api.post<{ problem: WorkflowProblem | null }>('/workflows/check', { definition: checked }),
    placeholderData: (previous) => previous,
  });
  const problem = check.data?.problem ?? null;
  const editing: Editing = { def, setDef, fresh, setFresh, names, setNames, problem, keyOf, rekey, keyNew };
  const save = () => (existing ? setConfirming(true) : onSave({ name, description, definition: def }));
  return (
    <form
      className="workflow-editor"
      onSubmit={(e) => {
        e.preventDefault();
        save();
      }}
    >
      <p className="note workflow-small-note">{t('workflowEditor.smallScreen')}</p>
      <Field label={t('workflowEditor.name')}>
        <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={80} />
      </Field>
      <Field label={t('workflowEditor.description')}>
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} maxLength={500} />
      </Field>
      {problem && !problem.subject && <ProblemNote problem={problem} />}
      <StatesSection editing={editing} />
      <TransitionsSection editing={editing} />
      <DiagramSection definition={def} />
      {problem && <p className="note note-error">{t('workflowEditor.fixFirst')}</p>}
      <ErrorNote error={error} />
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onCancel}>
          {t('common.cancel')}
        </button>
        <button type="submit" className="primary" disabled={busy || !!problem || name.trim() === ''}>
          {t('workflowEditor.save')}
        </button>
      </div>
      {confirming && existing && (
        <SaveDialog
          before={existing}
          after={def}
          busy={busy}
          onClose={() => setConfirming(false)}
          onSave={() => {
            setConfirming(false);
            onSave({ name, description, definition: def });
          }}
        />
      )}
    </form>
  );
}

interface Editing {
  def: WorkflowDefinition;
  setDef: (d: WorkflowDefinition) => void;
  fresh: string[];
  setFresh: (ids: string[]) => void;
  names: Names;
  setNames: (n: Names) => void;
  problem: WorkflowProblem | null;
  /** The row key of a state or transition, which survives a change of its id. */
  keyOf: (id: string) => string;
  rekey: (from: string, to: string) => void;
  /** Gives a new state or transition a key of its own, before its id starts to follow its name. */
  keyNew: (id: string) => void;
}

/** The validator's message, beside what it is about (DOC-912, WCAG 3.3.1). */
function ProblemNote({ problem }: { problem: WorkflowProblem }) {
  return (
    <p className="note note-error" role="alert">
      {problem.message}
    </p>
  );
}

function Row({
  index,
  count,
  onMove,
  onRemove,
  removeLabel,
}: {
  index: number;
  count: number;
  onMove: (step: number) => void;
  onRemove: () => void;
  removeLabel: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="row workflow-row-actions">
      <button type="button" className="ghost" disabled={index === 0} onClick={() => onMove(-1)}>
        {t('workflowEditor.moveUp')}
      </button>
      <button type="button" className="ghost" disabled={index === count - 1} onClick={() => onMove(1)}>
        {t('workflowEditor.moveDown')}
      </button>
      <button type="button" className="ghost" onClick={onRemove}>
        {removeLabel}
      </button>
    </div>
  );
}

function StatesSection({ editing }: { editing: Editing }) {
  const { t } = useTranslation();
  const { def, setDef, fresh, setFresh } = editing;
  const add = () => {
    const id = idFromName(
      '',
      def.states.map((s) => s.id),
      `state-${def.states.length + 1}`,
    );
    const state: WorkflowState = {
      id,
      name: '',
      description: '',
      kind: 'draft',
      start: def.states.length === 0,
      timed: false,
    };
    setFresh([...fresh, id]);
    editing.keyNew(id);
    setDef({ ...def, states: [...def.states, state] });
  };
  return (
    <section aria-labelledby="workflow-states">
      <h2 id="workflow-states">{t('workflowEditor.states')}</h2>
      {def.states.length === 0 && <p className="muted">{t('workflowEditor.noStates')}</p>}
      {def.states.map((s, i) => (
        <StateFields key={editing.keyOf(s.id)} editing={editing} state={s} index={i} />
      ))}
      <button type="button" onClick={add}>
        {t('workflowEditor.addState')}
      </button>
    </section>
  );
}

function StateFields({ editing, state, index }: { editing: Editing; state: WorkflowState; index: number }) {
  const { t } = useTranslation();
  const { def, setDef, fresh, problem } = editing;
  const update = (next: WorkflowState) => {
    // A new state takes its identifier from its name; a saved one keeps it, so history follows.
    const renamed = fresh.includes(state.id) && next.name !== state.name;
    const id = renamed
      ? idFromName(
          next.name,
          def.states.filter((s) => s.id !== state.id).map((s) => s.id),
          state.id,
        )
      : state.id;
    if (id !== state.id) {
      editing.setFresh(fresh.map((f) => (f === state.id ? id : f)));
      editing.rekey(state.id, id);
    }
    const states = def.states.map((s) =>
      s.id === state.id ? { ...next, id } : next.start && s.id !== state.id ? { ...s, start: false } : s,
    );
    const transitions = def.transitions.map((tr) => ({
      ...tr,
      from: tr.from === state.id ? id : tr.from,
      to: tr.to === state.id ? id : tr.to,
    }));
    setDef({ states, transitions });
  };
  const label = state.name || t('workflowEditor.stateNumber', { number: index + 1 });
  return (
    <fieldset className="workflow-item">
      <legend>{label}</legend>
      {problem?.subject === state.id && <ProblemNote problem={problem} />}
      <Field label={t('workflowEditor.stateName')}>
        <input
          value={state.name}
          onChange={(e) => update({ ...state, name: e.target.value })}
          maxLength={80}
        />
      </Field>
      <Field label={t('workflowEditor.kind')} help={t(`workflowEditor.kindHelp_${state.kind}`)}>
        <select
          value={state.kind}
          onChange={(e) => {
            const kind = e.target.value as WorkflowStateKind;
            update({ ...state, kind, review: kind === 'in_review' ? state.review : undefined, timed: false });
          }}
        >
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {t(`workflowEditor.kind_${k}`)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('workflowEditor.stateDescription')}>
        <textarea
          value={state.description}
          onChange={(e) => update({ ...state, description: e.target.value })}
          maxLength={500}
        />
      </Field>
      <label className="check">
        <input
          type="checkbox"
          checked={state.start}
          onChange={(e) => update({ ...state, start: e.target.checked })}
        />
        {t('workflowEditor.start')}
      </label>
      {state.kind === 'in_review' && (
        <ReviewFields editing={editing} state={state} onChange={(review) => update({ ...state, review })} />
      )}
      <Row
        index={index}
        count={def.states.length}
        onMove={(step) => setDef({ ...def, states: move(def.states, index, step) })}
        onRemove={() =>
          setDef({
            states: def.states.filter((s) => s.id !== state.id),
            transitions: def.transitions.filter((tr) => tr.from !== state.id && tr.to !== state.id),
          })
        }
        removeLabel={t('workflowEditor.removeState', { name: label })}
      />
    </fieldset>
  );
}

const NEW_REVIEW: WorkflowReview = {
  reviewers: [],
  rule: 'all',
  authors_may_review: false,
  authors_may_add: false,
};

function ReviewFields({
  editing,
  state,
  onChange,
}: {
  editing: Editing;
  state: WorkflowState;
  onChange: (review: WorkflowReview | undefined) => void;
}) {
  const { t } = useTranslation();
  const review = state.review;
  return (
    <div className="workflow-review">
      <label className="check">
        <input
          type="checkbox"
          checked={!!review}
          onChange={(e) => onChange(e.target.checked ? NEW_REVIEW : undefined)}
        />
        {t('workflowEditor.reviewersDecide')}
      </label>
      {review && (
        <>
          <ReviewerList editing={editing} review={review} onChange={onChange} />
          <Field label={t('workflowEditor.rule')}>
            <select
              value={review.rule}
              onChange={(e) => {
                const rule = e.target.value as WorkflowReview['rule'];
                onChange({ ...review, rule, quorum: rule === 'quorum' ? (review.quorum ?? 2) : undefined });
              }}
            >
              {RULES.map((r) => (
                <option key={r} value={r}>
                  {t(`workflowEditor.rule_${r}`)}
                </option>
              ))}
            </select>
          </Field>
          {review.rule === 'quorum' && (
            <Field label={t('workflowEditor.quorum')}>
              <input
                type="number"
                min={1}
                max={50}
                value={review.quorum ?? 1}
                onChange={(e) => onChange({ ...review, quorum: Number(e.target.value) })}
              />
            </Field>
          )}
          <label className="check">
            <input
              type="checkbox"
              checked={!review.authors_may_review}
              onChange={(e) => onChange({ ...review, authors_may_review: !e.target.checked })}
            />
            {t('workflowEditor.independent')}
          </label>
          <label className="check">
            <input
              type="checkbox"
              checked={review.authors_may_add}
              onChange={(e) => onChange({ ...review, authors_may_add: e.target.checked })}
            />
            {t('workflowEditor.authorsMayAdd')}
          </label>
        </>
      )}
    </div>
  );
}

function ReviewerList({
  editing,
  review,
  onChange,
}: {
  editing: Editing;
  review: WorkflowReview;
  onChange: (review: WorkflowReview) => void;
}) {
  const { t } = useTranslation();
  const roles = useRoles('space');
  const [role, setRole] = useState(NO_ROLE);
  const add = (entry: WorkflowReviewer, label: string) => {
    if (review.reviewers.some((r) => r.type === entry.type && r.id === entry.id)) return;
    editing.setNames({ ...editing.names, [entry.id]: label });
    onChange({ ...review, reviewers: [...review.reviewers, entry] });
  };
  const nameOf = (r: WorkflowReviewer) =>
    editing.names[r.id] ??
    roles.data?.roles.find((x) => x.id === r.id)?.name ??
    t(`workflowEditor.unknown_${r.type}`);
  return (
    <>
      <p className="muted">{t('workflowEditor.reviewersHelp')}</p>
      {review.reviewers.length === 0 ? (
        <p className="muted">{t('workflowEditor.noReviewers')}</p>
      ) : (
        <ul className="plain-list">
          {review.reviewers.map((r) => (
            <li key={r.type + r.id} className="row">
              <span>{t(`workflowEditor.reviewer_${r.type}`, { name: nameOf(r) })}</span>
              <button
                type="button"
                className="ghost"
                onClick={() => onChange({ ...review, reviewers: review.reviewers.filter((x) => x !== r) })}
              >
                {t('workflowEditor.removeReviewer', { name: nameOf(r) })}
              </button>
            </li>
          ))}
        </ul>
      )}
      <SubjectPicker
        label={t('workflowEditor.addReviewer')}
        onPick={(e: DirectoryEntry) => add({ type: e.type === 'group' ? 'group' : 'user', id: e.id }, e.name)}
      />
      <div className="row">
        <Field label={t('workflowEditor.addRole')}>
          <select value={role} onChange={(e) => setRole(e.target.value)}>
            <option value={NO_ROLE}>{t('workflowEditor.chooseRole')}</option>
            {roles.data?.roles.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </Field>
        <button
          type="button"
          disabled={role === NO_ROLE}
          onClick={() => {
            add({ type: 'role', id: role }, roles.data?.roles.find((x) => x.id === role)?.name ?? '');
            setRole(NO_ROLE);
          }}
        >
          {t('workflowEditor.addRoleButton')}
        </button>
      </div>
    </>
  );
}

function TransitionsSection({ editing }: { editing: Editing }) {
  const { t } = useTranslation();
  const { def, setDef, fresh, setFresh } = editing;
  const add = () => {
    const id = idFromName(
      '',
      def.transitions.map((x) => x.id),
      `step-${def.transitions.length + 1}`,
    );
    const first = def.states[0]?.id ?? '';
    const tr: WorkflowTransition = {
      id,
      name: '',
      description: '',
      from: first,
      to: def.states[1]?.id ?? first,
      requires_comment: false,
    };
    setFresh([...fresh, id]);
    editing.keyNew(id);
    setDef({ ...def, transitions: [...def.transitions, tr] });
  };
  return (
    <section aria-labelledby="workflow-transitions">
      <h2 id="workflow-transitions">{t('workflowEditor.transitions')}</h2>
      {def.transitions.length === 0 && <p className="muted">{t('workflowEditor.noTransitions')}</p>}
      {def.transitions.map((tr, i) => (
        <TransitionFields key={editing.keyOf(tr.id)} editing={editing} transition={tr} index={i} />
      ))}
      <button type="button" onClick={add} disabled={def.states.length < 2}>
        {t('workflowEditor.addTransition')}
      </button>
    </section>
  );
}

function StateSelect({
  label,
  value,
  states,
  onChange,
}: {
  label: string;
  value: string;
  states: WorkflowState[];
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Field label={label}>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {states.map((s, i) => (
          <option key={s.id} value={s.id}>
            {s.name || t('workflowEditor.stateNumber', { number: i + 1 })}
          </option>
        ))}
      </select>
    </Field>
  );
}

function TransitionFields({
  editing,
  transition: tr,
  index,
}: {
  editing: Editing;
  transition: WorkflowTransition;
  index: number;
}) {
  const { t } = useTranslation();
  const { def, setDef, fresh, problem } = editing;
  const update = (next: WorkflowTransition) => {
    const renamed = fresh.includes(tr.id) && next.name !== tr.name;
    const id = renamed
      ? idFromName(
          next.name,
          def.transitions.filter((x) => x.id !== tr.id).map((x) => x.id),
          tr.id,
        )
      : tr.id;
    if (id !== tr.id) {
      editing.setFresh(fresh.map((f) => (f === tr.id ? id : f)));
      editing.rekey(tr.id, id);
    }
    setDef({ ...def, transitions: def.transitions.map((x) => (x.id === tr.id ? { ...next, id } : x)) });
  };
  const fromReview = !!def.states.find((s) => s.id === tr.from)?.review;
  const label = tr.name || t('workflowEditor.transitionNumber', { number: index + 1 });
  return (
    <fieldset className="workflow-item">
      <legend>{label}</legend>
      {problem?.subject === tr.id && <ProblemNote problem={problem} />}
      <Field label={t('workflowEditor.buttonName')}>
        <input value={tr.name} onChange={(e) => update({ ...tr, name: e.target.value })} maxLength={80} />
      </Field>
      <div className="row workflow-pair">
        <StateSelect
          label={t('workflowEditor.from')}
          value={tr.from}
          states={def.states}
          onChange={(from) => update({ ...tr, from })}
        />
        <StateSelect
          label={t('workflowEditor.to')}
          value={tr.to}
          states={def.states}
          onChange={(to) => update({ ...tr, to })}
        />
      </div>
      {fromReview && (
        <Field label={t('workflowEditor.outcome')} help={t('workflowEditor.outcomeHelp')}>
          <select
            value={tr.outcome ?? ''}
            onChange={(e) =>
              update({ ...tr, outcome: (e.target.value || undefined) as WorkflowOutcome | undefined })
            }
          >
            {OUTCOMES.map((o) => (
              <option key={o} value={o}>
                {t(`workflowEditor.outcome_${o || 'none'}`)}
              </option>
            ))}
          </select>
        </Field>
      )}
      <label className="check">
        <input
          type="checkbox"
          checked={tr.requires_comment}
          onChange={(e) => update({ ...tr, requires_comment: e.target.checked })}
        />
        {t('workflowEditor.requiresComment')}
      </label>
      <Row
        index={index}
        count={def.transitions.length}
        onMove={(step) => setDef({ ...def, transitions: move(def.transitions, index, step) })}
        onRemove={() => setDef({ ...def, transitions: def.transitions.filter((x) => x.id !== tr.id) })}
        removeLabel={t('workflowEditor.removeTransition', { name: label })}
      />
    </fieldset>
  );
}

/** The diagram, and the same in words (DOC-911, WCAG 1.1.1). */
export function DiagramSection({ definition, title }: { definition: WorkflowDefinition; title?: ReactNode }) {
  const { t } = useTranslation();
  const nameOf = (id: string) => definition.states.find((s) => s.id === id)?.name || id;
  return (
    <section aria-labelledby="workflow-diagram" className="workflow-diagram-section">
      <h2 id="workflow-diagram">{title ?? t('workflowEditor.diagram')}</h2>
      {definition.states.length > 0 && <WorkflowDiagram definition={definition} />}
      <ul className="workflow-sentences" aria-label={t('workflowEditor.inWords')}>
        {definition.transitions.map((tr) => (
          <li key={tr.id}>
            {t(tr.outcome ? `workflowEditor.sentence_${tr.outcome}` : 'workflowEditor.sentence', {
              from: nameOf(tr.from),
              step: tr.name,
              to: nameOf(tr.to),
            })}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** What a save changes, for running rounds and renamed states, before it happens (DOC-913). */
function SaveDialog({
  before,
  after,
  busy,
  onSave,
  onClose,
}: {
  before: WorkflowDefinition;
  after: WorkflowDefinition;
  busy: boolean;
  onSave: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const changes = changesBetween(before, after);
  const renamed = changes.some((c) => c.kind === 'state_renamed');
  return (
    <Dialog title={t('workflowEditor.saveTitle')} onClose={onClose}>
      {changes.length === 0 ? (
        <p>{t('workflowEditor.noChanges')}</p>
      ) : (
        <ul>
          {changes.map((c) => (
            <li key={c.kind + c.id}>{t(`workflowEditor.change_${c.kind}`, { from: c.from, to: c.to })}</li>
          ))}
        </ul>
      )}
      <p>{t('workflowEditor.runningRounds')}</p>
      {renamed && <p>{t('workflowEditor.renamedStates')}</p>}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" disabled={busy} onClick={onSave}>
          {t('workflowEditor.saveVersion')}
        </button>
      </div>
    </Dialog>
  );
}
