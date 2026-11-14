// SPDX-License-Identifier: Apache-2.0
// The settings of one question of a template, in a Dialog with only the settings its kind
// has (spec 074, TPL-1012). The validator's words stand next to the setting they concern,
// before anything is saved (WCAG 3.3.1).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  checkField,
  FIELD_KINDS,
  KIND_SETTINGS,
  LINE_FORMATS,
  localToday,
  type DateRef,
  type FieldSpec,
} from '../template-fields';
import { problemWords } from '../components/template-form';
import { Dialog, Field } from '../components/ui';

/** Which setting a problem of the validator concerns, so its words stand beside it. */
const PROBLEM_SETTING: Record<string, string> = {
  yesno_required: 'required',
  help_too_long: 'help',
  placeholder_too_long: 'placeholder',
  needs_options: 'options',
  too_many_options: 'options',
  bad_option: 'options',
  bad_length: 'length',
  bad_format: 'format',
  bad_decimals: 'number',
  bad_range: 'number',
  bad_date_bound: 'dates',
  bad_choices: 'choices',
  bad_space: 'space',
  bad_show: 'show',
  bad_default: 'default',
  setting_not_for_kind: 'kind',
  unknown_kind: 'kind',
};

/** The ways a yes/no field or choices can show in a page, besides the usual one. */
const SHOW_WAYS = ['none', 'list'] as const;

/** Drops the settings a kind does not have, so changing the kind never leaves one behind. */
export function onlySettingsOf(spec: FieldSpec): FieldSpec {
  const has = KIND_SETTINGS[spec.kind];
  const out: FieldSpec = {
    name: spec.name,
    kind: spec.kind,
    required: spec.kind !== 'yesno' && spec.required,
  };
  if (spec.help) out.help = spec.help;
  if (has.includes('placeholder') && spec.placeholder) out.placeholder = spec.placeholder;
  if (has.includes('options') && spec.options?.length) out.options = spec.options;
  if (has.includes('length')) Object.assign(out, pick(spec, ['min_length', 'max_length']));
  if (has.includes('format') && spec.format) out.format = spec.format;
  if (has.includes('number')) Object.assign(out, pick(spec, ['min', 'max', 'decimals']));
  if (has.includes('dates')) Object.assign(out, pick(spec, ['earliest', 'latest']));
  if (has.includes('choices')) Object.assign(out, pick(spec, ['min_choices', 'max_choices']));
  if (has.includes('space') && spec.space_id) out.space_id = spec.space_id;
  if (has.includes('show') && spec.show) out.show = spec.show;
  const d = spec.default;
  const fits =
    d &&
    ((d.creator && (spec.kind === 'person' || spec.kind === 'people')) ||
      (d.date && spec.kind === 'date') ||
      (d.value && spec.kind !== 'date'));
  if (fits) out.default = d;
  return out;
}

function pick(spec: FieldSpec, keys: (keyof FieldSpec)[]): Partial<FieldSpec> {
  return Object.fromEntries(
    keys.filter((k) => spec[k] !== undefined && spec[k] !== '').map((k) => [k, spec[k]]),
  );
}

/** Reads a number typed into a setting; empty means not set. */
const numberOrNone = (text: string): number | undefined => (text.trim() === '' ? undefined : Number(text));

/** A day limit or default: none, the day the pages are made moved by days, or a fixed day. */
function DayChoice({
  label,
  value,
  onChange,
  error,
}: {
  label: string;
  value: DateRef | undefined;
  onChange: (next: DateRef | undefined) => void;
  error?: string;
}) {
  const { t } = useTranslation();
  const how = !value ? 'none' : value.fixed !== undefined ? 'fixed' : 'relative';
  return (
    <fieldset className="choices">
      <legend>{label}</legend>
      <select
        aria-label={label}
        value={how}
        onChange={(e) =>
          onChange(
            e.target.value === 'none'
              ? undefined
              : e.target.value === 'fixed'
                ? { fixed: localToday() }
                : { offset: 0 },
          )
        }
      >
        <option value="none">{t('templates.dayNone')}</option>
        <option value="relative">{t('templates.dayRelative')}</option>
        <option value="fixed">{t('templates.dayFixed')}</option>
      </select>
      {how === 'fixed' && (
        <Field label={t('templates.dayFixedLabel')}>
          <input
            type="date"
            value={value?.fixed ?? ''}
            onChange={(e) => onChange({ fixed: e.target.value })}
          />
        </Field>
      )}
      {how === 'relative' && (
        <div className="row">
          <Field label={t('templates.dayOffset')} help={t('templates.dayOffsetHelp')}>
            <input
              type="text"
              inputMode="numeric"
              value={String(value?.offset ?? 0)}
              onChange={(e) => onChange({ ...value, offset: Number(e.target.value) || 0 })}
            />
          </Field>
          <Field label={t('templates.dayUnit')}>
            <select
              value={value?.unit ?? 'day'}
              onChange={(e) => onChange({ ...value, unit: e.target.value === 'week' ? 'week' : 'day' })}
            >
              <option value="day">{t('templates.unitDay')}</option>
              <option value="week">{t('templates.unitWeek')}</option>
            </select>
          </Field>
        </div>
      )}
      {error && <span className="field-error">{error}</span>}
    </fieldset>
  );
}

/** The settings of one question. `spaceId` is the template's space, for a page field's limit. */
export function FieldDialog({
  start,
  spaceId,
  onSave,
  onClose,
}: {
  start: FieldSpec;
  spaceId: string | null;
  onSave: (spec: FieldSpec) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [spec, setSpec] = useState<FieldSpec>(start);
  const [tried, setTried] = useState(false);
  const set = (more: Partial<FieldSpec>) => setSpec({ ...spec, ...more });
  const cleaned = onlySettingsOf(spec);
  const problem = checkField(cleaned, localToday());
  const errorFor = (setting: string) =>
    (tried || setting !== 'required') && problem && PROBLEM_SETTING[problem.problem] === setting
      ? problemWords(problem)
      : undefined;
  const has = KIND_SETTINGS[spec.kind];
  const save = (e: FormEvent) => {
    e.preventDefault();
    // The dialog stands inside the Form section's own form; its submit is not that form's.
    e.stopPropagation();
    setTried(true);
    if (!problem) onSave(cleaned);
  };
  return (
    <Dialog title={t('templates.fieldTitle', { name: spec.name })} onClose={onClose}>
      <form noValidate onSubmit={save}>
        <Field label={t('templates.fieldKind')} error={errorFor('kind')}>
          <select
            value={spec.kind}
            onChange={(e) => set({ kind: FIELD_KINDS.find((k) => k === e.target.value) ?? 'line' })}
          >
            {FIELD_KINDS.map((k) => (
              <option key={k} value={k}>
                {t(`templates.kind_${k}`)}
              </option>
            ))}
          </select>
        </Field>
        {spec.kind !== 'yesno' && (
          <label className="check">
            <input
              type="checkbox"
              checked={spec.required}
              onChange={(e) => set({ required: e.target.checked })}
            />
            {t('organise.required')}
          </label>
        )}
        <Field label={t('templates.fieldHelp')} help={t('templates.fieldHelpHelp')} error={errorFor('help')}>
          <input value={spec.help ?? ''} onChange={(e) => set({ help: e.target.value })} />
        </Field>
        {has.includes('placeholder') && (
          <Field
            label={t('templates.fieldPlaceholder')}
            help={t('templates.fieldPlaceholderHelp')}
            error={errorFor('placeholder')}
          >
            <input value={spec.placeholder ?? ''} onChange={(e) => set({ placeholder: e.target.value })} />
          </Field>
        )}
        {has.includes('options') && (
          <Field
            label={t('templates.fieldOptions')}
            help={t('templates.fieldOptionsHelp')}
            error={errorFor('options')}
          >
            <textarea
              rows={4}
              value={(spec.options ?? []).join('\n')}
              onChange={(e) => set({ options: e.target.value.split('\n') })}
            />
          </Field>
        )}
        {has.includes('length') && (
          <div className="row">
            <Field label={t('templates.minLength')} error={errorFor('length')}>
              <input
                inputMode="numeric"
                value={spec.min_length ?? ''}
                onChange={(e) => set({ min_length: numberOrNone(e.target.value) })}
              />
            </Field>
            <Field label={t('templates.maxLength')}>
              <input
                inputMode="numeric"
                value={spec.max_length ?? ''}
                onChange={(e) => set({ max_length: numberOrNone(e.target.value) })}
              />
            </Field>
          </div>
        )}
        {has.includes('format') && (
          <Field label={t('templates.format')} error={errorFor('format')}>
            <select
              value={spec.format ?? ''}
              onChange={(e) => set({ format: LINE_FORMATS.find((f) => f === e.target.value) })}
            >
              <option value="">{t('templates.formatAny')}</option>
              {LINE_FORMATS.map((f) => (
                <option key={f} value={f}>
                  {t(`templates.format_${f}`)}
                </option>
              ))}
            </select>
          </Field>
        )}
        {has.includes('number') && (
          <div className="row">
            <Field label={t('templates.min')} error={errorFor('number')}>
              <input
                inputMode="decimal"
                value={spec.min ?? ''}
                onChange={(e) => set({ min: numberOrNone(e.target.value) })}
              />
            </Field>
            <Field label={t('templates.max')}>
              <input
                inputMode="decimal"
                value={spec.max ?? ''}
                onChange={(e) => set({ max: numberOrNone(e.target.value) })}
              />
            </Field>
            <Field label={t('templates.decimals')}>
              <select value={spec.decimals ?? 0} onChange={(e) => set({ decimals: Number(e.target.value) })}>
                {[0, 1, 2, 3, 4].map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </Field>
          </div>
        )}
        {has.includes('dates') && (
          <>
            <DayChoice
              label={t('templates.earliest')}
              value={spec.earliest}
              onChange={(earliest) => set({ earliest })}
              error={errorFor('dates')}
            />
            <DayChoice
              label={t('templates.latest')}
              value={spec.latest}
              onChange={(latest) => set({ latest })}
            />
          </>
        )}
        {has.includes('choices') && (
          <div className="row">
            <Field label={t('templates.minChoices')} error={errorFor('choices')}>
              <input
                inputMode="numeric"
                value={spec.min_choices ?? ''}
                onChange={(e) => set({ min_choices: numberOrNone(e.target.value) })}
              />
            </Field>
            <Field label={t('templates.maxChoices')}>
              <input
                inputMode="numeric"
                value={spec.max_choices ?? ''}
                onChange={(e) => set({ max_choices: numberOrNone(e.target.value) })}
              />
            </Field>
          </div>
        )}
        {has.includes('space') && spaceId && (
          <label className="check">
            <input
              type="checkbox"
              checked={spec.space_id === spaceId}
              onChange={(e) => set({ space_id: e.target.checked ? spaceId : undefined })}
            />
            {t('templates.onlyThisSpace')}
          </label>
        )}
        {has.includes('show') && (
          <Field label={t('templates.showAs')} error={errorFor('show')}>
            <select
              value={spec.show ?? ''}
              onChange={(e) => set({ show: SHOW_WAYS.find((how) => how === e.target.value) })}
            >
              <option value="">{t('templates.showUsual')}</option>
              {(spec.kind === 'yesno' ? (['none'] as const) : (['list'] as const)).map((how) => (
                <option key={how} value={how}>
                  {t(`templates.show_${how}`)}
                </option>
              ))}
            </select>
          </Field>
        )}
        <DefaultSetting spec={spec} onChange={(d) => set({ default: d })} error={errorFor('default')} />
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="primary">
            {t('templates.fieldDone')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** What a question shows filled in (TPL-316): a value, a day, or the person filling in. */
function DefaultSetting({
  spec,
  onChange,
  error,
}: {
  spec: FieldSpec;
  onChange: (next: FieldSpec['default']) => void;
  error?: string;
}) {
  const { t } = useTranslation();
  if (spec.kind === 'date') {
    return (
      <DayChoice
        label={t('templates.default')}
        value={spec.default?.date}
        onChange={(date) => onChange(date ? { date } : undefined)}
        error={error}
      />
    );
  }
  if (spec.kind === 'person' || spec.kind === 'people') {
    return (
      <label className="check">
        <input
          type="checkbox"
          checked={!!spec.default?.creator}
          onChange={(e) => onChange(e.target.checked ? { creator: true } : undefined)}
        />
        {t('templates.defaultCreator')}
      </label>
    );
  }
  if (spec.kind === 'page') return null;
  return (
    <Field label={t('templates.default')} help={t('templates.defaultHelp')} error={error}>
      <input
        value={spec.default?.value ?? ''}
        onChange={(e) => onChange(e.target.value ? { value: e.target.value } : undefined)}
      />
    </Field>
  );
}
