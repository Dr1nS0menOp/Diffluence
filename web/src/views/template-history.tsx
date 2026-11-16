// SPDX-License-Identifier: Apache-2.0
// The parts of a template's history beyond its texts (spec 074, wave GT5): the readable list of
// what a version changed (TPL-602), and the sets a version made (TPL-613).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type TemplateImportNotes, type TemplateSettingChange, type TemplateVersionSets } from '../api';
import { useDates } from '../dates';
import { Link } from '../router';
import { pageTitle } from '../page-name';
import { conditionSentence } from '../template-conditions';
import { Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { useConditionScope } from './template-rules';

/** One line of the change list in words: names as the author typed them, the rest translated. */
function ChangeLine({ change, nameOf }: { change: TemplateSettingChange; nameOf: (id: string) => string }) {
  const { t } = useTranslation();
  const condition = change.condition
    ? conditionSentence(change.condition, { name: nameOf, date: (day) => day })
    : '';
  const kind =
    change.code.startsWith('field_') && change.value ? t(`templates.kind_${change.value}`) : change.value;
  return (
    <li>
      {t(`templates.history.change_${change.code}`, {
        entry: change.entry || t('templates.firstPage'),
        field: change.field ?? '',
        step: change.step ?? '',
        value: kind ?? '',
        condition,
      })}
      {change.code === 'field_added' && change.required && <> {t('templates.history.required')}</>}
    </li>
  );
}

/** What a version changed besides its texts, one line each (TPL-602). */
export function ChangeList({ templateId, list }: { templateId: string; list: TemplateSettingChange[] }) {
  const { t } = useTranslation();
  const scope = useConditionScope(templateId);
  if (list.length === 0) return null;
  const nameOf = (id: string) => scope?.names[id] ?? t('templates.history.unknownName');
  return (
    <section aria-labelledby="change-list-heading">
      <h3 id="change-list-heading">{t('templates.history.changeListTitle')}</h3>
      <ul className="plain-list">
        {list.map((c, i) => (
          <ChangeLine key={`${c.code}-${i}`} change={c} nameOf={nameOf} />
        ))}
      </ul>
    </section>
  );
}

/** The sets one version made, showing only first pages the reader may see (TPL-613). */
function VersionSetsDialog({
  templateId,
  number,
  onClose,
}: {
  templateId: string;
  number: number;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['template-version-sets', templateId, number],
    queryFn: () => api.get<TemplateVersionSets>(`/templates/${templateId}/versions/${number}/sets`),
  });
  return (
    <Dialog title={t('templates.history.setsTitle', { number })} onClose={onClose}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && (
        <>
          <p>{t('templates.history.setsCount', { count: q.data.count })}</p>
          {q.data.sets.length > 0 && (
            <ul className="plain-list details-links">
              {q.data.sets.map((s) => (
                <li key={s.id}>
                  <Link href={`/pages/${s.root.id}`}>{pageTitle(t, s.root.title)}</Link>{' '}
                  <span className="muted">
                    {t('templates.history.setPages', { count: s.pages })}, {dates.moment(s.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {q.data.hidden > 0 && (
            <p className="muted">{t('templates.history.setsHidden', { count: q.data.hidden })}</p>
          )}
        </>
      )}
    </Dialog>
  );
}

/** The count of sets a version made, with a button that lists them, for its managers. */
export function VersionSetsCell({
  templateId,
  number,
  sets,
}: {
  templateId: string;
  number: number;
  sets: number;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  if (sets === 0) return <span className="muted">{t('templates.history.noSets')}</span>;
  return (
    <>
      <button type="button" className="ghost" onClick={() => setOpen(true)}>
        {t('templates.history.showSets', { count: sets, number })}
      </button>
      {open && <VersionSetsDialog templateId={templateId} number={number} onClose={() => setOpen(false)} />}
    </>
  );
}

/**
 * What an import left out and asks to choose again (TPL-852): it waits as a draft, and is not
 * approved before the people and groups are chosen here.
 */
export function ImportNote({ name, notes }: { name: string; notes: TemplateImportNotes }) {
  const { t } = useTranslation();
  const lines = [
    notes.people_left_out > 0 && t('templates.history.importPeople', { count: notes.people_left_out }),
    notes.groups_to_choose.length > 0 &&
      t('templates.history.importGroups', { names: notes.groups_to_choose.join(', ') }),
    notes.condition_groups_to_choose > 0 &&
      t('templates.history.importConditionGroups', { count: notes.condition_groups_to_choose }),
    notes.suggestions_left_out.length > 0 &&
      t('templates.history.importSuggestions', { names: notes.suggestions_left_out.join(', ') }),
  ].filter((line): line is string => typeof line === 'string');
  return (
    <div className="note" role="status">
      <p>{t('templates.history.imported', { name })}</p>
      {lines.length > 0 && (
        <ul className="plain-list">
          {lines.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** Which version the shown one is compared with: the one before it, or any earlier (TPL-602). */
export function CompareWith({
  number,
  against,
  onChange,
}: {
  number: number;
  against: number;
  onChange: (against: number) => void;
}) {
  const { t } = useTranslation();
  if (number <= 2) return null;
  const earlier = Array.from({ length: number - 1 }, (_, i) => number - 1 - i);
  return (
    <Field label={t('templates.history.compareWith')}>
      <select value={against > 0 ? against : number - 1} onChange={(e) => onChange(Number(e.target.value))}>
        {earlier.map((n) => (
          <option key={n} value={n}>
            {t('templates.history.versionNumber', { number: n })}
          </option>
        ))}
      </select>
    </Field>
  );
}
