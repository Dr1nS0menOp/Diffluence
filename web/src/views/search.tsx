// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type SearchResult, type Space } from '../api';
import { Link, navigate, useLocation } from '../router';
import { ErrorNote, Field, Loading } from '../components/ui';

/** Snippets are plain text with markers around matches, never HTML. */
export function highlight(snippet: string): { text: string; hit: boolean }[] {
  return snippet
    .split(/(\[\[\[.*?\]\]\])/)
    .filter((s) => s !== '')
    .map((s) => (s.startsWith('[[[') ? { text: s.slice(3, -3), hit: true } : { text: s, hit: false }));
}

export function SearchView() {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const text = searchParams.get('q') ?? '';
  const space = searchParams.get('space_id') ?? '';
  const since = searchParams.get('since') ?? '';
  const [input, setInput] = useState(text);
  useEffect(() => setInput(text), [text]);

  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  const params = new URLSearchParams({ q: text });
  if (space) params.set('space_id', space);
  if (since) params.set('since', new Date(since).toISOString());
  const q = useQuery({
    queryKey: ['search', params.toString()],
    queryFn: () => api.get<{ results: SearchResult[] }>('/search?' + params.toString()),
    enabled: text.trim() !== '',
  });

  const go = (next: { q?: string; space_id?: string; since?: string }) => {
    const p = new URLSearchParams({ q: next.q ?? input });
    const s = next.space_id ?? space;
    const d = next.since ?? since;
    if (s) p.set('space_id', s);
    if (d) p.set('since', d);
    navigate('/search?' + p.toString());
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    go({ q: input });
  };

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
        <select
          aria-label={t('search.anySpace')}
          value={space}
          onChange={(e) => go({ space_id: e.target.value })}
        >
          <option value="">{t('search.anySpace')}</option>
          {spaces.data?.spaces.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
        <Field label={t('search.since')}>
          <input type="date" value={since} onChange={(e) => go({ since: e.target.value })} />
        </Field>
        <button type="submit" className="primary">
          {t('common.search')}
        </button>
      </form>
      {text.trim() === '' && <p className="muted">{t('search.hint')}</p>}
      {q.isFetching && <Loading />}
      <ErrorNote error={q.error} />
      {q.data?.results.length === 0 && <p>{t('search.noResults')}</p>}
      <ol className="results">
        {q.data?.results.map((r) => (
          <li key={r.page_id}>
            <Link href={`/pages/${r.page_id}`}>{r.title}</Link> <span className="muted">{r.space_name}</span>
            <p>
              {highlight(r.snippet).map((part, i) =>
                part.hit ? <mark key={i}>{part.text}</mark> : <span key={i}>{part.text}</span>,
              )}
            </p>
          </li>
        ))}
      </ol>
    </>
  );
}
