// SPDX-License-Identifier: Apache-2.0
// Buttons that narrow the search box: spaces, type, labels, people and date. Each opens
// one small chooser below the row; what is chosen becomes words of the search syntax, or
// the parameters type and date (spec 033).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SubjectPicker } from './picker';
import { useOrderedSpaces } from './space-strip';
import { filterCount, spaceChoices, type SearchFilters } from './search-filters';

const KINDS = ['page', 'post', 'database', 'comment'];
const WITHIN = ['day', 'week', 'month', 'year'];
const SPACES_SHOWN = 20;
type Part = 'spaces' | 'kind' | 'labels' | 'people' | 'within';
const PARTS: Part[] = ['spaces', 'kind', 'labels', 'people', 'within'];

function countOf(filters: SearchFilters, part: Part): number {
  if (part === 'spaces') return filters.spaces.length;
  if (part === 'labels') return filters.labels.length;
  if (part === 'people') return filters.people.length;
  return filters[part] ? 1 : 0;
}

function toggled(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
}

export function SearchFilterBar({
  filters,
  onChange,
}: {
  filters: SearchFilters;
  onChange: (next: SearchFilters) => void;
}) {
  const { t } = useTranslation();
  const [part, setPart] = useState<Part | null>(null);
  const [label, setLabel] = useState('');
  const [spaceText, setSpaceText] = useState('');
  const spaces = spaceChoices(useOrderedSpaces(), filters.spaces, spaceText, SPACES_SHOWN);
  const set = (change: Partial<SearchFilters>) => onChange({ ...filters, ...change });
  const choice = (pressed: boolean, text: string, onClick: () => void, key: string) => (
    <button key={key} type="button" className="filter-choice" aria-pressed={pressed} onClick={onClick}>
      {text}
    </button>
  );
  return (
    <div className="quick-filters" role="group" aria-label={t('search.filters')}>
      <div className="filter-row">
        {PARTS.map((name) => {
          const count = countOf(filters, name);
          return (
            <button
              key={name}
              type="button"
              className="filter-button"
              aria-expanded={part === name}
              aria-controls="quick-filter-choices"
              onClick={() => setPart(part === name ? null : name)}
            >
              {t(`search.filter_${name}`)}
              {count > 0 && <span className="filter-count">{count}</span>}
            </button>
          );
        })}
        {filterCount(filters) > 0 && (
          <button
            type="button"
            className="ghost"
            onClick={() => onChange({ spaces: [], labels: [], people: [], kind: '', within: '' })}
          >
            {t('search.clearFilters')}
          </button>
        )}
      </div>
      {part && (
        <div id="quick-filter-choices" className="filter-choices">
          {part === 'spaces' && (
            <>
              {choice(filters.spaces.length === 0, t('search.allSpaces'), () => set({ spaces: [] }), 'all')}
              {spaces.map((s) =>
                choice(
                  filters.spaces.includes(s.key),
                  s.name,
                  () => set({ spaces: toggled(filters.spaces, s.key) }),
                  s.id,
                ),
              )}
              <input
                type="search"
                className="filter-input"
                value={spaceText}
                aria-label={t('search.findSpace')}
                placeholder={t('search.findSpace')}
                onChange={(e) => setSpaceText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key !== 'Enter') return;
                  // Enter picks the first match here, rather than sending the search.
                  e.preventDefault();
                  const first = spaces.find((s) => !filters.spaces.includes(s.key));
                  if (!first || !spaceText.trim()) return;
                  set({ spaces: [...filters.spaces, first.key] });
                  setSpaceText('');
                }}
              />
            </>
          )}
          {part === 'kind' && (
            <>
              {choice(!filters.kind, t('search.kind_any'), () => set({ kind: '' }), 'any')}
              {KINDS.map((k) => choice(filters.kind === k, t(`search.kind_${k}`), () => set({ kind: k }), k))}
            </>
          )}
          {part === 'within' && (
            <>
              {choice(!filters.within, t('search.within_any'), () => set({ within: '' }), 'any')}
              {WITHIN.map((w) =>
                choice(filters.within === w, t(`search.within_${w}`), () => set({ within: w }), w),
              )}
            </>
          )}
          {part === 'labels' && (
            <>
              {filters.labels.map((l) =>
                choice(true, l, () => set({ labels: filters.labels.filter((x) => x !== l) }), l),
              )}
              <input
                className="filter-input"
                value={label}
                aria-label={t('search.addLabelFilter')}
                placeholder={t('search.addLabelFilter')}
                onChange={(e) => setLabel(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key !== 'Enter') return;
                  e.preventDefault();
                  const value = label.trim().toLowerCase();
                  if (value && !filters.labels.includes(value)) set({ labels: [...filters.labels, value] });
                  setLabel('');
                }}
              />
            </>
          )}
          {part === 'people' && (
            <>
              {filters.people.map((p) =>
                choice(true, p, () => set({ people: filters.people.filter((x) => x !== p) }), p),
              )}
              <SubjectPicker
                label={t('search.addPersonFilter')}
                usersOnly
                onPick={(person) =>
                  !filters.people.includes(person.name) && set({ people: [...filters.people, person.name] })
                }
              />
            </>
          )}
        </div>
      )}
    </div>
  );
}
