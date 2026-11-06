// SPDX-License-Identifier: Apache-2.0
// Starting a page from a template: a list with a preview, then the form.
import { useState, type FormEvent, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Page, type Space, type TemplateField, type TemplateSummary } from '../api';
import { navigate } from '../router';
import { DocumentView } from './editor/editor';
import { Dialog, ErrorNote, Field, Loading } from './ui';

function FieldInput({
  field,
  value,
  onChange,
}: {
  field: TemplateField;
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  const help = field.required ? t('organise.required') : undefined;
  if (field.kind === 'text') {
    return (
      <Field label={field.name} help={help}>
        <textarea
          rows={3}
          value={value}
          required={field.required}
          onChange={(e) => onChange(e.target.value)}
        />
      </Field>
    );
  }
  if (field.kind === 'choice') {
    return (
      <Field label={field.name} help={help}>
        <select value={value} required={field.required} onChange={(e) => onChange(e.target.value)}>
          <option value="">{t('organise.chooseOne')}</option>
          {field.options?.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
      </Field>
    );
  }
  return (
    <Field label={field.name} help={help}>
      <input value={value} required={field.required} onChange={(e) => onChange(e.target.value)} />
    </Field>
  );
}

/** Groups fields in the steps of a template. Fields outside any step go last. */
export function stepsOf(template: TemplateSummary): { title: string; fields: TemplateField[] }[] {
  const fields = template.fields ?? [];
  const placed = new Set(template.steps.flatMap((s) => s.fields));
  const steps = template.steps.map((s) => ({
    title: s.title,
    fields: fields.filter((f) => s.fields.includes(f.name)),
  }));
  const rest = fields.filter((f) => !placed.has(f.name));
  return rest.length > 0 || steps.length === 0 ? [...steps, { title: '', fields: rest }] : steps;
}

/** A template is not a page, so its files have a route of their own. */
export const templateFileAddress = (templateId: string) => (fileId: string) =>
  `/api/v1/templates/${templateId}/files/${fileId}`;

/** Makes the page. An empty page that was waiting for a template makes way for it. */
async function startPage(
  template: TemplateSummary,
  space: Space,
  parent: string | null,
  answers: Record<string, string>,
  replaces?: string,
) {
  const made = await api.post<{ page: Page }>(`/templates/${template.id}/use`, {
    space_id: space.id,
    parent_id: parent,
    answers,
  });
  if (replaces) await api.del(`/pages/${replaces}`);
  navigate(`/pages/${made.page.id}/edit`);
}

function TemplateForm({
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
  const { t } = useTranslation();
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [step, setStep] = useState(0);
  const steps = stepsOf(template);
  const current = steps[step];
  const last = step === steps.length - 1;
  const use = useMutation({
    mutationFn: () => startPage(template, space, parent, answers, replaces),
    onSuccess: onClose,
  });
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        if (last) use.mutate();
        else setStep(step + 1);
      }}
    >
      <ErrorNote error={use.error} />
      {current?.title && <h3>{current.title}</h3>}
      {steps.length > 1 && (
        <p role="status" className="muted">
          {t('organise.stepOf', { at: step + 1, total: steps.length })}
        </p>
      )}
      {current?.fields.map((f) => (
        <FieldInput
          key={f.name}
          field={f}
          value={answers[f.name] ?? ''}
          onChange={(value) => setAnswers({ ...answers, [f.name]: value })}
        />
      ))}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={step === 0 ? onBack : () => setStep(step - 1)}>
          {t('organise.back')}
        </button>
        <button type="submit" className="primary" disabled={use.isPending}>
          {last ? t('organise.startPage') : t('organise.next')}
        </button>
      </div>
    </form>
  );
}

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

export function TemplateGallery({
  space,
  parent,
  replaces,
  onClose,
}: {
  space: Space;
  parent: string | null;
  /** An empty page to remove once the new one exists. */
  replaces?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const [marked, setMarked] = useState(0);
  const [chosen, setChosen] = useState<TemplateSummary | null>(null);
  const list = useQuery({
    queryKey: ['gallery', space.id],
    queryFn: () => api.get<{ templates: TemplateSummary[] }>(`/spaces/${space.id}/templates`),
  });
  const wanted = filter.trim().toLowerCase();
  const rows = (list.data?.templates ?? []).filter((x) =>
    `${x.name} ${x.description} ${x.category}`.toLowerCase().includes(wanted),
  );
  const row = rows[marked];
  const preview = useQuery({
    queryKey: ['template', row?.id],
    queryFn: () => api.get<TemplateSummary>(`/templates/${row?.id}`),
    enabled: !!row,
  });
  const start = useMutation({
    mutationFn: async () => {
      const full = preview.data;
      if (!full) return;
      if ((full.fields?.length ?? 0) > 0) {
        setChosen(full);
        return;
      }
      await startPage(full, space, parent, {}, replaces);
      onClose();
    },
  });
  const keys = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (rows.length > 0) {
        setMarked((marked + (e.key === 'ArrowDown' ? 1 : rows.length - 1)) % rows.length);
      }
    } else if (e.key === 'Enter') {
      e.preventDefault();
      start.mutate();
    }
  };
  if (chosen) {
    return (
      <Dialog title={chosen.name} onClose={onClose}>
        <TemplateForm
          template={chosen}
          space={space}
          parent={parent}
          replaces={replaces}
          onBack={() => setChosen(null)}
          onClose={onClose}
        />
      </Dialog>
    );
  }
  return (
    <Dialog title={t('organise.gallery')} onClose={onClose} wide>
      <ErrorNote error={list.error ?? start.error} />
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
                aria-selected={index === marked}
                className={index === marked ? 'quick-row marked' : 'quick-row'}
                onMouseEnter={() => setMarked(index)}
                onClick={() => start.mutate()}
              >
                <span className="quick-title">{x.name}</span>
                <span className="quick-detail">{x.category}</span>
              </li>
            ))}
          </ul>
          {list.data && rows.length === 0 && <p className="muted">{t('organise.noTemplates')}</p>}
        </div>
        <div className="gallery-preview" aria-label={t('organise.preview')}>
          {row && <p className="muted">{row.description}</p>}
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
