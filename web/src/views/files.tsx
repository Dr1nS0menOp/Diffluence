// SPDX-License-Identifier: Apache-2.0
// Every file of a space in one list (FIL-010). The server leaves out what the reader
// may not see, so nothing here decides about access.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api, type Space } from '../api';
import { Link } from '../router';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { ErrorNote, formatSize, Loading } from '../components/ui';
import { useDates } from '../dates';

const FILES_PAGE_SIZE = 50;

interface SpaceFile {
  id: string;
  filename: string;
  size: number;
  page_id: string;
  page_title: string;
  uploader: string;
  updated_at: string;
}

interface Usage {
  bytes: number;
  largest: SpaceFile[];
}

function FileTable({ files }: { files: SpaceFile[] }) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <div className="block-frame">
      <table className="file-table">
        <thead>
          <tr>
            <th>{t('files.name')}</th>
            <th>{t('files.page')}</th>
            <th>{t('files.size')}</th>
            <th>{t('files.uploader')}</th>
            <th>{t('files.changed')}</th>
          </tr>
        </thead>
        <tbody>
          {files.map((file) => (
            <tr key={file.id}>
              <td>
                <a href={`/api/v1/attachments/${file.id}/content?download=1`}>{file.filename}</a>
              </td>
              <td>
                <Link href={`/pages/${file.page_id}`}>{file.page_title}</Link>
              </td>
              <td>{formatSize(file.size)}</td>
              <td>{file.uploader}</td>
              <td>{dates.day(file.updated_at)}</td>
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
          <p>{t('files.total', { size: formatSize(q.data.bytes) })}</p>
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

function FileList({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const q = useInfiniteQuery({
    queryKey: ['space-files', space.id, search],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.get<{ files: SpaceFile[] }>(
        `/spaces/${space.id}/files?q=${encodeURIComponent(search)}&offset=${pageParam}`,
      ),
    getNextPageParam: (last, pages) =>
      last.files.length === FILES_PAGE_SIZE ? pages.length * FILES_PAGE_SIZE : undefined,
  });
  const files = q.data?.pages.flatMap((page) => page.files) ?? [];
  return (
    <>
      <input
        type="search"
        aria-label={t('files.search')}
        placeholder={t('files.search')}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && files.length === 0 && <p className="muted">{t('files.none')}</p>}
      {files.length > 0 && <FileTable files={files} />}
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
