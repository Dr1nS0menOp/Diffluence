// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type TrashItem } from '../api';
import { navigate } from '../router';
import { spaceByKeyQuery } from '../components/frame';
import { useDelayed } from '../components/quick-search';
import { ConfirmButton, ErrorNote, Loading } from '../components/ui';
import { useDates } from '../dates';
import { TableBox } from '../components/table-box';
import { SpaceToolFrame } from './space-settings-frame';

function useSpaceByKey(key: string) {
  return useQuery(spaceByKeyQuery(key));
}

/** Opening a space goes to its home page. */
export function SpaceHome({ spaceKey }: { spaceKey: string }) {
  const space = useSpaceByKey(spaceKey);
  useEffect(() => {
    if (space.data?.home_page_id) navigate(`/pages/${space.data.home_page_id}`, true);
  }, [space.data]);
  if (space.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={space.error} />
      </main>
    );
  }
  return <Loading />;
}

export function TrashView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const q = useSpaceByKey(spaceKey);
  const space = q.data;
  const [filter, setFilter] = useState('');
  const [byTitle, setByTitle] = useState(false);
  const [picked, setPicked] = useState<string[]>([]);
  const wanted = useDelayed(filter.trim());
  const items = useQuery({
    queryKey: ['trash', space?.id, wanted, byTitle],
    queryFn: () => {
      const params = new URLSearchParams({ q: wanted });
      if (byTitle) params.set('sort', 'title');
      return api.get<{ items: TrashItem[] }>(`/spaces/${space?.id}/trash?${params.toString()}`);
    },
    enabled: !!space,
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['trash', space?.id] });
    await client.invalidateQueries({ queryKey: ['tree', space?.id] });
  };
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/trash/${id}/restore`),
    onSettled: refresh,
  });
  const purge = useMutation({ mutationFn: (id: string) => api.del(`/trash/${id}`), onSettled: refresh });
  const bulk = useMutation({
    mutationFn: (action: 'restore' | 'purge') =>
      api.post(`/spaces/${space?.id}/trash/bulk`, { action, ids: picked }),
    onSuccess: () => setPicked([]),
    onSettled: refresh,
  });
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space) return <Loading />;
  return (
    <SpaceToolFrame space={space} entry="trash">
      <h1>{t('trash.title')}</h1>
      <ErrorNote error={items.error ?? restore.error ?? purge.error ?? bulk.error} />
      <div className="row">
        <input
          type="search"
          className="grow"
          value={filter}
          aria-label={t('trash.filter')}
          placeholder={t('trash.filter')}
          onChange={(e) => setFilter(e.target.value)}
        />
        <label className="check">
          <input type="checkbox" checked={byTitle} onChange={(e) => setByTitle(e.target.checked)} />
          {t('organise.byName')}
        </label>
      </div>
      {picked.length > 0 && (
        <div className="row" role="group" aria-label={t('content.actions')}>
          <span>{t('content.selected', { count: picked.length })}</span>
          <button type="button" onClick={() => bulk.mutate('restore')}>
            {t('trash.restore')}
          </button>
          <ConfirmButton
            label={t('trash.purge')}
            message={t('trash.confirmPurgeMany', { count: picked.length })}
            className="danger"
            onConfirm={() => bulk.mutate('purge')}
          />
        </div>
      )}
      {items.data?.items.length === 0 && <p className="muted">{t('trash.empty')}</p>}
      {(items.data?.items.length ?? 0) > 0 && (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('history.pick')}</th>
                <th>{t('common.name')}</th>
                <th>{t('common.type')}</th>
                <th>{t('trash.deletedBy')}</th>
                <th>{t('trash.deleted')}</th>
                <th>{t('trash.purgeAfter')}</th>
                <th>{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {items.data?.items.map((i) => (
                <tr key={i.id} className={picked.includes(i.id) ? 'picked' : undefined}>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t('favourites.pick', { title: i.hidden ? t('trash.restricted') : i.title })}
                      checked={picked.includes(i.id)}
                      onChange={(e) =>
                        setPicked(e.target.checked ? [...picked, i.id] : picked.filter((x) => x !== i.id))
                      }
                    />
                  </td>
                  <td>{i.hidden ? t('trash.restricted') : i.title}</td>
                  <td>{i.object_type}</td>
                  <td>{i.deleted_by_name || t('common.unknown')}</td>
                  <td>{dates.moment(i.deleted_at)}</td>
                  <td>{dates.moment(i.purge_after)}</td>
                  <td className="row">
                    <button type="button" onClick={() => restore.mutate(i.id)}>
                      {t('trash.restore')}
                    </button>
                    {!i.hidden && (
                      <ConfirmButton
                        label={t('trash.purge')}
                        message={t('trash.confirmPurge')}
                        className="danger"
                        onConfirm={() => purge.mutate(i.id)}
                      />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
    </SpaceToolFrame>
  );
}
