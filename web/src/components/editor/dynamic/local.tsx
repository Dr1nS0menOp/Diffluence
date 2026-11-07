// SPDX-License-Identifier: Apache-2.0
// Blocks that answer in the browser, from the document or from their own settings.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api, type Json } from '../../../api';
import { navigate } from '../../../router';
import { TimelineChart } from '../../chart';
import { ErrorNote } from '../../ui';
import { contentsOf, jumpToHeading, type Heading } from '../outline';
import { matchesPattern, sectionOf, timelineOf } from './kinds';

type Settings = Record<string, unknown>;

function HeadingList({ headings, numbered }: { headings: Heading[]; numbered: boolean }) {
  const { t } = useTranslation();
  if (headings.length === 0) return <p className="muted">{t('blocks.noHeadings')}</p>;
  const List = numbered ? 'ol' : 'ul';
  return (
    <List className="block-list">
      {headings.map((h) => (
        <li key={h.slug} data-level={h.level}>
          <a
            href={`#${h.slug}`}
            onClick={(e) => {
              e.preventDefault();
              jumpToHeading(h.slug, true);
            }}
          >
            {h.text}
          </a>
        </li>
      ))}
    </List>
  );
}

function levelsOf(settings: Settings) {
  return {
    from: typeof settings.from === 'number' ? settings.from : 1,
    to: typeof settings.to === 'number' ? settings.to : 3,
  };
}

function Contents({ document: content, settings }: { document: Json; settings: Settings }) {
  const { from, to } = levelsOf(settings);
  const exclude = typeof settings.exclude === 'string' ? settings.exclude : '';
  const headings = contentsOf(content).filter(
    (h) => h.level >= from && h.level <= to && !(exclude && matchesPattern(h.text, exclude)),
  );
  return <HeadingList headings={headings} numbered={settings.numbered === true} />;
}

/** The same, for one part of the page. The slugs stay those of the whole page. */
function SectionContents({ document: content, settings }: { document: Json; settings: Settings }) {
  const { from, to } = levelsOf(settings);
  const inside = new Set(contentsOf(sectionOf(content, settings)).map((h) => h.text));
  const headings = contentsOf(content).filter((h) => inside.has(h.text) && h.level >= from && h.level <= to);
  return <HeadingList headings={headings} numbered={false} />;
}

function SearchBox({ settings }: { settings: Settings }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const pageId = usePageIdFromAddress();
  const page = useQuery({
    queryKey: ['block-search-page', pageId],
    queryFn: () => api.get<{ space_id: string }>(`/pages/${pageId}`),
    enabled: settings.this_space === true && !!pageId,
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const address = new URLSearchParams({ q: query });
    if (page.data) address.set('space_id', page.data.space_id);
    if (typeof settings.below === 'string') address.set('below', settings.below);
    if (typeof settings.content_type === 'string') address.set('kind', settings.content_type);
    if (Array.isArray(settings.labels)) {
      for (const label of settings.labels as string[]) address.append('label', label);
    }
    navigate(`/search?${address.toString()}`);
  };
  const hint = typeof settings.placeholder === 'string' ? settings.placeholder : t('blocks.searchHint');
  return (
    <form role="search" onSubmit={submit}>
      <input
        type="search"
        aria-label={hint}
        placeholder={hint}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
    </form>
  );
}

const PAGE_IN_ADDRESS = /^\/pages\/([0-9a-f-]{36})/i;

function usePageIdFromAddress(): string | undefined {
  return PAGE_IN_ADDRESS.exec(window.location.pathname)?.[1];
}

/** The server decides whether this person may start a page here (spec 026, D9). */
function CreateFromTemplate({ settings }: { settings: Settings }) {
  const { t } = useTranslation();
  const pageId = usePageIdFromAddress();
  const template = typeof settings.template === 'string' ? settings.template : '';
  const parent = typeof settings.parent === 'string' ? settings.parent : pageId;
  const start = useMutation({
    mutationFn: async () => {
      const here = await api.get<{ space_id: string }>(`/pages/${parent ?? pageId}`);
      return api.post<{ page: { id: string } }>(`/templates/${template}/use`, {
        space_id: here.space_id,
        parent_id: parent ?? null,
      });
    },
    onSuccess: (made) => navigate(`/pages/${made.page.id}/edit`),
  });
  if (!template) return <p className="muted">{t('blocks.chooseTemplate')}</p>;
  const label = typeof settings.label === 'string' ? settings.label : t('blocks.createFromTemplate');
  return (
    <>
      <ErrorNote error={start.error} />
      <button type="button" disabled={start.isPending} onClick={() => start.mutate()}>
        {label}
      </button>
    </>
  );
}

function Timeline({ settings }: { settings: Settings }) {
  const { t } = useTranslation();
  const items = timelineOf(settings.items);
  if (items.length === 0) return <p className="muted">{t('blocks.noTimeline')}</p>;
  return <TimelineChart items={items} />;
}

export function Local({ kind, settings, content }: { kind: string; settings: Settings; content: Json }) {
  const { t } = useTranslation();
  switch (kind) {
    case 'toc':
      return <Contents document={content} settings={settings} />;
    case 'section_contents':
      return <SectionContents document={content} settings={settings} />;
    case 'search_box':
      return <SearchBox settings={settings} />;
    case 'create_from_template':
      return <CreateFromTemplate settings={settings} />;
    case 'timeline':
      return <Timeline settings={settings} />;
  }
  return (
    <a
      href="#main"
      onClick={(e) => {
        e.preventDefault();
        window.scrollTo(0, 0);
      }}
    >
      {t('blocks.backToTop')}
    </a>
  );
}
