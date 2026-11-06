// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Page, type Space, type TemplateSummary } from '../api';
import { navigate } from '../router';
import { AnimalChoice } from './animal-choice';
import { TemplateGallery } from './template-gallery';
import { Dialog, ErrorNote, Field } from './ui';

const SUGGESTED_KEY_LENGTH = 12;
const VALID_KEY = /^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$/;

/** A key from a name: its letters and digits, in capitals. */
export function suggestKey(name: string): string {
  const letters = name.normalize('NFKD').replace(/[^A-Za-z0-9]/g, '');
  return letters.slice(0, SUGGESTED_KEY_LENGTH).toUpperCase();
}

/** Where the Create button in the top bar puts a new page. */
export interface CreateTarget {
  space: Space;
  parent: string | null;
}

export function NewPageDialog({
  space,
  parent,
  onClose,
}: {
  space: Space;
  parent: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [title, setTitle] = useState('');
  const [gallery, setGallery] = useState(false);
  const create = useMutation({
    mutationFn: (untitled: boolean) =>
      api.post<Page>('/pages', { space_id: space.id, parent_id: parent, title, untitled }),
    onSuccess: (p) => {
      onClose();
      navigate(`/pages/${p.id}/edit`);
    },
  });
  if (gallery) return <TemplateGallery space={space} parent={parent} onClose={onClose} />;
  return (
    <Dialog title={t('space.newPage')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate(false);
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('page.title')}>
          <input value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
        </Field>
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={() => setGallery(true)}>
            {t('organise.fromTemplate')}
          </button>
          <button type="button" className="quiet" onClick={() => create.mutate(true)}>
            {t('organise.noTitleYet')}
          </button>
          <button type="submit" className="primary" disabled={create.isPending}>
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export function NewPostDialog({ space, onClose }: { space: Space; onClose: () => void }) {
  const { t } = useTranslation();
  const [title, setTitle] = useState('');
  const create = useMutation({
    mutationFn: () => api.post<Page>('/pages', { space_id: space.id, title, kind: 'post' }),
    onSuccess: (p) => {
      onClose();
      navigate(`/pages/${p.id}/edit`);
    },
  });
  return (
    <Dialog title={t('blog.newPost')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('page.title')}>
          <input value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={create.isPending}>
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** A tree entry that opens an address outside this space. */
export function NewLinkDialog({
  space,
  parent,
  onClose,
}: {
  space: Space;
  parent: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [title, setTitle] = useState('');
  const [link, setLink] = useState('');
  const create = useMutation({
    mutationFn: () =>
      api.post<Page>('/pages', { space_id: space.id, parent_id: parent, title, kind: 'link', link }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['tree', space.id] });
      onClose();
    },
  });
  return (
    <Dialog title={t('tree.addLink')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('page.title')}>
          <input value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
        </Field>
        <Field label={t('tree.linkAddress')}>
          <input type="url" value={link} onChange={(e) => setLink(e.target.value)} required />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={create.isPending}>
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export function NewSpaceDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState({ key: '', name: '', description: '', icon: '' });
  const [keyTouched, setKeyTouched] = useState(false);
  const [templateId, setTemplateId] = useState('');
  const [preset, setPreset] = useState('');
  const starts = useQuery({
    queryKey: ['space-templates'],
    queryFn: () =>
      api.get<{ templates: Pick<TemplateSummary, 'id' | 'name' | 'description'>[]; default_preset: string }>(
        '/space-templates',
      ),
  });
  const chosenPreset = preset || starts.data?.default_preset || 'private';
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) =>
    setForm({ ...form, [k]: e.target.value });
  const free = useQuery({
    queryKey: ['key-check', form.key],
    queryFn: () => api.get<{ free: boolean }>(`/space-key-check?key=${encodeURIComponent(form.key)}`),
    enabled: VALID_KEY.test(form.key),
  });
  const create = useMutation({
    mutationFn: () =>
      api.post<Space>('/spaces', {
        ...form,
        preset: chosenPreset,
        ...(templateId ? { template_id: templateId } : {}),
      }),
    onSuccess: async (s) => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      onClose();
      navigate(`/spaces/${s.key}`);
    },
  });
  return (
    <Dialog title={t('home.newSpace')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('common.name')}>
          <input
            value={form.name}
            onChange={(e) =>
              setForm({
                ...form,
                name: e.target.value,
                key: keyTouched ? form.key : suggestKey(e.target.value),
              })
            }
            required
            autoFocus
          />
        </Field>
        <Field label={t('space.key')} help={t('space.keyHelp')}>
          <input
            value={form.key}
            onChange={(e) => {
              setKeyTouched(true);
              set('key')(e);
            }}
            required
            pattern="[A-Za-z0-9][A-Za-z0-9_\-]{1,31}"
          />
        </Field>
        {free.data?.free === false && (
          <p role="alert" className="note note-error">
            {t('space.keyTaken')}
          </p>
        )}
        <Field label={t('space.description')}>
          <textarea value={form.description} onChange={set('description')} rows={2} />
        </Field>
        <AnimalChoice
          spaceKey={form.key}
          name={form.name}
          value={form.icon}
          onChange={(icon) => setForm({ ...form, icon })}
        />
        <Field label={t('spaceSettings.startFrom')}>
          <select value={templateId} onChange={(e) => setTemplateId(e.target.value)}>
            <option value="">{t('spaceSettings.startEmpty')}</option>
            {starts.data?.templates.map((x) => (
              <option key={x.id} value={x.id}>
                {x.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('spaceSettings.whoGetsAccess')} help={t(`spaceSettings.access_${chosenPreset}`)}>
          <select value={chosenPreset} onChange={(e) => setPreset(e.target.value)}>
            <option value="private">{t('spaceSettings.preset_private')}</option>
            <option value="team_space">{t('permissions.team_space')}</option>
            <option value="read_only_kb">{t('permissions.read_only_kb')}</option>
          </select>
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={create.isPending}>
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
