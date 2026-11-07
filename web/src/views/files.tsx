// SPDX-License-Identifier: Apache-2.0
// Every file of a space in one list (FIL-010). The server leaves out what the reader
// may not see, so nothing here decides about access.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Space } from '../api';
import { Link } from '../router';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { ConfirmButton, ErrorNote, formatSize, Loading } from '../components/ui';
import { useDates } from '../dates';

const FILES_PAGE_SIZE = 50;
const FILE_KINDS = ['image', 'media', 'pdf', 'text', 'office', 'other'] as const;

interface SpaceFile {
  id: string;
  filename: string;
  size: number;
  page_id: string;
  page_title: string;
  uploader: string;
  updated_at: string;
  labels: string[];
}

interface Usage {
  bytes: number;
  limit_bytes: number;
  largest: SpaceFile[];
}

interface FileChoices {
  labels: string[];
  uploaders: { id: string; name: string }[];
}

interface FileFilters {
  kind: string;
  label: string;
  uploader: string;
}

function FileTable({
  files,
  picked,
  onPick,
}: {
  files: SpaceFile[];
  /** Only someone who may delete in bulk gets boxes to tick. */
  picked?: string[];
  onPick?: (id: string) => void;
}) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <div className="block-frame">
      <table className="file-table">
        <thead>
          <tr>
            {picked && <th>{t('files.pick')}</th>}
            <th>{t('files.name')}</th>
            <th>{t('files.page')}</th>
            <th>{t('files.size')}</th>
            <th>{t('files.uploader')}</th>
            <th>{t('files.changed')}</th>
            <th>{t('files.labels')}</th>
          </tr>
        </thead>
        <tbody>
          {files.map((file) => (
            <tr key={file.id}>
              {picked && (
                <td>
                  <input
                    type="checkbox"
                    aria-label={t('files.pickFile', { name: file.filename })}
                    checked={picked.includes(file.id)}
                    onChange={() => onPick?.(file.id)}
                  />
                </td>
              )}
              <td>
                <a href={`/api/v1/attachments/${file.id}/content?download=1`}>{file.filename}</a>
              </td>
              <td>
                <Link href={`/pages/${file.page_id}`}>{file.page_title}</Link>
              </td>
              <td>{formatSize(file.size)}</td>
              <td>{file.uploader}</td>
              <td>{dates.day(file.updated_at)}</td>
              <td>{file.labels.join(', ')}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SpaceUsage({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['file-usage', space.id],
    queryFn: () => api.get<Usage>(`/spaces/${space.id}/files/usage`),
  });
  return (
    <section aria-label={t('files.usage')}>
      <ErrorNote error={q.error} />
      {q.data && (
        <>
          <p>
            {q.data.limit_bytes > 0
              ? t('files.totalOfLimit', {
                  size: formatSize(q.data.bytes),
                  limit: formatSize(q.data.limit_bytes),
                })
              : t('files.total', { size: formatSize(q.data.bytes) })}
          </p>
          {q.data.largest.length > 0 && (
            <>
              <h2>{t('files.largest')}</h2>
              <FileTable files={q.data.largest} />
            </>
          )}
        </>
      )}
    </section>
  );
}

/** The choices come from the server, so they hold nothing from a page this person cannot see. */
function FilterRow({
  space,
  filters,
  onChange,
}: {
  space: Space;
  filters: FileFilters;
  onChange: (next: FileFilters) => void;
}) {
  const { t } = useTranslation();
  const choices = useQuery({
    queryKey: ['file-choices', space.id],
    queryFn: () => api.get<FileChoices>(`/spaces/${space.id}/files/filters`),
  });
  return (
    <>
      <ErrorNote error={choices.error} />
      <select
        aria-label={t('files.filterKind')}
        value={filters.kind}
        onChange={(e) => onChange({ ...filters, kind: e.target.value })}
      >
        <option value="">{t('files.anyKind')}</option>
        {FILE_KINDS.map((kind) => (
          <option key={kind} value={kind}>
            {t(`files.kind_${kind}`)}
          </option>
        ))}
      </select>
      <select
        aria-label={t('files.filterLabel')}
        value={filters.label}
        onChange={(e) => onChange({ ...filters, label: e.target.value })}
      >
        <option value="">{t('files.anyLabel')}</option>
        {choices.data?.labels.map((label) => (
          <option key={label} value={label}>
            {label}
          </option>
        ))}
      </select>
      <select
        aria-label={t('files.filterPerson')}
        value={filters.uploader}
        onChange={(e) => onChange({ ...filters, uploader: e.target.value })}
      >
        <option value="">{t('files.anyone')}</option>
        {choices.data?.uploaders.map((person) => (
          <option key={person.id} value={person.id}>
            {person.name}
          </option>
        ))}
      </select>
    </>
  );
}

function BulkDelete({ space, picked, onDone }: { space: Space; picked: string[]; onDone: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const remove = useMutation({
    mutationFn: () =>
      api.post<{ deleted: number; skipped: number }>(`/spaces/${space.id}/files/delete`, { ids: picked }),
    onSuccess: onDone,
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['space-files', space.id] });
      await client.invalidateQueries({ queryKey: ['file-usage', space.id] });
    },
  });
  return (
    <>
      <ErrorNote error={remove.error} />
      {remove.data && (
        <p role="status" className="note">
          {t('files.deleted', { count: remove.data.deleted })}{' '}
          {remove.data.skipped > 0 && t('files.skipped', { count: remove.data.skipped })}
        </p>
      )}
      {picked.length > 0 && (
        <ConfirmButton
          label={t('files.deletePicked', { count: picked.length })}
          message={t('files.confirmDeletePicked', { count: picked.length })}
          className="danger"
          onConfirm={() => remove.mutate()}
        />
      )}
    </>
  );
}

function FileList({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [filters, setFilters] = useState<FileFilters>({ kind: '', label: '', uploader: '' });
  const [picked, setPicked] = useState<string[]>([]);
  const query = new URLSearchParams({ q: search, ...filters }).toString();
  const q = useInfiniteQuery({
    queryKey: ['space-files', space.id, query],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.get<{ files: SpaceFile[] }>(`/spaces/${space.id}/files?${query}&offset=${pageParam}`),
    getNextPageParam: (last, pages) =>
      last.files.length === FILES_PAGE_SIZE ? pages.length * FILES_PAGE_SIZE : undefined,
  });
  const files = q.data?.pages.flatMap((page) => page.files) ?? [];
  const canDelete = space.can?.['space.administer'] ?? false;
  const pick = (id: string) =>
    setPicked((current) => (current.includes(id) ? current.filter((x) => x !== id) : [...current, id]));
  return (
    <>
      <div className="file-filters">
        <input
          type="search"
          aria-label={t('files.search')}
          placeholder={t('files.search')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <FilterRow space={space} filters={filters} onChange={setFilters} />
      </div>
      {canDelete && <BulkDelete space={space} picked={picked} onDone={() => setPicked([])} />}
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && files.length === 0 && <p className="muted">{t('files.none')}</p>}
      {files.length > 0 && <FileTable files={files} picked={canDelete ? picked : undefined} onPick={pick} />}
      {q.hasNextPage && (
        <button type="button" onClick={() => void q.fetchNextPage()}>
          {t('files.more')}
        </button>
      )}
    </>
  );
}

export function FilesView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  const space = q.data;
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space) return <Loading />;
  return (
    <Frame space={space}>
      <h1>{t('files.title')}</h1>
      <FileList space={space} />
      {space.can?.['space.administer'] && <SpaceUsage space={space} />}
    </Frame>
  );
}
