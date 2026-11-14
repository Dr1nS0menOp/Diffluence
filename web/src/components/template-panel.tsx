// SPDX-License-Identifier: Apache-2.0
// An empty page offers templates beside the editor (ORG-045): a few to choose from, a box
// that finds one, and the whole gallery. It goes as soon as the person starts typing.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type PageDetail, type Space, type TemplateSummary } from '../api';
import { TemplateGallery } from './template-gallery';
import { ErrorNote } from './ui';

/** How many templates the panel lists before the person searches or opens the gallery. */
const SHOWN = 5;

export function TemplatePanel({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  // undefined: the gallery is shut; null: the whole gallery; an id: the gallery on that one.
  const [open, setOpen] = useState<string | null | undefined>(undefined);
  const [filter, setFilter] = useState('');
  const space = useQuery({
    queryKey: ['space-id', page.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.space_id}`),
  });
  const list = useQuery({
    queryKey: ['gallery', page.space_id],
    queryFn: () => api.get<{ templates: TemplateSummary[] }>(`/spaces/${page.space_id}/templates`),
  });
  const wanted = filter.trim().toLowerCase();
  const rows = (list.data?.templates ?? [])
    .filter((x) => `${x.name} ${x.category}`.toLowerCase().includes(wanted))
    .slice(0, SHOWN);
  return (
    <aside className="template-panel" aria-labelledby="template-panel-title">
      <p className="muted edit-hint">{t('editor.emptyPage')}</p>
      <h2 id="template-panel-title">{t('organise.panelTitle')}</h2>
      <input
        type="search"
        value={filter}
        aria-label={t('organise.findTemplate')}
        placeholder={t('organise.findTemplate')}
        onChange={(e) => setFilter(e.target.value)}
      />
      <ErrorNote error={list.error} />
      <ul className="plain-list template-panel-list">
        {rows.map((x) => (
          <li key={x.id}>
            <button type="button" className="ghost" onClick={() => setOpen(x.id)}>
              {x.name}
            </button>
          </li>
        ))}
      </ul>
      {list.data && rows.length === 0 && <p className="muted">{t('organise.noTemplates')}</p>}
      <button type="button" onClick={() => setOpen(null)}>
        {t('organise.fromTemplate')}
      </button>
      {open !== undefined && space.data && (
        <TemplateGallery
          space={space.data}
          parent={page.parent_id}
          replaces={page.id}
          initial={open ?? undefined}
          onClose={() => setOpen(undefined)}
        />
      )}
    </aside>
  );
}
