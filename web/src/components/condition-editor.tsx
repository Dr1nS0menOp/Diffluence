// SPDX-License-Identifier: Apache-2.0
// A condition, edited as a form of native fields (spec 074, TPL-1014): "All of" or "Any of",
// then rows of subject, operator and value, each a native select or input of the right kind,
// Add a test, and Remove on each row. A sentence above the rows says what the condition means,
// and the validator's words stand beside the test they concern before anything is saved.
import { useId, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import {
  conditionSentence,
  isDay,
  LIST_VALUE,
  MAX_TESTS,
  NO_VALUE,
  OPERATORS,
  validateCondition,
  type Condition,
  type ConditionField,
  type ConditionKind,
  type ConditionProblem,
  type Op,
  type Test,
} from '../template-conditions';
import { localToday } from '../template-fields';
import { readableDate } from './template-form';
import { SubjectPicker } from './picker';
import { Dialog, Field } from './ui';

/** A question a condition may test, by its name. */
export type NamedField = ConditionField & { name: string };

/** What a condition may test where it stands. */
export interface ConditionScope {
  /** The questions it may test: those asked before it, or every question. */
  fields: NamedField[];
  /** Questions it may not test, because they come at or after it (TPL-421). */
  later: string[];
  /** The template is one of all spaces, so the space can be tested. */
  space: boolean;
  /** Today's names of the groups and spaces tests name, by id. */
  names: Record<string, string>;
  /** The spaces a test of the space can choose. */
  spaces: { id: string; name: string }[];
}

/** The sentence words of a scope: names by id, days as the reader writes them. */
export function sentenceWords(scope: Pick<ConditionScope, 'names'>, extra: Record<string, string> = {}) {
  return {
    name: (id: string) => extra[id] ?? scope.names[id] ?? id,
    date: readableDate,
  };
}

/** Checks a condition where it stands, as the server will. */
export function conditionProblem(c: Condition, scope: ConditionScope): ConditionProblem | null {
  return validateCondition(c, {
    fields: Object.fromEntries(scope.fields.map((f) => [f.name, { kind: f.kind, options: f.options }])),
    later: new Set(scope.later),
    space: scope.space,
  });
}

const kindOf = (t: Test, scope: ConditionScope): ConditionKind | null => {
  if (t.subject === 'space') return 'space';
  if (t.subject === 'groups') return 'groups';
  return scope.fields.find((f) => f.name === t.field)?.kind ?? null;
};

/** A new test of a subject, with the first operator its kind has. */
function testOf(subject: string, scope: ConditionScope): Test {
  if (subject === 'space') return { subject: 'space', op: 'one_of', values: [] };
  if (subject === 'groups') return { subject: 'groups', op: 'member', value: '' };
  const field = scope.fields.find((f) => f.name === subject);
  const op = OPERATORS[field?.kind ?? 'line'][0] ?? 'empty';
  return { subject: 'field', field: subject, op };
}

/** A test keeps what still fits after its operator changes, and drops the rest. */
function withOp(t: Test, op: Op): Test {
  const next: Test = { subject: t.subject, field: t.field, op };
  if (LIST_VALUE.includes(op)) next.values = t.values ?? [];
  else if (!NO_VALUE.includes(op)) {
    if (t.today) next.today = true;
    else next.value = t.value ?? '';
  }
  return next;
}

function TestValue({
  test,
  at,
  scope,
  names,
  onChange,
  onName,
  describedBy,
}: {
  test: Test;
  at: number;
  scope: ConditionScope;
  names: Record<string, string>;
  onChange: (next: Test) => void;
  onName: (id: string, name: string) => void;
  describedBy?: string;
}) {
  const { t } = useTranslation();
  const kind = kindOf(test, scope);
  const options = scope.fields.find((f) => f.name === test.field)?.options ?? [];
  if (NO_VALUE.includes(test.op) || kind === null) return null;
  const label = t('templates.condition.value', { at: at + 1 });
  if (kind === 'groups') {
    const chosen = test.value ? (names[test.value] ?? scope.names[test.value] ?? test.value) : '';
    return (
      <div className="field">
        {chosen && <p>{chosen}</p>}
        <SubjectPicker
          groupsOnly
          label={chosen ? t('templates.condition.chooseGroup') : label}
          onPick={(entry) => {
            onName(entry.id, entry.name);
            onChange({ ...test, value: entry.id });
          }}
        />
        <p className="muted">{t('templates.condition.groupsNote')}</p>
      </div>
    );
  }
  if (LIST_VALUE.includes(test.op)) {
    const list =
      kind === 'space'
        ? scope.spaces.map((s) => ({ id: s.id, name: s.name }))
        : options.map((o) => ({ id: o, name: o }));
    const values = test.values ?? [];
    if (kind === 'label') {
      return (
        <Field
          label={t('templates.condition.values', { at: at + 1 })}
          help={t('templates.condition.valuesLinesHelp')}
        >
          <textarea
            rows={3}
            value={values.join('\n')}
            aria-describedby={describedBy}
            onChange={(e) => onChange({ ...test, values: e.target.value.split('\n') })}
          />
        </Field>
      );
    }
    return (
      <fieldset className="choices" aria-describedby={describedBy}>
        <legend>{t('templates.condition.values', { at: at + 1 })}</legend>
        {list.map((item) => (
          <label key={item.id} className="check">
            <input
              type="checkbox"
              checked={values.includes(item.id)}
              onChange={(e) =>
                onChange({
                  ...test,
                  values: e.target.checked ? [...values, item.id] : values.filter((v) => v !== item.id),
                })
              }
            />
            {item.name}
          </label>
        ))}
      </fieldset>
    );
  }
  if (kind === 'choice' || kind === 'choices') {
    return (
      <Field label={label}>
        <select
          value={test.value ?? ''}
          aria-describedby={describedBy}
          onChange={(e) => onChange({ ...test, value: e.target.value })}
        >
          <option value="">{t('organise.chooseOne')}</option>
          {options.map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </select>
      </Field>
    );
  }
  if (kind === 'date') {
    return (
      <div className="row">
        <Field label={t('templates.condition.dayHow', { at: at + 1 })}>
          <select
            value={test.today ? 'today' : 'fixed'}
            onChange={(e) =>
              onChange(
                e.target.value === 'today'
                  ? { subject: test.subject, field: test.field, op: test.op, today: true }
                  : { subject: test.subject, field: test.field, op: test.op, value: localToday() },
              )
            }
          >
            <option value="today">{t('templates.condition.dayToday')}</option>
            <option value="fixed">{t('templates.condition.dayFixed')}</option>
          </select>
        </Field>
        {!test.today && (
          <Field label={label}>
            <input
              type="date"
              value={isDay(test.value ?? '') ? test.value : ''}
              aria-describedby={describedBy}
              onChange={(e) => onChange({ ...test, value: e.target.value })}
            />
          </Field>
        )}
      </div>
    );
  }
  return (
    <Field label={label}>
      <input
        value={test.value ?? ''}
        inputMode={kind === 'number' ? 'decimal' : undefined}
        aria-describedby={describedBy}
        onChange={(e) => onChange({ ...test, value: e.target.value })}
      />
    </Field>
  );
}

/** The rows of one condition, with its sentence and its problem. */
export function ConditionEditor({
  value,
  scope,
  onChange,
  showProblem,
}: {
  value: Condition;
  scope: ConditionScope;
  onChange: (next: Condition) => void;
  /** Show the validator's words; before the first save they wait for the author. */
  showProblem: boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [names, setNames] = useState<Record<string, string>>({});
  const problem = conditionProblem(value, scope);
  const problemId = `${id}-problem`;
  const setTest = (i: number, next: Test) =>
    onChange({ ...value, tests: value.tests.map((test, j) => (j === i ? next : test)) });
  const subjects = [
    ...scope.fields.map((f) => ({ id: f.name, label: f.name })),
    ...(scope.space ? [{ id: 'space', label: t('templates.condition.subjectSpace') }] : []),
    { id: 'groups', label: t('templates.condition.subjectGroups') },
  ];
  const problemWords = problem ? t(`templates.conditionProblem.${problem.problem}`) : '';
  return (
    <div className="condition-editor">
      {value.tests.length > 0 && (
        <p className="condition-sentence">{conditionSentence(value, sentenceWords(scope, names))}</p>
      )}
      <Field label={t('templates.condition.match')}>
        <select
          value={value.match}
          onChange={(e) => onChange({ ...value, match: e.target.value === 'any' ? 'any' : 'all' })}
        >
          <option value="all">{t('templates.condition.matchAll')}</option>
          <option value="any">{t('templates.condition.matchAny')}</option>
        </select>
      </Field>
      {scope.fields.length === 0 && <p className="muted">{t('templates.condition.noSubject')}</p>}
      <ol className="condition-tests">
        {value.tests.map((test, i) => {
          const kind = kindOf(test, scope);
          const subject = test.subject === 'field' ? (test.field ?? '') : test.subject;
          const mine = showProblem && problem?.test === i;
          return (
            <li key={i} className="rule-step">
              <div className="row">
                <Field label={t('templates.condition.subject', { at: i + 1 })}>
                  <select
                    value={subject}
                    aria-describedby={mine ? problemId : undefined}
                    onChange={(e) => setTest(i, testOf(e.target.value, scope))}
                  >
                    {!subjects.some((s) => s.id === subject) && <option value={subject}>{subject}</option>}
                    {subjects.map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label={t('templates.condition.op', { at: i + 1 })}>
                  <select
                    value={test.op}
                    aria-describedby={mine ? problemId : undefined}
                    onChange={(e) => {
                      const op = (kind ? OPERATORS[kind] : []).find((o) => o === e.target.value);
                      if (op) setTest(i, withOp(test, op));
                    }}
                  >
                    {(kind ? OPERATORS[kind] : [test.op]).map((op) => (
                      <option key={op} value={op}>
                        {t(`templates.condition.opName_${op}`)}
                      </option>
                    ))}
                  </select>
                </Field>
              </div>
              <TestValue
                test={test}
                at={i}
                scope={scope}
                names={names}
                describedBy={mine ? problemId : undefined}
                onChange={(next) => setTest(i, next)}
                onName={(groupId, name) => setNames({ ...names, [groupId]: name })}
              />
              {mine && (
                <p className="field-error" id={problemId}>
                  {problemWords}
                </p>
              )}
              <button
                type="button"
                className="ghost"
                onClick={() => onChange({ ...value, tests: value.tests.filter((_, j) => j !== i) })}
              >
                {t('templates.condition.removeTest', { at: i + 1 })}
              </button>
            </li>
          );
        })}
      </ol>
      {showProblem && problem && problem.test < 0 && (
        <p className="field-error" id={problemId}>
          {problemWords}
        </p>
      )}
      <p className="row">
        <button
          type="button"
          disabled={value.tests.length >= MAX_TESTS || subjects.length === 0}
          onClick={() =>
            onChange({ ...value, tests: [...value.tests, testOf(subjects[0]?.id ?? '', scope)] })
          }
        >
          {t('templates.condition.addTest')}
        </button>
        <span className="muted">{t('templates.condition.testCount', { count: value.tests.length })}</span>
      </p>
    </div>
  );
}

/**
 * A condition in a Dialog, for whatever it switches: an entry, a question, a step, a rule or
 * a section of text. Remove takes the condition away; Keep refuses while it has a problem.
 */
export function ConditionDialog({
  title,
  start,
  scope,
  onSave,
  onClose,
}: {
  title: string;
  start: Condition | undefined;
  scope: ConditionScope;
  onSave: (next: Condition | undefined) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const first = scope.fields[0];
  const [value, setValue] = useState<Condition>(
    start ?? {
      match: 'all',
      tests: [first ? testOf(first.name, scope) : { subject: 'groups', op: 'member', value: '' }],
    },
  );
  const [tried, setTried] = useState(false);
  const problem = conditionProblem(value, scope);
  // A condition is often set from a row inside another form, such as the Form section, so the
  // dialog stands outside it: its own form must never submit that one.
  return createPortal(
    <Dialog title={title} onClose={onClose}>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          e.stopPropagation();
          setTried(true);
          if (!problem) onSave(value);
        }}
      >
        <ConditionEditor value={value} scope={scope} onChange={setValue} showProblem={tried} />
        <div className="dialog-actions">
          {start && (
            <button type="button" className="quiet" onClick={() => onSave(undefined)}>
              {t('templates.condition.remove')}
            </button>
          )}
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="primary">
            {t('templates.condition.keep')}
          </button>
        </div>
      </form>
    </Dialog>,
    document.body,
  );
}
