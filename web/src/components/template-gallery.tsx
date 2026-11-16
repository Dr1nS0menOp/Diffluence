// SPDX-License-Identifier: Apache-2.0
// Starting a page from a template: a list with a preview, then the form.
import { useCallback, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Space, type TemplateSummary } from '../api';
import { useCapabilities } from '../capabilities';
import { DocumentView } from './editor/editor';
import { CloseFormDialog, startPage, TemplateForm } from './template-form';
import { Dialog, ErrorNote, Loading } from './ui';

export { stepsOf } from './template-form';

/** A template is not a page, so its files have a route of their own. */
export const templateFileAddress = (templateId: string) => (fileId: string) =>
  `/api/v1/templates/${templateId}/files/${fileId}`;

/** A template asks first when it has questions, or makes more than one page to check (TPL-340). */
const asksFirst = (template: TemplateSummary) =>
  (template.fields?.length ?? 0) > 0 || (template.pages ?? 1) > 1;

/** Past this many templates the gallery searches on the server (TPL-701). */
const SERVER_SEARCH_ABOVE = 100;

function FavouriteButton({ template, spaceId }: { template: TemplateSummary; spaceId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const path = `/templates/${template.id}/favourite`;
  const flip = useMutation({
    mutationFn: () => (template.favourite ? api.del(path) : api.put(path)),
    onSettled: () => client.invalidateQueries({ queryKey: ['gallery', spaceId] }),
  });
  return (
    <>
      <button type="button" className="ghost" aria-pressed={template.favourite} onClick={() => flip.mutate()}>
        {t('organise.favouriteTemplate')}
      </button>
      <ErrorNote error={flip.error} />
    </>
  );
}

/** The form of one template in its dialog, which asks before it closes with answers in it. */
function FormDialog({
  template,
  space,
  parent,
  replaces,
  onBack,
  onClose,
}: {
  template: TemplateSummary;
  space: Space;
  parent: string | null;
  replaces?: string;
  onBack: () => void;
  onClose: () => void;
}) {
  const guidedOn = useCapabilities().guided_templates;
  const [dirty, setDirty] = useState(false);
  const [asking, setAsking] = useState<'close' | 'back' | null>(null);
  const markDirty = useCallback(() => setDirty(true), []);
  const leave = (how: 'close' | 'back') => (dirty ? setAsking(how) : how === 'close' ? onClose() : onBack());
  // The question stands beside the form's dialog, not inside it: React passes the close of a
  // dialog on to the dialogs around it, which would close the form as well.
  return (
    <>
      <Dialog title={template.name} onClose={() => leave('close')} onEscape={() => leave('close')}>
        <TemplateForm
          template={template}
          space={space}
          parent={parent}
          replaces={replaces}
          guidedOn={guidedOn}
          onBack={() => leave('back')}
          onClose={onClose}
          onDirty={markDirty}
        />
      </Dialog>
      {asking && (
        <CloseFormDialog onConfirm={asking === 'close' ? onClose : onBack} onClose={() => setAsking(null)} />
      )}
    </>
  );
}

export function TemplateGallery({
  space,
  parent,
  replaces,
  onClose,
  initial,
}: {
  space: Space;
  parent: string | null;
  /** An empty page to remove once the new one exists. */
  replaces?: string;
  onClose: () => void;
  /** A template to mark when the gallery opens, such as one chosen in the panel beside the editor. */
  initial?: string;
}) {
  const { t } = useTranslation();
  const guidedOn = useCapabilities().guided_templates;
  const [filter, setFilter] = useState('');
  const [marked, setMarked] = useState<number | null>(null);
  const [chosen, setChosen] = useState<TemplateSummary | null>(null);
  const [queued, setQueued] = useState(false);
  const list = useQuery({
    queryKey: ['gallery', space.id],
    queryFn: () => api.get<{ templates: TemplateSummary[] }>(`/spaces/${space.id}/templates`),
  });
  const wanted = filter.trim().toLowerCase();
  // A space that offers more than the browser lists well is searched on the server, which also
  // knows the names of the pages each template makes (spec 074, TPL-701).
  const onServer = guidedOn && (list.data?.templates.length ?? 0) > SERVER_SEARCH_ABOVE && wanted !== '';
  const found = useQuery({
    queryKey: ['gallery-search', space.id, wanted],
    queryFn: () =>
      api.get<{ templates: TemplateSummary[] }>(
        `/spaces/${space.id}/templates?q=${encodeURIComponent(wanted)}`,
      ),
    enabled: onServer,
  });
  const rows = onServer
    ? (found.data?.templates ?? [])
    : (list.data?.templates ?? []).filter((x) =>
        `${x.name} ${x.description} ${x.category}`.toLowerCase().includes(wanted),
      );
  const markedIndex =
    marked ??
    Math.max(
      0,
      rows.findIndex((x) => x.id === initial),
    );
  const row = rows[markedIndex];
  const preview = useQuery({
    queryKey: ['template', row?.id],
    queryFn: () => api.get<TemplateSummary>(`/templates/${row?.id}`),
    enabled: !!row,
  });
  const start = useMutation({
    mutationFn: async () => {
      const full = preview.data;
      if (!full) return;
      if (asksFirst(full)) {
        setChosen(full);
        return;
      }
      const outcome = await startPage(
        full,
        space,
        { parent_id: parent, answers: {}, replaces: replaces ?? null },
        crypto.randomUUID(),
      );
      if (outcome.kind === 'made') onClose();
      else setQueued(true);
    },
  });
  const keys = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (rows.length > 0) {
        setMarked((markedIndex + (e.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length);
      }
    } else if (e.key === 'Enter') {
      e.preventDefault();
      start.mutate();
    }
  };
  if (chosen) {
    return (
      <FormDialog
        template={chosen}
        space={space}
        parent={parent}
        replaces={replaces}
        onBack={() => setChosen(null)}
        onClose={onClose}
      />
    );
  }
  const pages = preview.data?.pages ?? 1;
  return (
    <Dialog title={t('organise.gallery')} onClose={onClose} wide>
      <ErrorNote error={list.error ?? found.error ?? start.error} />
      {queued && (
        <p role="status" className="note">
          {t('templates.setQueued')}
        </p>
      )}
      <div className="gallery">
        <div>
          <input
            type="search"
            className="dialog-filter"
            value={filter}
            autoFocus
            role="combobox"
            aria-expanded="true"
            aria-controls="template-list"
            aria-activedescendant={row ? `template-${row.id}` : undefined}
            aria-label={t('organise.findTemplate')}
            placeholder={t('organise.findTemplate')}
            onChange={(e) => {
              setFilter(e.target.value);
              setMarked(0);
            }}
            onKeyDown={keys}
          />
          {list.isPending && <Loading />}
          <ul id="template-list" role="listbox" aria-label={t('organise.gallery')} className="gallery-list">
            {rows.map((x, index) => (
              <li
                key={x.id}
                id={`template-${x.id}`}
                role="option"
                aria-selected={index === markedIndex}
                className={index === markedIndex ? 'quick-row marked' : 'quick-row'}
                // Only a mouse that moves marks a row: one that rests where the dialog opened does not.
                onMouseMove={() => setMarked(index)}
                onClick={() => start.mutate()}
              >
                <span className="quick-title">
                  {x.name}
                  {/* Only its owner is offered a draft, to try it (spec 074, TPL-510). */}
                  {x.template_version === 0 && x.state && (
                    <>
                      {' '}
                      <span className="lozenge">{t(`templates.state_${x.state}`)}</span>
                    </>
                  )}
                </span>
                <span className="quick-detail">{x.category}</span>
              </li>
            ))}
          </ul>
          {list.data && rows.length === 0 && <p className="muted">{t('organise.noTemplates')}</p>}
        </div>
        <div className="gallery-preview" aria-label={t('organise.preview')}>
          {row && <p className="muted">{row.description}</p>}
          {preview.data && pages > 1 && <p>{t('templates.makesPages', { count: pages })}</p>}
          {row && <FavouriteButton template={row} spaceId={space.id} />}
          {preview.data?.document && (
            <DocumentView
              document={preview.data.document}
              plain
              fileAddress={templateFileAddress(preview.data.id)}
            />
          )}
        </div>
      </div>
      <div className="dialog-actions">
        <button type="button" className="primary" disabled={!row} onClick={() => start.mutate()}>
          {t('organise.useTemplate')}
        </button>
      </div>
    </Dialog>
  );
}
