// SPDX-License-Identifier: Apache-2.0
// The form a person fills in to start pages from a template (spec 074, wave GT2): one step
// at a time, each question in a Field, errors beside their field and in a summary, the
// place of the pages last, and a check step that shows what will be made before anything is.
import { useEffect, useId, useMemo, useRef, useState, type FormEvent, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import i18n from '../i18n';
import {
  api,
  ApiError,
  type FieldProblem,
  type Me,
  type Page,
  type PlannedPage,
  type SetCheck,
  type SetOutcome,
  type Space,
  type TemplateField,
  type TemplateSummary,
} from '../api';
import { navigate } from '../router';
import {
  checkAnswer,
  defaultValue,
  fieldSpecOf,
  localToday,
  MAX_PEOPLE,
  type FieldSpec,
  type Problem,
} from '../template-fields';
import { holds, type Condition, type Facts } from '../template-conditions';
import { IconLock } from './icons';
import { PagePicker, SubjectPicker } from './picker';
import { ConfirmDialog, ErrorNote, Field, fieldErrorId, fieldHelpId, Loading } from './ui';
import { pageTitle } from '../page-name';

/** Up to this many options show as radio buttons; more become a list (TPL-302). */
const RADIO_LIMIT = 6;

/** Groups fields in the steps of a template. Fields outside any step go last. */
export function stepsOf(
  template: TemplateSummary,
): { title: string; fields: TemplateField[]; condition?: Condition }[] {
  const fields = template.fields ?? [];
  const placed = new Set(template.steps.flatMap((s) => s.fields));
  const steps = template.steps.map((s) => ({
    title: s.title,
    condition: s.condition,
    fields: fields.filter((f) => s.fields.includes(f.name)),
  }));
  const rest = fields.filter((f) => !placed.has(f.name));
  return rest.length > 0 || steps.length === 0 ? [...steps, { title: '', fields: rest }] : steps;
}

/**
 * The questions the answers so far show (spec 074, TPL-421): a step's condition and a
 * question's own, each testing only questions asked before, decided in the order the form
 * asks them, as the server does. A hidden question counts as unanswered. The server decides
 * again on submit; this only previews (TPL-1406).
 */
export function shownQuestions(
  template: TemplateSummary,
  answers: Record<string, string>,
  base: Omit<Facts, 'answers'>,
): Set<string> {
  const facts: Facts = { ...base, answers: {} };
  const shown = new Set<string>();
  for (const step of stepsOf(template)) {
    const stepShown = holds(step.condition, facts);
    for (const field of step.fields.map(fieldSpecOf)) {
      const visible = stepShown && holds(field.condition, facts);
      if (visible) shown.add(field.name);
      const [value, problem] = visible
        ? checkAnswer(field, answers[field.name] ?? '', base.today)
        : ['', null];
      facts.answers[field.name] = { kind: field.kind, value: problem ? '' : value };
    }
  }
  return shown;
}

/**
 * A step of the form: questions, of which the last step also holds the place of the pages
 * and their labels (TPL-333), or the check of what will be made (TPL-340).
 */
export type FormStep =
  { kind: 'fields'; title: string; fields: FieldSpec[]; place: boolean } | { kind: 'check' };

/**
 * The steps a person goes through. A step without a question is skipped (TPL-330); the last
 * one sets where the pages go; the check step follows for a guided template, whose dry run
 * the server works out. A plain template keeps the form it always had (TPL-1104).
 */
export function formSteps(template: TemplateSummary, withCheck: boolean, shown?: Set<string>): FormStep[] {
  const steps: FormStep[] = stepsOf(template)
    .map((s) => ({
      kind: 'fields' as const,
      title: s.title,
      fields: s.fields.map(fieldSpecOf).filter((f) => !shown || shown.has(f.name)),
      place: false,
    }))
    .filter((s) => s.fields.length > 0);
  const last = steps[steps.length - 1];
  if (last?.kind === 'fields') last.place = true;
  else steps.push({ kind: 'fields', title: '', fields: [], place: true });
  if (withCheck) steps.push({ kind: 'check' });
  return steps;
}

/** The answers a form starts with: every question, filled with its default where it has one. */
export function startingAnswers(
  fields: FieldSpec[],
  today: string,
  creatorId: string,
): Record<string, string> {
  return Object.fromEntries(
    fields.map((f) => [f.name, f.default_gone ? '' : defaultValue(f, today, creatorId)]),
  );
}

/** A day written as the reader's language writes it (TPL-1043). */
export function readableDate(value: string): string {
  const day = /^\d{4}-\d{2}-\d{2}$/.test(value) ? new Date(`${value}T00:00:00Z`) : null;
  if (!day || Number.isNaN(day.getTime())) return value;
  return new Intl.DateTimeFormat(i18n.language, { dateStyle: 'long', timeZone: 'UTC' }).format(day);
}

/** The words of a problem, from the translation files, with dates written out. */
export function problemWords(problem: Problem): string {
  const params = { ...problem.params };
  if (params.date) params.date = readableDate(params.date);
  return i18n.t(`templates.problem.${problem.problem}`, params);
}

/** Makes the page, or the set of pages, from the answers. */
export async function startPage(
  template: TemplateSummary,
  space: Space,
  body: { parent_id: string | null; answers: Record<string, string>; replaces: string | null },
  requestKey: string,
): Promise<SetOutcome> {
  const common = { space_id: space.id, ...body, version: template.template_version };
  // A guided template is bound to its version and to a key made when the form opened, so a
  // second press makes no second set (TPL-123, TPL-603).
  const made = template.guided
    ? await api.post<{ page?: Page; set_id?: string; job_id?: string }>(`/templates/${template.id}/sets`, {
        ...common,
        request_key: requestKey,
      })
    : await api.post<{ page?: Page; set_id?: string; job_id?: string }>(
        `/templates/${template.id}/use`,
        common,
      );
  if (made.job_id) return { kind: 'queued', job_id: made.job_id };
  if (!made.page) throw new Error('no page');
  navigate(`/pages/${made.page.id}/edit`);
  return { kind: 'made', page: made.page, set_id: made.set_id ?? '' };
}

interface InputProps {
  field: FieldSpec;
  id: string;
  value: string;
  error?: string;
  names: Record<string, string>;
  spaceKey: string;
  onChange: (next: string) => void;
  onLeave: () => void;
  onName: (id: string, name: string) => void;
}

/** The label of a question: its name as typed, and "required" in words (TPL-315). */
function labelOf(field: FieldSpec): string {
  return field.required ? i18n.t('templates.requiredLabel', { name: field.name }) : field.name;
}

/** The ids an input names in aria-describedby: its help and its error. */
function describedBy(props: InputProps): string | undefined {
  const ids = [
    props.field.help ? fieldHelpId(props.id) : '',
    props.error ? fieldErrorId(props.id) : '',
  ].filter(Boolean);
  return ids.length > 0 ? ids.join(' ') : undefined;
}

/** Help and error under a group of inputs, where a Field cannot hold them. */
function GroupNotes({ props }: { props: InputProps }) {
  return (
    <>
      {props.field.help && (
        <small className="muted" id={fieldHelpId(props.id)}>
          {props.field.help}
        </small>
      )}
      {props.error && (
        <span className="field-error" id={fieldErrorId(props.id)}>
          {props.error}
        </span>
      )}
    </>
  );
}

function ChoiceInput(props: InputProps) {
  const { t } = useTranslation();
  const { field, id, value, onChange, onLeave } = props;
  const options = field.options ?? [];
  if (options.length > RADIO_LIMIT) {
    return (
      <Field label={labelOf(field)} help={field.help} error={props.error} id={id}>
        <select
          id={id}
          value={value}
          aria-invalid={!!props.error}
          aria-describedby={describedBy(props)}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onLeave}
        >
          <option value="">{t('organise.chooseOne')}</option>
          {options.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
      </Field>
    );
  }
  return (
    <fieldset
      className="choices"
      id={id}
      tabIndex={-1}
      aria-invalid={!!props.error}
      aria-describedby={describedBy(props)}
    >
      <legend>{labelOf(field)}</legend>
      {options.map((option) => (
        <label key={option} className="check">
          <input
            type="radio"
            name={id}
            checked={value === option}
            onChange={() => onChange(option)}
            onBlur={onLeave}
          />
          {option}
        </label>
      ))}
      <GroupNotes props={props} />
    </fieldset>
  );
}

function ChoicesInput(props: InputProps) {
  const { field, id, value, onChange, onLeave } = props;
  const chosen = value ? value.split('\n') : [];
  const flip = (option: string) => {
    const next = chosen.includes(option) ? chosen.filter((c) => c !== option) : [...chosen, option];
    // Keep the order of the options, so the page lists them as the template does.
    onChange((field.options ?? []).filter((o) => next.includes(o)).join('\n'));
  };
  return (
    <fieldset
      className="choices"
      id={id}
      tabIndex={-1}
      aria-invalid={!!props.error}
      aria-describedby={describedBy(props)}
    >
      <legend>{labelOf(field)}</legend>
      {(field.options ?? []).map((option) => (
        <label key={option} className="check">
          <input
            type="checkbox"
            checked={chosen.includes(option)}
            onChange={() => flip(option)}
            onBlur={onLeave}
          />
          {option}
        </label>
      ))}
      <GroupNotes props={props} />
    </fieldset>
  );
}

function PeopleInput(props: InputProps) {
  const { t } = useTranslation();
  const { field, id, value, onChange, names } = props;
  const chosen = value ? value.split('\n') : [];
  const many = field.kind === 'people';
  const full = many ? chosen.length >= MAX_PEOPLE : chosen.length >= 1;
  return (
    <fieldset
      className="choices"
      id={id}
      tabIndex={-1}
      aria-invalid={!!props.error}
      aria-describedby={describedBy(props)}
    >
      <legend>{labelOf(field)}</legend>
      {chosen.length > 0 && (
        <ul className="plain-list">
          {chosen.map((person) => (
            <li key={person} className="row">
              <span>{names[person] ?? t('templates.chosenPerson')}</span>
              <button
                type="button"
                className="ghost"
                onClick={() => {
                  onChange(chosen.filter((c) => c !== person).join('\n'));
                  props.onLeave();
                }}
              >
                {t('templates.removePerson', { name: names[person] ?? t('templates.chosenPerson') })}
              </button>
            </li>
          ))}
        </ul>
      )}
      {!full && (
        <SubjectPicker
          usersOnly
          label={many ? t('templates.addPerson') : t('templates.choosePerson')}
          onPick={(entry) => {
            props.onName(entry.id, entry.name);
            if (!chosen.includes(entry.id)) onChange([...chosen, entry.id].join('\n'));
            props.onLeave();
          }}
        />
      )}
      {field.default_gone && <p className="muted">{t('templates.defaultGone')}</p>}
      <GroupNotes props={props} />
    </fieldset>
  );
}

/** One question of the form, with the input its kind needs (TPL-300 to TPL-311). */
function FieldInput(props: InputProps) {
  const { t } = useTranslation();
  const { field, id, value, onChange, onLeave, error } = props;
  const common = {
    id,
    value,
    placeholder: field.placeholder,
    'aria-invalid': !!error,
    'aria-describedby': describedBy(props),
    onBlur: onLeave,
  };
  switch (field.kind) {
    case 'choice':
      return <ChoiceInput {...props} />;
    case 'choices':
      return <ChoicesInput {...props} />;
    case 'person':
    case 'people':
      return <PeopleInput {...props} />;
    case 'yesno':
      // A single checkbox with the question as its label; "no" is an answer (TPL-305).
      return (
        <div className="field">
          <label className="check">
            <input
              id={id}
              type="checkbox"
              checked={value === 'yes'}
              aria-describedby={describedBy(props)}
              onChange={(e) => onChange(e.target.checked ? 'yes' : 'no')}
              onBlur={onLeave}
            />
            {field.name}
          </label>
          <GroupNotes props={props} />
        </div>
      );
    case 'page':
      return (
        <div className="field" id={id} tabIndex={-1}>
          <PagePicker
            label={labelOf(field)}
            help={field.help ?? t('templates.pageHelp')}
            value={value}
            spaceKey={field.space_id ? props.spaceKey : undefined}
            onChange={(next) => {
              onChange(next);
              onLeave();
            }}
          />
          {error && (
            <span className="field-error" id={fieldErrorId(id)}>
              {error}
            </span>
          )}
        </div>
      );
    case 'text':
      return (
        <Field label={labelOf(field)} help={field.help} error={error} id={id}>
          <textarea rows={3} {...common} onChange={(e) => onChange(e.target.value)} />
        </Field>
      );
    case 'date':
      // The browser's own date input, which falls back to typing where it has none (TPL-1043).
      return (
        <Field label={labelOf(field)} help={field.help ?? t('templates.dateHelp')} error={error} id={id}>
          <input type="date" {...common} onChange={(e) => onChange(e.target.value)} />
        </Field>
      );
    case 'number':
      // Text with a decimal keyboard, so a scroll wheel never changes it (TPL-303).
      return (
        <Field label={labelOf(field)} help={field.help} error={error} id={id}>
          <input type="text" inputMode="decimal" {...common} onChange={(e) => onChange(e.target.value)} />
        </Field>
      );
    default:
      return (
        <Field label={labelOf(field)} help={field.help} error={error} id={id}>
          <input
            type="text"
            inputMode={field.kind === 'link' ? 'url' : field.format === 'email' ? 'email' : undefined}
            {...common}
            onChange={(e) => onChange(e.target.value)}
          />
        </Field>
      );
  }
}

/** The problems of a step, as a summary that links to each field (TPL-321). */
function ErrorSummary({
  problems,
  focusTo,
  summaryRef,
}: {
  problems: { name: string; id: string; words: string }[];
  focusTo: (id: string) => void;
  summaryRef: RefObject<HTMLDivElement | null>;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="error-summary"
      role="alert"
      tabIndex={-1}
      ref={summaryRef}
      aria-labelledby={`${problems[0]?.id}-summary`}
    >
      <h3 id={`${problems[0]?.id}-summary`}>{t('templates.problemTitle')}</h3>
      <ul>
        {problems.map((p) => (
          <li key={p.id}>
            <a
              href={`#${p.id}`}
              onClick={(e) => {
                e.preventDefault();
                focusTo(p.id);
              }}
            >
              {p.name}: {p.words}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** An answer as the check step shows it (TPL-1024). */
function AnswerWords({
  field,
  value,
  names,
}: {
  field: FieldSpec;
  value: string;
  names: Record<string, string>;
}) {
  const { t } = useTranslation();
  const page = useQuery({
    queryKey: ['picked-page', value],
    queryFn: () => api.get<{ title: string }>(`/pages/${value}`),
    enabled: field.kind === 'page' && value !== '',
    retry: false,
  });
  if (field.kind === 'yesno') return <>{value === 'yes' ? t('common.yes') : t('common.no')}</>;
  if (value === '') return <span className="muted">{t('templates.noAnswer')}</span>;
  switch (field.kind) {
    case 'date':
      return <>{readableDate(value)}</>;
    case 'person':
    case 'people':
      return (
        <>
          {value
            .split('\n')
            .map((id) => names[id] ?? t('templates.chosenPerson'))
            .join(', ')}
        </>
      );
    case 'choices':
      return <>{value.split('\n').join(', ')}</>;
    case 'page':
      return <>{page.data?.title ?? t('picker.pageGone')}</>;
  }
  return <>{value}</>;
}

/** The pages a set will make, indented as they will sit in the tree (TPL-340). */
function PlannedPages({ pages, parent }: { pages: PlannedPage[]; parent: string }) {
  const { t } = useTranslation();
  const below = pages.filter((p) => p.parent_entry === parent);
  if (below.length === 0) return null;
  return (
    <ul className="planned-pages">
      {below.map((p) => (
        <li key={p.entry}>
          <span className="planned-title">{pageTitle(t, p.title)}</span>
          {p.labels.map((label) => (
            <span key={label} className="planned-label">
              #{label}
            </span>
          ))}
          {p.restricted && (
            <span className="planned-lock">
              <IconLock size={14} /> {t(`templates.restricted_${p.restricted}`)}
            </span>
          )}
          {p.exists && <span className="muted"> {t('templates.titleExists')}</span>}
          <PlannedPages pages={pages} parent={p.entry} />
        </li>
      ))}
    </ul>
  );
}

function CheckStep({
  check,
  steps,
  answers,
  names,
  ids,
  onChange,
}: {
  check: SetCheck;
  steps: FormStep[];
  answers: Record<string, string>;
  names: Record<string, string>;
  ids: Record<string, string>;
  onChange: (step: number, fieldId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {steps.map((step, index) =>
        step.kind !== 'fields' ? null : (
          <section key={index} aria-label={step.title || t('templates.yourAnswers')}>
            <h4>{step.title || t('templates.yourAnswers')}</h4>
            <dl className="check-answers">
              {step.fields.map((f) => (
                <div key={f.name} className="check-answer">
                  <dt>{f.name}</dt>
                  <dd>
                    <AnswerWords field={f} value={answers[f.name] ?? ''} names={names} />
                  </dd>
                  <dd>
                    <button
                      type="button"
                      className="ghost"
                      onClick={() => onChange(index, ids[f.name] ?? '')}
                    >
                      {t('templates.changeAnswer', { name: f.name })}
                    </button>
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        ),
      )}
      <section aria-labelledby="planned-pages-title">
        <h4 id="planned-pages-title">{t('templates.pagesMade')}</h4>
        <p className="muted">{check.publish ? t('templates.willPublish') : t('templates.willDraft')}</p>
        {check.default_parent_unusable && <p className="note">{t('templates.defaultParentGone')}</p>}
        <PlannedPages pages={check.pages} parent="" />
        {check.background && (
          <p className="muted">{t('templates.background', { count: check.pages.length })}</p>
        )}
      </section>
    </>
  );
}

/** Where the pages go, and the labels they get (TPL-333). */
function PlaceStep({
  template,
  space,
  place,
  onPlace,
}: {
  template: TemplateSummary;
  space: Space;
  place: string;
  onPlace: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <PagePicker
        label={t('templates.place')}
        help={t('templates.placeHelp')}
        value={place}
        spaceKey={space.key}
        onChange={onPlace}
      />
      <p className="muted">
        {place ? t('templates.placeBelow') : t('templates.placeTop', { space: space.name })}
      </p>
      <p>
        <strong>{t('organise.labels')}</strong>{' '}
        {template.labels.length > 0 ? (
          template.labels.map((label) => (
            <span key={label} className="planned-label">
              #{label}
            </span>
          ))
        ) : (
          <span className="muted">{t('templates.noLabels')}</span>
        )}
      </p>
    </>
  );
}

/**
 * The form of a template, in the gallery's dialog. The answers live here, so Back and a
 * refusal of the server keep every one of them (TPL-324, TPL-332).
 */
export function TemplateForm({
  template,
  space,
  parent,
  replaces,
  guidedOn,
  onBack,
  onClose,
  onDirty,
  tryOnly,
}: {
  template: TemplateSummary;
  space: Space;
  parent: string | null;
  replaces?: string;
  /** Whether the check step can be worked out on the server (the part guided templates). */
  guidedOn: boolean;
  onBack: () => void;
  onClose: () => void;
  /** Called once something is typed, so closing can ask first. */
  onDirty: () => void;
  /** Try the form as its author: the check step is a dry run, and nothing is made (TPL-1012). */
  tryOnly?: boolean;
}) {
  const { t } = useTranslation();
  const formId = useId();
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const fields = useMemo(() => (template.fields ?? []).map(fieldSpecOf), [template.fields]);
  const ids = useMemo(
    () => Object.fromEntries(fields.map((f, i) => [f.name, `${formId}-field-${i}`])),
    [fields, formId],
  );
  const today = useMemo(localToday, []);
  const [answers, setAnswers] = useState<Record<string, string> | null>(null);
  const [names, setNames] = useState<Record<string, string>>({});
  const [left, setLeft] = useState<Set<string>>(new Set());
  const [tried, setTried] = useState<Set<number>>(new Set());
  const [served, setServed] = useState<Record<string, FieldProblem>>({});
  const [at, setAt] = useState(0);
  const [place, setPlace] = useState(parent ?? template.default_parent ?? '');
  const [placeChanged, setPlaceChanged] = useState(false);
  const [summaryShown, setSummaryShown] = useState(0);
  const [focusAfter, setFocusAfter] = useState('');
  const summaryRef = useRef<HTMLDivElement>(null);
  const [requestKey] = useState(() => crypto.randomUUID());
  const creator = me.data?.user.id ?? '';
  const start = useMemo(
    () => (creator ? startingAnswers(fields, today, creator) : null),
    [fields, today, creator],
  );
  const current = answers ?? start ?? {};
  // The creator's own name is known without a pick, for a default of "the person filling in".
  const allNames = me.data ? { [me.data.user.id]: me.data.user.display_name, ...names } : names;
  const shown = useMemo(
    () =>
      shownQuestions(template, current, {
        today,
        space: space.id,
        groups: template.condition_groups?.member ?? [],
        missingGroups: template.condition_groups?.missing ?? [],
      }),
    [template, current, today, space.id],
  );
  // The check step is for a guided template, whose dry run the server works out (TPL-340).
  const steps = useMemo(
    () => formSteps(template, guidedOn && template.guided, shown),
    [template, guidedOn, shown],
  );
  const step = steps[at];
  // A question a condition shows appears after the one that caused it, and is announced in a
  // polite live region; the focus never moves by itself (TPL-1023).
  const asked = step?.kind === 'fields' ? step.fields.length : 0;
  const [counted, setCounted] = useState({ at, asked });
  const [announced, setAnnounced] = useState('');
  if (counted.at !== at || counted.asked !== asked) {
    const more = asked - counted.asked;
    setAnnounced(
      counted.at !== at
        ? ''
        : more > 0
          ? t('templates.moreQuestions', { count: more })
          : t('templates.fewerQuestions', { count: -more }),
    );
    setCounted({ at, asked });
  }

  useEffect(() => {
    if (summaryShown > 0) summaryRef.current?.focus();
  }, [summaryShown]);
  useEffect(() => {
    if (!focusAfter) return;
    document.getElementById(focusAfter)?.focus();
    setFocusAfter('');
  }, [focusAfter, at]);

  const body = () => ({
    parent_id: placeChanged ? place || null : parent,
    // An answer to a question the person was not shown is not sent (TPL-1407).
    answers: Object.fromEntries(Object.entries(current).filter(([name]) => shown.has(name))),
    replaces: replaces ?? null,
  });
  const showServerProblems = (error: unknown) => {
    if (!(error instanceof ApiError) || error.fields.length === 0) return;
    setServed(Object.fromEntries(error.fields.map((f) => [f.field, f])));
    // The form goes back to the first step that holds a refused answer, keeping every answer.
    const first = steps.findIndex(
      (s) => s.kind === 'fields' && s.fields.some((f) => error.fields.some((p) => p.field === f.name)),
    );
    if (first >= 0) {
      setAt(first);
      setTried((t0) => new Set(t0).add(first));
      setSummaryShown((n) => n + 1);
    }
  };
  const check = useMutation({
    mutationFn: () =>
      api.post<SetCheck>(`/templates/${template.id}/sets/check`, { space_id: space.id, ...body() }),
    onError: showServerProblems,
  });
  const make = useMutation({
    mutationFn: () => startPage(template, space, body(), requestKey),
    onSuccess: (outcome) => {
      if (outcome.kind === 'made') onClose();
    },
    onError: showServerProblems,
  });

  if (!start) return me.error ? <ErrorNote error={me.error} /> : <Loading />;

  const problemOf = (f: FieldSpec): string | undefined => {
    const fromServer = served[f.name];
    if (fromServer) return problemWords(fromServer);
    const [, p] = checkAnswer(f, current[f.name] ?? '', today);
    return p ? problemWords(p) : undefined;
  };
  // An error shows once the person left the field or pressed Next, never while they first
  // type in an empty one, and goes as soon as the answer is right (TPL-322).
  const shownProblem = (f: FieldSpec) => (left.has(f.name) || tried.has(at) ? problemOf(f) : undefined);
  const stepProblems =
    step?.kind === 'fields'
      ? step.fields.flatMap((f) => {
          const words = problemOf(f);
          return words ? [{ name: f.name, id: ids[f.name] ?? '', words }] : [];
        })
      : [];
  const change = (name: string, value: string) => {
    setAnswers({ ...current, [name]: value });
    const rest = { ...served };
    delete rest[name];
    setServed(rest);
    onDirty();
  };
  const goTo = (index: number) => {
    setAt(index);
    if (steps[index]?.kind === 'check') check.mutate();
  };
  const next = (e: FormEvent) => {
    e.preventDefault();
    if (stepProblems.length > 0) {
      setTried(new Set(tried).add(at));
      setSummaryShown((n) => n + 1);
      return;
    }
    if (at < steps.length - 1) goTo(at + 1);
    else if (tryOnly) onClose();
    else make.mutate();
  };
  const last = at === steps.length - 1;
  const pages = check.data?.pages.length ?? template.pages ?? 1;
  // The primary button says what it does: the number of pages, and whether they are drafts.
  const primaryLabel = (): string => {
    if (!last) return t('organise.next');
    if (tryOnly) return t('templates.tryDone');
    if (step?.kind !== 'check') return t('organise.startPage');
    return check.data?.publish
      ? t('templates.createPages', { count: pages })
      : t('templates.createDrafts', { count: pages });
  };
  const createLabel = primaryLabel();

  if (make.data?.kind === 'queued') {
    return (
      <>
        <p role="status" className="note">
          {t('templates.setQueued')}
        </p>
        <div className="dialog-actions">
          <button type="button" className="primary" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </>
    );
  }
  const heading =
    step?.kind === 'check'
      ? t('templates.checkTitle')
      : step?.title || (step?.fields.length === 0 ? t('templates.placeTitle') : '');
  return (
    <form noValidate onSubmit={next} className="template-form">
      <p className="visually-hidden" aria-live="polite">
        {announced}
      </p>
      {heading && <h3>{heading}</h3>}
      {steps.length > 1 && (
        <p role="status" className="muted">
          {t('organise.stepOf', { at: at + 1, total: steps.length })}
        </p>
      )}
      {tried.has(at) && stepProblems.length > 0 && (
        <ErrorSummary
          problems={stepProblems}
          summaryRef={summaryRef}
          focusTo={(id) => document.getElementById(id)?.focus()}
        />
      )}
      {step?.kind === 'check' && <ErrorNote error={make.error ?? check.error} />}
      {step?.kind !== 'check' && !(make.error instanceof ApiError && make.error.fields.length > 0) && (
        <ErrorNote error={make.error} />
      )}
      {step?.kind === 'fields' &&
        step.fields.map((f) => (
          <FieldInput
            key={f.name}
            field={f}
            id={ids[f.name] ?? ''}
            value={current[f.name] ?? ''}
            error={shownProblem(f)}
            names={allNames}
            spaceKey={space.key}
            onChange={(value) => change(f.name, value)}
            onLeave={() => setLeft(new Set(left).add(f.name))}
            onName={(id, name) => setNames({ ...names, [id]: name })}
          />
        ))}
      {step?.kind === 'fields' && step.place && (
        <PlaceStep
          template={template}
          space={space}
          place={place}
          onPlace={(id) => {
            setPlace(id);
            setPlaceChanged(true);
          }}
        />
      )}
      {step?.kind === 'check' && check.data && (
        <CheckStep
          check={check.data}
          steps={steps}
          answers={current}
          names={allNames}
          ids={ids}
          onChange={(index, fieldId) => {
            setAt(index);
            setFocusAfter(fieldId);
          }}
        />
      )}
      {step?.kind === 'check' && check.isPending && <p className="muted">{t('templates.checking')}</p>}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={at === 0 ? onBack : () => setAt(at - 1)}>
          {t('organise.back')}
        </button>
        <button
          type="submit"
          className="primary"
          disabled={make.isPending || (step?.kind === 'check' && !check.data)}
        >
          {createLabel}
        </button>
      </div>
    </form>
  );
}

/** Asks before a form with answers closes (TPL-332). */
export function CloseFormDialog({ onConfirm, onClose }: { onConfirm: () => void; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <ConfirmDialog
      title={t('templates.closeTitle')}
      message={t('templates.closeMessage')}
      confirmLabel={t('templates.closeConfirm')}
      onConfirm={onConfirm}
      onClose={onClose}
    />
  );
}
