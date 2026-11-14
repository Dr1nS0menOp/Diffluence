// SPDX-License-Identifier: Apache-2.0
// The release screen of a draft space (spec 073, PUB-1210 to PUB-1220): what waits to be
// released, what is left out, what was removed in the draft, and the history of releases. It
// exists only for those who may release; the server leaves out what the person may not see.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ReleaseRow, type ReleaseRun, type ReleaseScreen, type RunPage, type Space } from '../api';
import { Link } from '../router';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { Menu, MenuItem } from '../components/menu';
import { PagePicker } from '../components/picker';
import { LeaveOutDialog, RunDialog, type RunScope } from '../components/release-dialogs';
import { SortTable, type SortColumn } from '../components/sort-table';
import { Tabs } from '../components/tabs';
import { PageMark } from '../components/topbar';
import { Dialog, ErrorNote, Loading, RelativeTime } from '../components/ui';
import { MissingPage } from './organise';

const TABS = ['to_release', 'left_out', 'removed', 'history'] as const;
type Tab = (typeof TABS)[number];
const PANEL = 'release-panel';
const PAGE_SIZE = 200;
/** How often the screen asks about a run that goes, in milliseconds. */
const PROGRESS_EVERY = 2000;

function aspectWords(t: (key: string) => string, aspects: string[]): string {
  return aspects.map((a) => t(`publishing.aspect_${a}`)).join(', ');
}

function statusWords(t: (key: string) => string, row: ReleaseRow): string {
  switch (row.status) {
    case 'changed':
      return t('publishing.tileChanged');
    case 'cannot_release':
      return t('publishing.tileCannot');
    case 'left_out':
      return t('publishing.statusLeftOut');
    case 'left_out_released':
      return t('publishing.statusLeftOutReleased');
    case 'removed':
      return t('publishing.statusRemoved');
    default:
      return t('publishing.tileNotReleased');
  }
}

export function ReleaseView({ spaceKey }: { spaceKey: string }) {
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
  if (space.pair_role !== 'draft' || !space.can?.['space.release'])
    return <MissingPage spaceKey={spaceKey} />;
  return (
    <Frame space={space}>
      <ReleaseScreenOf space={space} />
    </Frame>
  );
}

function ReleaseScreenOf({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [tab, setTab] = useState<Tab>('to_release');
  const [offset, setOffset] = useState(0);
  const [picked, setPicked] = useState<string[]>([]);
  const [run, setRun] = useState<RunScope | null>(null);
  const [branch, setBranch] = useState(false);
  const [leaving, setLeaving] = useState<string | null>(null);
  const screen = useQuery({
    queryKey: ['release-screen', space.id, tab, offset],
    queryFn: () =>
      api.get<ReleaseScreen>(
        `/spaces/${space.id}/releases?tab=${tab === 'history' ? 'to_release' : tab}&offset=${offset}`,
      ),
    refetchInterval: (query) => {
      const going = query.state.data?.going;
      return going && (going.status === 'queued' || going.status === 'running') ? PROGRESS_EVERY : false;
    },
  });
  const refresh = async () => {
    setPicked([]);
    await client.invalidateQueries({ queryKey: ['release-screen', space.id] });
    await client.invalidateQueries({ queryKey: ['release-history', space.id] });
  };
  const include = useMutation({
    mutationFn: async () => {
      for (const id of picked) await api.del(`/pages/${id}/leave-out`);
    },
    onSettled: refresh,
  });
  const data = screen.data;
  const tiles = data?.tiles;
  const choose = (tabNext: Tab) => {
    setTab(tabNext);
    setOffset(0);
    setPicked([]);
  };
  return (
    <>
      <h1>{space.name}</h1>
      {data?.going && <RunProgress space={space} run={data.going} onChange={refresh} />}
      {tiles && (
        <dl className="tiles">
          <div>
            <dt>{t('publishing.tileNotReleased')}</dt>
            <dd>{tiles.not_released}</dd>
          </div>
          <div>
            <dt>{t('publishing.tileChanged')}</dt>
            <dd>{tiles.changed}</dd>
          </div>
          <div>
            <dt>{t('publishing.tileWaiting')}</dt>
            <dd>0</dd>
          </div>
          <div>
            <dt>{t('publishing.tileCannot')}</dt>
            <dd>{tiles.cannot_release}</dd>
          </div>
          <div>
            <dt>{t('publishing.tileRemoved')}</dt>
            <dd>{tiles.removed}</dd>
          </div>
        </dl>
      )}
      <div className="row">
        {tab === 'to_release' && (
          <button
            type="button"
            className="primary"
            onClick={() =>
              setRun(picked.length > 0 ? { scope: 'pages', page_ids: picked } : { scope: 'space' })
            }
          >
            {picked.length > 0
              ? t('publishing.releasePicked', { count: picked.length })
              : t('publishing.releaseAll')}
          </button>
        )}
        {tab === 'removed' && picked.length > 0 && (
          <button
            type="button"
            className="primary"
            onClick={() => setRun({ scope: 'take_off', page_ids: picked })}
          >
            {t('publishing.takeOffPicked')}
          </button>
        )}
        {tab === 'to_release' && (
          <button type="button" className="quiet" onClick={() => setBranch(true)}>
            {t('publishing.releaseBranch')}
          </button>
        )}
        {picked.length > 0 && tab !== 'removed' && (
          <Menu label={t('publishing.selection')} trigger={t('publishing.selection')}>
            {tab === 'to_release' && picked.length === 1 && (
              <MenuItem onClick={() => setLeaving(picked[0] ?? null)}>
                {t('publishing.leaveOutPicked')}
              </MenuItem>
            )}
            {tab === 'left_out' && (
              <MenuItem onClick={() => include.mutate()}>{t('publishing.includePicked')}</MenuItem>
            )}
            <MenuItem onClick={() => setRun({ scope: 'take_off', page_ids: picked })}>
              {t('publishing.takeOffPicked')}
            </MenuItem>
          </Menu>
        )}
      </div>
      <ErrorNote error={screen.error ?? include.error} />
      <Tabs
        label={t('publishing.screenTabs')}
        panel={PANEL}
        tabs={TABS}
        current={tab}
        name={(name) => t(`publishing.tab_${name}`)}
        onChange={choose}
      />
      <div id={PANEL} role="tabpanel" aria-labelledby={`${PANEL}-tab-${tab}`}>
        {tab === 'history' ? (
          <RunHistory space={space} />
        ) : (
          <>
            {screen.isPending && <Loading />}
            {data && data.rows.length === 0 && (
              <p className="muted">{t(`publishing.empty${emptyKey(tab)}`)}</p>
            )}
            {data && data.rows.length > 0 && (
              <ReleaseTable
                rows={data.rows}
                tab={tab}
                picked={picked}
                onPick={(id) =>
                  setPicked((now) => (now.includes(id) ? now.filter((x) => x !== id) : [...now, id]))
                }
              />
            )}
            {data?.more && (
              <button type="button" onClick={() => setOffset(offset + PAGE_SIZE)}>
                {t('publishing.more')}
              </button>
            )}
          </>
        )}
      </div>
      {run && (
        <RunDialog
          spaceId={space.id}
          scope={run}
          onClose={() => setRun(null)}
          onStarted={() => void refresh()}
        />
      )}
      {branch && (
        <BranchDialog
          onClose={() => setBranch(false)}
          onChosen={(root) => {
            setBranch(false);
            setRun({ scope: 'branch', root_page_id: root });
          }}
        />
      )}
      {leaving && (
        <LeaveOutDialog
          pageId={leaving}
          released={false}
          onClose={() => {
            setLeaving(null);
            void refresh();
          }}
          onTakeOff={() => undefined}
        />
      )}
    </>
  );
}

function emptyKey(tab: Tab): string {
  if (tab === 'left_out') return 'LeftOut';
  if (tab === 'removed') return 'Removed';
  return 'ToRelease';
}

/** The pages of a tab, with a real checkbox per row and the tint on the chosen ones (PUB-1211). */
function ReleaseTable({
  rows,
  tab,
  picked,
  onPick,
}: {
  rows: ReleaseRow[];
  tab: Tab;
  picked: string[];
  onPick: (id: string) => void;
}) {
  const { t } = useTranslation();
  const idOf = (row: ReleaseRow) => (tab === 'removed' ? (row.twin_page_id ?? row.page_id) : row.page_id);
  const columns: SortColumn<ReleaseRow>[] = [
    {
      key: 'pick',
      label: t('publishing.colPick'),
      value: (row) => (picked.includes(idOf(row)) ? 0 : 1),
      show: (row) => (
        <input
          type="checkbox"
          aria-label={t('publishing.pickPage', { title: row.title })}
          checked={picked.includes(idOf(row))}
          onChange={() => onPick(idOf(row))}
        />
      ),
    },
    {
      key: 'page',
      label: t('publishing.colPage'),
      value: (row) => row.title,
      show: (row) => (
        <>
          <PageMark page={row} />{' '}
          {tab === 'removed' && row.twin_page_id ? (
            <Link href={`/pages/${row.twin_page_id}`}>{row.title}</Link>
          ) : (
            <Link href={`/pages/${row.page_id}`}>{row.title}</Link>
          )}
          {row.path.length > 0 && <span className="muted"> · {row.path.join(' / ')}</span>}
        </>
      ),
    },
    {
      key: 'status',
      label: t('publishing.colStatus'),
      value: (row) => row.status,
      show: (row) => (
        <>
          <span className="lozenge">{statusWords(t, row)}</span>
          {row.twin_restricted && <span className="muted"> · {t('publishing.twinRestricted')}</span>}
        </>
      ),
    },
    tab === 'removed'
      ? {
          key: 'cause',
          label: t('publishing.colCause'),
          value: (row) => row.reason ?? '',
          show: (row) => t(`publishing.cause_${row.reason ?? 'deleted'}`),
        }
      : tab === 'left_out'
        ? {
            key: 'reason',
            label: t('publishing.colReason'),
            value: (row) => row.left_out?.reason ?? '',
          }
        : {
            key: 'changed',
            label: t('publishing.colChanged'),
            value: (row) => row.aspects.join(','),
            show: (row) =>
              row.status === 'cannot_release'
                ? t(`publishing.reason_${row.reason || 'kind'}`)
                : aspectWords(t, row.aspects),
          },
    {
      key: 'since',
      label: t('publishing.colSince'),
      value: (row) => row.since,
      show: (row) => <RelativeTime value={row.since} />,
    },
    {
      key: 'released',
      label: t('publishing.colReleased'),
      value: (row) => row.released_at ?? '',
      show: (row) =>
        row.status === 'changed' ? (
          <Link
            className="button ghost"
            href={`/pages/${row.page_id}/compare?from=${row.released_version}&to=${row.current_version}`}
          >
            {t('publishing.compare')}
          </Link>
        ) : row.released_at ? (
          <RelativeTime value={row.released_at} />
        ) : (
          t('publishing.never')
        ),
    },
  ];
  return (
    <SortTable
      caption={t('publishing.tableCaption')}
      columns={columns}
      rows={rows}
      rowKey={idOf}
      rowClass={(row) => (picked.includes(idOf(row)) ? 'picked' : undefined)}
    />
  );
}

/** A run that waits, goes or stopped, with its progress as text (PUB-1214). */
function RunProgress({
  space,
  run,
  onChange,
}: {
  space: Space;
  run: ReleaseRun;
  onChange: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const act = useMutation({
    mutationFn: (what: 'cancel' | 'resume') => api.post(`/spaces/${space.id}/releases/${run.id}/${what}`),
    onSettled: onChange,
  });
  let line = t('publishing.progress', { done: run.done, total: run.total });
  if (run.status === 'queued') line = t('publishing.waiting');
  if (run.status === 'stopped') {
    line = t('publishing.stopped', { reason: t(`publishing.stop_${run.stop_reason || 'failed'}`) });
  }
  return (
    <div className="note">
      <ErrorNote error={act.error} />
      <p role="status">{line}</p>
      <div className="row">
        {run.status === 'stopped' && (
          <button type="button" disabled={act.isPending} onClick={() => act.mutate('resume')}>
            {t('publishing.resumeRun')}
          </button>
        )}
        <button type="button" className="quiet" disabled={act.isPending} onClick={() => act.mutate('cancel')}>
          {t('publishing.cancelRun')}
        </button>
      </div>
    </div>
  );
}

/** The runs of the pair, newest first, each opening its report (PUB-910, PUB-1215). */
function RunHistory({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ['release-history', space.id],
    queryFn: () => api.get<{ runs: ReleaseRun[] }>(`/spaces/${space.id}/releases/history`),
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const runs = q.data.runs;
  if (runs.length === 0) return <p className="muted">{t('publishing.emptyHistory')}</p>;
  const columns: SortColumn<ReleaseRun>[] = [
    {
      key: 'when',
      label: t('publishing.colWhen'),
      value: (r) => r.finished_at ?? r.started_at ?? r.previewed_at,
      show: (r) => <RelativeTime value={r.finished_at ?? r.started_at ?? r.previewed_at} />,
    },
    { key: 'who', label: t('publishing.colWho'), value: (r) => r.actor_name },
    { key: 'scope', label: t('publishing.colScope'), value: (r) => t(`publishing.scope_${r.scope}`) },
    {
      key: 'outcome',
      label: t('publishing.colOutcome'),
      value: (r) => r.status,
      show: (r) => <span className="lozenge">{t(`publishing.runStatus_${r.status}`)}</span>,
    },
    { key: 'note', label: t('publishing.colNote'), value: (r) => r.note },
    {
      key: 'report',
      label: t('publishing.report'),
      value: () => 0,
      show: (r) => (
        <button type="button" className="ghost" onClick={() => setOpen(r.id)}>
          {t('publishing.report')}
        </button>
      ),
    },
  ];
  return (
    <>
      <SortTable
        caption={t('publishing.historyCaption')}
        columns={columns}
        rows={runs}
        rowKey={(r) => r.id}
      />
      {open && <RunReport space={space} runId={open} onClose={() => setOpen(null)} />}
    </>
  );
}

function RunReport({ space, runId, onClose }: { space: Space; runId: string; onClose: () => void }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['release-run', runId],
    queryFn: () => api.get<ReleaseRun>(`/spaces/${space.id}/releases/${runId}`),
  });
  const pages: RunPage[] = q.data?.pages ?? [];
  return (
    <Dialog title={t('publishing.reportTitle')} onClose={onClose} wide>
      <ErrorNote error={q.error} />
      {q.isPending && <Loading />}
      {q.data && (
        <ul className="plain-list">
          {pages.map((p) => (
            <li key={p.page_id}>
              {p.title}{' '}
              <span className="muted">
                {t(`publishing.outcome_${p.outcome ?? 'waiting'}`)}
                {p.reason ? ` · ${t(`publishing.skip_${p.reason}`)}` : ''}
              </span>
            </li>
          ))}
        </ul>
      )}
      <div className="dialog-actions">
        <button type="button" onClick={onClose}>
          {t('common.close')}
        </button>
      </div>
    </Dialog>
  );
}

/** Asks for the top page of a branch, with the existing page chooser (PUB-1212). */
function BranchDialog({ onClose, onChosen }: { onClose: () => void; onChosen: (id: string) => void }) {
  const { t } = useTranslation();
  const [root, setRoot] = useState('');
  return (
    <Dialog title={t('publishing.branchTitle')} onClose={onClose}>
      <PagePicker
        label={t('publishing.branchPage')}
        help={t('publishing.branchHelp')}
        value={root}
        onChange={setRoot}
      />
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" disabled={root === ''} onClick={() => onChosen(root)}>
          {t('publishing.branchNext')}
        </button>
      </div>
    </Dialog>
  );
}
