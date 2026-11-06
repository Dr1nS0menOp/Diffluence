// SPDX-License-Identifier: Apache-2.0
// Actions on a page that need a question first: archive, kinds, templates, sorting.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type DirectoryEntry, type PageDetail, type PageSummary } from '../api';
import { navigate } from '../router';
import { SubjectPicker } from '../components/picker';
import { useToast } from '../components/toast';
import { Dialog, ErrorNote, Field } from '../components/ui';

const SORTS = ['title', 'created', 'changed'];

function useTreeRefresh(page: PageSummary) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    await client.invalidateQueries({ queryKey: ['page', page.id] });
    await client.invalidateQueries({ queryKey: ['page-facts', page.id] });
  };
}

export function ArchiveDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const refresh = useTreeRefresh(page);
  const [children, setChildren] = useState(true);
  const archive = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/archive`, { include_children: children }),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  return (
    <Dialog title={t('organise.archivePage')} onClose={onClose}>
      <ErrorNote error={archive.error} />
      <p>{t('organise.archiveHelp')}</p>
      <label className="check">
        <input type="checkbox" checked={children} onChange={(e) => setChildren(e.target.checked)} />
        {t('organise.archiveChildren')}
      </label>
      <p className="muted">{t('organise.archiveLeftBehind')}</p>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" onClick={() => archive.mutate()}>
          {t('organise.archivePage')}
        </button>
      </div>
    </Dialog>
  );
}

/** Folder, page or post. Says what happens to the place in the tree first. */
export function KindDialog({
  page,
  kind,
  onClose,
}: {
  page: PageSummary;
  kind: 'page' | 'folder' | 'post';
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const refresh = useTreeRefresh(page);
  const change = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/kind`, { kind }),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  return (
    <Dialog title={t(`organise.turnInto_${kind}`)} onClose={onClose}>
      <ErrorNote error={change.error} />
      <p>{t(`organise.kindHelp_${kind}`)}</p>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" onClick={() => change.mutate()}>
          {t(`organise.turnInto_${kind}`)}
        </button>
      </div>
    </Dialog>
  );
}

export function SaveTemplateDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState(page.title);
  const save = useMutation({
    mutationFn: () => api.post<{ id: string }>(`/pages/${page.id}/save-as-template`, { name }),
    onSuccess: (x) => navigate(`/templates/${x.id}/edit`),
  });
  return (
    <Dialog title={t('organise.saveAsTemplate')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        <Field label={t('common.name')}>
          <input value={name} required autoFocus onChange={(e) => setName(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export function SortChildrenDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const refresh = useTreeRefresh(page);
  const [by, setBy] = useState('title');
  const sort = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/sort-children`, { by }),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  return (
    <Dialog title={t('organise.sortLevel')} onClose={onClose}>
      <ErrorNote error={sort.error} />
      <p>{t('organise.sortHelp')}</p>
      <Field label={t('directory.sort')}>
        <select value={by} onChange={(e) => setBy(e.target.value)}>
          {SORTS.map((s) => (
            <option key={s} value={s}>
              {t(`organise.sort_${s}`)}
            </option>
          ))}
        </select>
      </Field>
      <div className="dialog-actions">
        <button type="button" className="primary" onClick={() => sort.mutate()}>
          {t('organise.sortLevel')}
        </button>
      </div>
    </Dialog>
  );
}

/** Handing over gives the new owner no extra right (spec 011, D9). */
export function OwnerDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const hand = useMutation({
    mutationFn: (who: DirectoryEntry) => api.put(`/pages/${page.id}/owner`, { user_id: who.id }),
    onSuccess: async (_, who) => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      toast({ message: t('organise.ownerChanged', { name: who.name }) });
      onClose();
    },
  });
  return (
    <Dialog title={t('organise.changeOwner')} onClose={onClose}>
      <ErrorNote error={hand.error} />
      <SubjectPicker label={t('organise.newOwner')} usersOnly onPick={(who) => hand.mutate(who)} />
    </Dialog>
  );
}

export function ReviewDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [on, setOn] = useState(page.review_on);
  const save = useMutation({
    mutationFn: () => api.put(`/pages/${page.id}/review`, { on }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      onClose();
    },
  });
  return (
    <Dialog title={t('organise.reviewDate')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        <Field label={t('organise.reviewDate')}>
          <input type="date" value={on} autoFocus onChange={(e) => setOn(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
