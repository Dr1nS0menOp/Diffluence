// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api, type DirectoryEntry, type SearchAnswer, type SearchResult, type Space } from '../api';
import { Link, navigate, useLocation } from '../router';
import { StageMark } from '../components/page-marks';
import { SubjectPicker } from '../components/picker';
import { ErrorNote, Loading } from '../components/ui';
import { NO_FILTERS, queryWith, splitQuery } from '../components/search-filters';

const RESULTS_PER_LOAD = 25;
const TABS = ['content', 'people', 'spaces'] as const;
const DATE_PRESETS: Record<string, number> = { day: 1, week: 7, month: 31, year: 366 };
const MILLISECONDS_PER_DAY = 86_400_000;

/** Snippets are plain text with markers around matches, never HTML. */
export function highlight(snippet: string): { text: string; hit: boolean }[] {
  return snippet
    .split(/(\[\[\[.*?\]\]\])/)
    .filter((s) => s !== '')
    .map((s) => (s.startsWith('[[[') ? { text: s.slice(3, -3), hit: true } : { text: s, hit: false }));
}

/** The filters of the address, as the server wants them. */
export function searchParamsFor(address: URLSearchParams, now = Date.now()): URLSearchParams {
  const params = new URLSearchParams();
  for (const key of ['q', 'space_id', 'kind', 'contributor', 'stage', 'below', 'sort']) {
    for (const value of address.getAll(key)) {
      if (value) params.append(key, value);
    }
  }
  const labels = address.getAll('label').flatMap((value) => value.split(','));
  for (const label of labels.map((l) => l.trim()).filter(Boolean)) params.append('label', label);
  if (address.get('titles') === '1') params.set('titles', '1');
  if (address.get('archived') === '1') params.set('archived', '1');
  const preset = DATE_PRESETS[address.get('within') ?? ''];
  const since = address.get('since');
  if (preset) params.set('since', new Date(now - preset * MILLISECONDS_PER_DAY).toISOString());
  else if (since) params.set('since', new Date(since).toISOString());
  const until = address.get('until');
  if (until) params.set('until', new Date(until).toISOString());
  return params;
}

function ResultRow({ result }: { result: SearchResult }) {
  const { t } = useTranslation();
  return (
    <li>
      <Link href={`/pages/${result.page_id}${result.comment_id ? `#comment-${result.comment_id}` : ''}`}>
        {result.title}
      </Link>{' '}
      <span className="muted">{result.space_name}</span>{' '}
      <StageMark text={result.stage_text ?? ''} color={result.stage_color ?? ''} />
      {result.archived && <span className="lozenge">{t('search.archived')}</span>}
      {(result.breadcrumbs?.length ?? 0) > 0 && (
        <nav aria-label={t('page.breadcrumbs')} className="result-crumbs">
          {result.breadcrumbs?.map((c) => (
            <Link key={c.id} href={`/pages/${c.id}`}>
              {c.title}
            </Link>
          ))}
        </nav>
      )}
      <p>
        {highlight(result.snippet).map((part, i) =>
          part.hit ? <mark key={i}>{part.text}</mark> : <span key={i}>{part.text}</span>,
        )}
      </p>
    </li>
  );
}

/** Spaces are chosen with space: words in the search box and its panel, not here. */
function Filters({
  address,
  onSet,
}: {
  address: URLSearchParams;
  onSet: (key: string, value: string) => void;
}) {
  const { t } = useTranslation();
  const flag = (key: string, label: string) => (
    <label className="check">
      <input
        type="checkbox"
        checked={address.get(key) === '1'}
        onChange={(e) => onSet(key, e.target.checked ? '1' : '')}
      />
      {label}
    </label>
  );
  return (
    <form
      className="filters search-filters"
      aria-label={t('search.filters')}
      onSubmit={(e) => e.preventDefault()}
    >
      <label className="field">
        <span>{t('search.kind')}</span>
        <select value={address.get('kind') ?? ''} onChange={(e) => onSet('kind', e.target.value)}>
          <option value="">{t('search.kind_any')}</option>
          <option value="page">{t('search.kind_page')}</option>
          <option value="post">{t('search.kind_post')}</option>
          <option value="database">{t('search.kind_database')}</option>
          <option value="comment">{t('search.kind_comment')}</option>
        </select>
      </label>
      <label className="field">
        <span>{t('organise.labels')}</span>
        <input value={address.get('label') ?? ''} onChange={(e) => onSet('label', e.target.value)} />
      </label>
      <label className="field">
        <span>{t('organise.status')}</span>
        <input value={address.get('stage') ?? ''} onChange={(e) => onSet('stage', e.target.value)} />
      </label>
      <label className="field">
        <span>{t('search.within')}</span>
        <select value={address.get('within') ?? ''} onChange={(e) => onSet('within', e.target.value)}>
          <option value="">{t('search.within_any')}</option>
          {Object.keys(DATE_PRESETS).map((name) => (
            <option key={name} value={name}>
              {t(`search.within_${name}`)}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t('search.since')}</span>
        <input
          type="date"
          value={address.get('since') ?? ''}
          onChange={(e) => onSet('since', e.target.value)}
        />
      </label>
      <label className="field">
        <span>{t('search.until')}</span>
        <input
          type="date"
          value={address.get('until') ?? ''}
          onChange={(e) => onSet('until', e.target.value)}
        />
      </label>
      <label className="field">
        <span>{t('directory.sort')}</span>
        <select value={address.get('sort') ?? ''} onChange={(e) => onSet('sort', e.target.value)}>
          <option value="">{t('search.sort_relevance')}</option>
          <option value="date">{t('search.sort_date')}</option>
        </select>
      </label>
      <SubjectPicker
        label={t('search.contributor')}
        usersOnly
        onPick={(person) => onSet('contributor', person.id)}
      />
      {address.get('contributor') && (
        <button type="button" className="ghost" onClick={() => onSet('contributor', '')}>
          {t('search.clearContributor')}
        </button>
      )}
      {flag('titles', t('search.titlesOnly'))}
      {flag('archived', t('search.includeArchived'))}
    </form>
  );
}

/** With fewer page results than this, comments get a place of their own. */
const COMMENTS_BELOW = 25;
const COMMENTS_SHOWN = 5;

function ContentResults({ params }: { params: URLSearchParams }) {
  const { t } = useTranslation();
  const query = params.toString();
  const hasFilter = [...params.keys()].some((key) => key !== 'q');
  const q = useInfiniteQuery({
    queryKey: ['search', query],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.get<SearchAnswer>(`/search?${query}&limit=${RESULTS_PER_LOAD}&offset=${pageParam}`),
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((sum, p) => sum + p.results.length, 0);
      return last.results.length === RESULTS_PER_LOAD && (last.capped || loaded < last.total)
        ? loaded
        : undefined;
    },
    enabled: params.get('q')?.trim() !== undefined || hasFilter,
  });
  const first = q.data?.pages[0];
  const results = q.data?.pages.flatMap((p) => p.results) ?? [];
  return (
    <>
      {q.isFetching && <Loading />}
      <ErrorNote error={q.error} />
      {first?.timed_out && <p role="alert">{t('search.timedOut')}</p>}
      {first && !first.timed_out && (
        <p role="status" className="muted">
          {first.capped
            ? t('search.totalCapped', { count: first.total })
            : t('search.total', { count: first.total })}
        </p>
      )}
      {first?.suggestion && (
        <p>
          {t('search.didYouMean')}{' '}
          <Link href={`/search?q=${encodeURIComponent(first.suggestion)}`}>{first.suggestion}</Link>
        </p>
      )}
      {first && !first.timed_out && results.length === 0 && (
        <p>{hasFilter ? t('search.noResultsFiltered') : t('search.noResults')}</p>
      )}
      <ol className="results">
        {results.map((r) => (
          <ResultRow key={r.comment_id ?? r.page_id} result={r} />
        ))}
      </ol>
      {q.hasNextPage && (
        <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t('common.more')}
        </button>
      )}
      {first && !first.capped && first.total < COMMENTS_BELOW && <CommentResults params={params} />}
    </>
  );
}

/** A word that lives in a comment must not end in "nothing found". */
function CommentResults({ params }: { params: URLSearchParams }) {
  const { t } = useTranslation();
  const text = params.get('q')?.trim() ?? '';
  const searched = params.has('kind') || text === '';
  const q = useQuery({
    queryKey: ['search-comments', params.toString()],
    queryFn: () => {
      const query = new URLSearchParams(params);
      query.set('kind', 'comment');
      query.set('limit', String(COMMENTS_SHOWN));
      return api.get<SearchAnswer>(`/search?${query.toString()}`);
    },
    enabled: !searched,
  });
  const found = q.data?.results ?? [];
  if (found.length === 0) return <ErrorNote error={q.error} />;
  return (
    <section>
      <h2>{t('search.inComments')}</h2>
      <ol className="results">
        {found.map((r) => (
          <ResultRow key={r.comment_id ?? r.page_id} result={r} />
        ))}
      </ol>
    </section>
  );
}

function PeopleResults({ text }: { text: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['directory', text],
    queryFn: () => api.get<{ entries: DirectoryEntry[] }>(`/directory?q=${encodeURIComponent(text)}`),
    enabled: text !== '',
  });
  const people = q.data?.entries.filter((e) => e.type === 'user') ?? [];
  return (
    <>
      <ErrorNote error={q.error} />
      {q.data && people.length === 0 && <p>{t('search.noResults')}</p>}
      <ul className="page-list">
        {people.map((p) => (
          <li key={p.id}>
            {p.name} <span className="muted">{p.detail}</span>
          </li>
        ))}
      </ul>
    </>
  );
}

function SpaceResults({ text, spaces }: { text: string; spaces: Space[] }) {
  const { t } = useTranslation();
  const wanted = text.toLowerCase();
  const found = spaces.filter((s) => `${s.name} ${s.key}`.toLowerCase().includes(wanted));
  return (
    <>
      {found.length === 0 && <p>{t('search.noResults')}</p>}
      <ul className="page-list">
        {found.map((s) => (
          <li key={s.id}>
            <Link href={`/spaces/${s.key}`}>{s.name}</Link> <span className="muted">{s.key}</span>
          </li>
        ))}
      </ul>
    </>
  );
}

/** The filter words in the query, as chips that take a word out again (spec 033). */
function QueryWords({
  text,
  spaces,
  onChange,
}: {
  text: string;
  spaces: Space[];
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  const words = splitQuery(text);
  const shown = [
    ...words.spaces.map((key) => ({
      kind: 'spaces' as const,
      value: key,
      label: t('search.wordSpace', { name: spaces.find((s) => s.key.toUpperCase() === key)?.name ?? key }),
    })),
    ...words.labels.map((value) => ({
      kind: 'labels' as const,
      value,
      label: t('search.wordLabel', { label: value }),
    })),
    ...words.people.map((value) => ({
      kind: 'people' as const,
      value,
      label: t('search.wordPerson', { name: value }),
    })),
  ];
  if (shown.length === 0) return null;
  return (
    <ul className="chip-list" aria-label={t('search.filters')}>
      {shown.map((w) => (
        <li key={w.kind + w.value} className="chip">
          {w.label}
          <button
            type="button"
            className="ghost"
            aria-label={t('search.removeWord', { word: w.label })}
            onClick={() =>
              onChange(
                queryWith(words.text, {
                  ...NO_FILTERS,
                  spaces: w.kind === 'spaces' ? words.spaces.filter((v) => v !== w.value) : words.spaces,
                  labels: w.kind === 'labels' ? words.labels.filter((v) => v !== w.value) : words.labels,
                  people: w.kind === 'people' ? words.people.filter((v) => v !== w.value) : words.people,
                }),
              )
            }
          >
            {t('common.remove')}
          </button>
        </li>
      ))}
    </ul>
  );
}

export function SearchView() {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const text = searchParams.get('q') ?? '';
  const tab = TABS.find((name) => name === searchParams.get('tab')) ?? 'content';
  const [input, setInput] = useState(text);
  useEffect(() => setInput(text), [text]);

  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  const list = spaces.data?.spaces ?? [];
  const params = searchParamsFor(searchParams);

  const set = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    navigate(`/search?${next.toString()}`, true);
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    set('q', input);
  };
  const searching = [...params.keys()].length > 0;

  return (
    <>
      <h1>{t('search.title')}</h1>
      <form className="row" onSubmit={submit}>
        <input
          type="search"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          aria-label={t('search.title')}
          className="grow"
        />
        <button type="submit" className="primary">
          {t('common.search')}
        </button>
      </form>
      <div role="tablist" aria-label={t('search.title')} className="tabs">
        {TABS.map((name) => (
          <button
            key={name}
            type="button"
            role="tab"
            aria-selected={tab === name}
            onClick={() => set('tab', name === 'content' ? '' : name)}
          >
            {t(`search.tab_${name}`)}
          </button>
        ))}
      </div>
      <QueryWords text={text} spaces={list} onChange={(next) => set('q', next)} />
      {tab === 'content' && <Filters address={searchParams} onSet={set} />}
      {!searching && <p className="muted">{t('search.hint')}</p>}
      {tab === 'content' && searching && <ContentResults params={params} />}
      {tab === 'people' && searching && <PeopleResults text={text} />}
      {tab === 'spaces' && searching && <SpaceResults text={text} spaces={list} />}
    </>
  );
}
