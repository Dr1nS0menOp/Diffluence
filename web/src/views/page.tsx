// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Attachment, type PageDetail, type Space } from '../api';
import { Link, navigate } from '../router';
import { DocumentView } from '../components/editor';
import {
  IconClock,
  IconCopy,
  IconDownload,
  IconLock,
  IconMore,
  IconMove,
  IconPencil,
  IconRename,
  IconStar,
  IconTrash,
} from '../components/icons';
import { Menu, MenuDownload, MenuItem, MenuLink, MenuSeparator } from '../components/menu';
import {
  Avatar,
  ConfirmDialog,
  ErrorNote,
  FilePicker,
  formatDay,
  formatSize,
  Loading,
} from '../components/ui';
import { CopyDialog, MoveDialog, RenameDialog, RestrictionsDialog } from './page-dialogs';

type PageDialog = 'restrictions' | 'move' | 'rename' | 'copy' | 'delete';

function Breadcrumbs({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const space = useQuery({
    queryKey: ['space-id', page.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.space_id}`),
  });
  return (
    <nav aria-label={t('page.breadcrumbs')} className="crumbs">
      <ol>
        {space.data && (
          <li>
            <Link href={`/spaces/${space.data.key}`}>{space.data.name}</Link>
          </li>
        )}
        {page.breadcrumbs
          .filter((c) => c.id !== space.data?.home_page_id)
          .map((c) => (
            <li key={c.id}>
              <Link href={`/pages/${c.id}`}>{c.title}</Link>
            </li>
          ))}
      </ol>
    </nav>
  );
}

function Byline({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const author = page.created_by_name || t('common.unknown');
  return (
    <div className="byline">
      <Avatar name={author} size="small" />
      <span>{t('page.byline', { name: author })}</span>
      <span aria-hidden="true">·</span>
      <span data-testid="last-updated">{t('page.lastUpdated', { date: formatDay(page.updated_at) })}</span>
      {page.current_version > 0 && (
        <>
          <span aria-hidden="true">·</span>
          <span>{t('history.version', { number: page.current_version })}</span>
        </>
      )}
      {page.restricted && <span className="lozenge">{t('page.restricted')}</span>}
    </div>
  );
}

function Attachments({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['attachments', page.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ attachments: Attachment[] }>(`/pages/${page.id}/attachments`),
  });
  const upload = useMutation({
    mutationFn: (f: File) =>
      api.upload(`/pages/${page.id}/attachments?filename=${encodeURIComponent(f.name)}`, f),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/attachments/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const list = q.data?.attachments ?? [];
  const canAdd = page.can['attachment.add'];
  return (
    <section className="attachments">
      <div className="section-head">
        <h2>{t('attachments.title')}</h2>
        {canAdd && <FilePicker label={t('attachments.upload')} onPick={(f) => upload.mutate(f)} />}
      </div>
      <ErrorNote error={upload.error ?? remove.error} />
      {list.length === 0 && <p className="muted">{t('attachments.none')}</p>}
      <ul className="plain-list">
        {list.map((a) => (
          <li key={a.id} className="row">
            {a.inline && <img className="thumb" src={`/api/v1/attachments/${a.id}/content`} alt="" />}
            <a href={`/api/v1/attachments/${a.id}/content?download=1`}>{a.filename}</a>
            <span className="muted">
              {t('attachments.version', { number: a.version })} · {formatSize(a.size)}
            </span>
            {canAdd && (
              <button type="button" className="ghost" onClick={() => remove.mutate(a.id)}>
                {t('common.delete')}
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function PageMenu({ page, onOpen }: { page: PageDetail; onOpen: (dialog: PageDialog) => void }) {
  const { t } = useTranslation();
  const canEdit = page.can['page.edit'];
  const published = page.current_version > 0;
  return (
    <Menu label={t('page.moreActions')} trigger={<IconMore />}>
      <MenuLink href={`/pages/${page.id}/history`}>
        <IconClock size={18} /> {t('page.history')}
      </MenuLink>
      {canEdit && (
        <>
          <MenuItem onClick={() => onOpen('rename')}>
            <IconRename size={18} /> {t('page.rename')}
          </MenuItem>
          <MenuItem onClick={() => onOpen('move')}>
            <IconMove size={18} /> {t('page.move')}
          </MenuItem>
        </>
      )}
      {page.can['page.create'] && published && (
        <MenuItem onClick={() => onOpen('copy')}>
          <IconCopy size={18} /> {t('page.copy')}
        </MenuItem>
      )}
      {page.can['space.export'] && published && (
        <>
          <MenuSeparator />
          <MenuDownload href={`/api/v1/pages/${page.id}/export.md`}>
            <IconDownload size={18} /> {t('page.exportMd')}
          </MenuDownload>
          <MenuDownload href={`/api/v1/pages/${page.id}/export.zip`}>
            <IconDownload size={18} /> {t('page.exportZip')}
          </MenuDownload>
        </>
      )}
      {page.can['page.delete'] && (
        <>
          <MenuSeparator />
          <MenuItem className="danger" onClick={() => onOpen('delete')}>
            <IconTrash size={18} /> {t('page.delete')}
          </MenuItem>
        </>
      )}
    </Menu>
  );
}

export function PageView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({ queryKey: ['page', pageId], queryFn: () => api.get<PageDetail>(`/pages/${pageId}`) });
  const [dialog, setDialog] = useState<PageDialog | null>(null);
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/pages/${pageId}/favourite`) : api.del(`/pages/${pageId}/favourite`),
    onSettled: () => client.invalidateQueries({ queryKey: ['page', pageId] }),
  });
  const remove = useMutation({
    mutationFn: () => api.del(`/pages/${pageId}`),
    onSuccess: async () => {
      const space = q.data?.space_id;
      await client.invalidateQueries({ queryKey: ['tree', space] });
      client.removeQueries({ queryKey: ['page', pageId] });
      navigate('/');
    },
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const page = q.data;
  const close = () => setDialog(null);
  const favouriteLabel = page.favourite ? t('page.unfavourite') : t('page.favourite');
  return (
    <article className="column">
      <div className="page-top">
        <Breadcrumbs page={page} />
        <div className="page-actions">
          {page.can['page.edit'] && (
            <Link className="button" href={`/pages/${page.id}/edit`}>
              <IconPencil size={16} /> {t('page.edit')}
            </Link>
          )}
          <button
            type="button"
            className="icon"
            aria-label={favouriteLabel}
            title={favouriteLabel}
            aria-pressed={page.favourite}
            onClick={() => favourite.mutate(!page.favourite)}
          >
            <IconStar />
          </button>
          <button
            type="button"
            className="icon"
            aria-label={t('page.restrictions')}
            title={t('page.restrictions')}
            onClick={() => setDialog('restrictions')}
          >
            <IconLock />
          </button>
          <PageMenu page={page} onOpen={setDialog} />
        </div>
      </div>
      <h1>{page.title}</h1>
      <Byline page={page} />
      <ErrorNote error={remove.error ?? favourite.error} />
      {page.status === 'draft' && (
        <p role="status" className="note">
          {t('page.unpublished')}
        </p>
      )}
      {page.has_draft && page.status !== 'draft' && (
        <p role="status" className="note">
          <Link href={`/pages/${page.id}/edit`}>{t('page.draftExists')}</Link>
        </p>
      )}
      <DocumentView document={page.document} />
      <Attachments page={page} />
      {dialog === 'restrictions' && <RestrictionsDialog page={page} onClose={close} />}
      {dialog === 'move' && <MoveDialog page={page} onClose={close} />}
      {dialog === 'rename' && <RenameDialog page={page} onClose={close} />}
      {dialog === 'copy' && <CopyDialog page={page} onClose={close} />}
      {dialog === 'delete' && (
        <ConfirmDialog
          title={t('page.delete')}
          message={t('page.confirmDelete')}
          confirmLabel={t('page.delete')}
          onConfirm={() => remove.mutate()}
          onClose={close}
        />
      )}
    </article>
  );
}
