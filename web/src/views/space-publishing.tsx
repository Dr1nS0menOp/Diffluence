// SPDX-License-Identifier: Apache-2.0
// Space settings, group Content, section "Published space" (spec 073, PUB-1200): in a space
// in no pair, what a pair is and the way to link one, with its preview; in a draft space the
// settings of the pair, who may release and its health; in a published space where it comes
// from. Ending a pair stays here for the administrators of both spaces.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type JobState,
  type LinkPreview,
  type PairSettings,
  type PublishingState,
  type Space,
  type SpaceVerification,
  type StartPreview,
} from '../api';
import { Link } from '../router';
import { ConfirmButton, Dialog, ErrorNote, Field, formatSize, Loading, RelativeTime } from '../components/ui';
import { pageTitle } from '../page-name';
import { TableBox } from '../components/table-box';

const NAMES_SHOWN = 50;

export function PublishingSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['publishing', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<PublishingState>(`/spaces/${space.id}/publishing`),
  });
  const [linking, setLinking] = useState(false);
  const [starting, setStarting] = useState(false);
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: key });
    await client.invalidateQueries({ queryKey: ['space-id', space.id] });
    await client.invalidateQueries({ queryKey: ['space', space.key] });
  };
  const end = useMutation({
    mutationFn: () => api.post(`/spaces/${space.id}/publishing/end`),
    onSuccess: refresh,
  });
  if (q.isLoading) return <Loading />;
  const state = q.data;
  if (!state) return <ErrorNote error={q.error} />;
  const pair = state.pair;
  return (
    <section aria-labelledby="publishing-heading">
      <h2 id="publishing-heading" className="visually-hidden">
        {t('publishing.settingsTitle')}
      </h2>
      <ErrorNote error={end.error} />
      {state.side === '' && (
        <>
          <p>{t('publishing.explain')}</p>
          {state.can.start && <p>{t('publishing.explainStart')}</p>}
          <div className="row">
            {state.can.link && (
              <button type="button" className="primary" onClick={() => setLinking(true)}>
                {t('publishing.link')}
              </button>
            )}
            {state.can.start && (
              <button type="button" onClick={() => setStarting(true)}>
                {t('publishing.start')}
              </button>
            )}
          </div>
        </>
      )}
      {state.side === 'draft' && pair && (
        <>
          <p>
            {pair.published
              ? t('publishing.draftSide', { name: pair.published.name })
              : t('publishing.draftSideHidden')}{' '}
            {pair.published && (
              <Link href={`/spaces/${pair.published.key}`}>{t('publishing.openPublished')}</Link>
            )}
          </p>
          {pair.health && <Health health={pair.health} />}
          {state.can.verify && <VerifySpace space={space} />}
          {state.can.release && <LinkingIn space={space} />}
          {pair.settings && <PairSettingsForm space={space} settings={pair.settings} onSaved={refresh} />}
          {pair.releasers && (
            <>
              <h2>{t('publishing.releasers')}</h2>
              <p className="muted">{t('publishing.releasersHelp')}</p>
              {pair.releasers.length === 0 ? (
                <p className="muted">{t('publishing.noReleasers')}</p>
              ) : (
                <ul className="plain-list">
                  {pair.releasers.map((r) => (
                    <li key={`${r.type}-${r.id}`}>{r.name || t(`subjects.${r.type}`)}</li>
                  ))}
                </ul>
              )}
            </>
          )}
        </>
      )}
      {state.side === 'published' && pair && (
        <p>
          {pair.draft ? t('publishing.publishedSide', { name: pair.draft.name }) : t('publishing.managed')}{' '}
          {pair.draft && <Link href={`/spaces/${pair.draft.key}`}>{t('publishing.openDraftSpace')}</Link>}
        </p>
      )}
      {state.side === 'published' && pair && pair.draft && <p className="muted">{t('publishing.managed')}</p>}
      {state.can.end && (
        <div className="row">
          <ConfirmButton
            label={t('publishing.end')}
            message={t('publishing.endConfirm')}
            tone="danger"
            onConfirm={() => end.mutate()}
          />
          <UndoStart space={space} onUndone={refresh} />
        </div>
      )}
      {linking && <LinkDialog space={space} onClose={() => setLinking(false)} onLinked={refresh} />}
      {starting && <StartDialog space={space} onClose={() => setStarting(false)} onStarted={refresh} />}
    </section>
  );
}

/** Undoes a start before its first release: the draft space goes to the trash (PUB-1311). It
 * shows only while that is still possible. */
function UndoStart({ space, onUndone }: { space: Space; onUndone: () => Promise<void> }) {
  const { t } = useTranslation();
  const end = useQuery({
    queryKey: ['publishing-end', space.id],
    queryFn: () => api.get<{ undo: boolean }>(`/spaces/${space.id}/publishing/end-preview`),
  });
  const undo = useMutation({
    mutationFn: () => api.post(`/spaces/${space.id}/publishing/undo-start`),
    onSuccess: onUndone,
  });
  if (!end.data?.undo) return null;
  return (
    <>
      <ConfirmButton
        label={t('publishing.undoStart')}
        message={t('publishing.undoStartConfirm')}
        tone="danger"
        onConfirm={() => undo.mutate()}
      />
      <ErrorNote error={undo.error} />
    </>
  );
}

/** How often the screen asks about a job in the background. */
const JOB_POLL_MS = 1000;

/** Follows a job of its maker until it is done or failed. */
function useJob(job: string | null) {
  return useQuery({
    queryKey: ['job', job],
    queryFn: () => api.get<JobState>(`/jobs/${job}`),
    enabled: !!job,
    refetchInterval: (q) => (['done', 'failed'].includes(q.state.data?.status ?? '') ? false : JOB_POLL_MS),
  });
}

/** What a failed job says went wrong. */
function jobError(job: JobState | undefined): string | null {
  const result = job?.status === 'failed' ? job.result : null;
  if (typeof result !== 'object' || result === null || !('error' in result)) return null;
  return typeof result.error === 'string' ? result.error : null;
}

/** The result of a job that is done, as the route that queued it describes it. */
function jobResult<R>(job: JobState | undefined): R | null {
  return job?.status === 'done' && typeof job.result === 'object' && job.result !== null
    ? (job.result as R)
    : null;
}

/** Starts a draft space from this space, which has content: its name and key, the preview in
 * numbers, and then the start in the background (PUB-1302, PUB-1309). */
function StartDialog({
  space,
  onClose,
  onStarted,
}: {
  space: Space;
  onClose: () => void;
  onStarted: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [key, setKey] = useState('');
  const [preview, setPreview] = useState<StartPreview | null>(null);
  const [job, setJob] = useState<string | null>(null);
  const ask = useMutation({
    mutationFn: () => api.post<StartPreview>(`/spaces/${space.id}/publishing/start-preview`, { name, key }),
    onSuccess: setPreview,
  });
  const start = useMutation({
    mutationFn: () => api.post<{ job_id: string }>(`/spaces/${space.id}/publishing/start`, { name, key }),
    onSuccess: (out) => setJob(out.job_id),
  });
  const followed = useJob(job);
  const failure = jobError(followed.data);
  const result = jobResult<StartResult>(followed.data);
  const change = (fn: () => void) => {
    fn();
    setPreview(null);
  };
  const close = () => {
    if (result) void onStarted();
    onClose();
  };
  return (
    <Dialog title={t('publishing.startTitle')} onClose={close}>
      <ErrorNote error={ask.error ?? start.error ?? followed.error} />
      {failure && <p className="note note-error">{failure}</p>}
      {!job && (
        <>
          <Field label={t('publishing.startName')}>
            <input value={name} onChange={(e) => change(() => setName(e.target.value))} required />
          </Field>
          <Field label={t('publishing.startKey')}>
            <input value={key} onChange={(e) => change(() => setKey(e.target.value))} required />
          </Field>
          {preview && <StartPreviewWords preview={preview} />}
        </>
      )}
      {job && !result && !failure && <p role="status">{t('publishing.starting')}</p>}
      {result && (
        <p role="status">
          {t('publishing.started')}{' '}
          <Link href={`/spaces/${result.draft_key}`}>{t('publishing.openStarted')}</Link>
        </p>
      )}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={close}>
          {result ? t('common.close') : t('common.cancel')}
        </button>
        {!job &&
          (preview ? (
            <button
              type="button"
              className="primary"
              disabled={start.isPending}
              onClick={() => start.mutate()}
            >
              {t('publishing.startConfirm')}
            </button>
          ) : (
            <button type="button" className="primary" disabled={ask.isPending} onClick={() => ask.mutate()}>
              {t('publishing.preview')}
            </button>
          ))}
      </div>
    </Dialog>
  );
}

interface StartResult {
  draft_space_id: string;
  draft_key: string;
  pages: number;
}

function StartPreviewWords({ preview }: { preview: StartPreview }) {
  const { t } = useTranslation();
  const moving = preview.move_to_draft;
  return (
    <div role="status" className="note">
      <p>{t('publishing.startPreviewPages', { count: preview.pages })}</p>
      {preview.hidden > 0 && <p>{t('publishing.startPreviewHidden', { count: preview.hidden })}</p>}
      {preview.restricted > 0 && (
        <p>{t('publishing.startPreviewRestricted', { count: preview.restricted })}</p>
      )}
      {preview.files > 0 && (
        <p>{t('publishing.startPreviewFiles', { count: preview.files, size: formatSize(preview.bytes) })}</p>
      )}
      {moving.everyone && <p>{t('publishing.startPreviewMoveEveryone')}</p>}
      {(moving.people > 0 || moving.groups > 0) && (
        <p>{t('publishing.startPreviewMove', { people: moving.people, groups: moving.groups })}</p>
      )}
      {(moving.names ?? []).length > 0 && (
        <p className="muted">
          {t('publishing.previewNames', { names: (moving.names ?? []).slice(0, NAMES_SHOWN).join(', ') })}
        </p>
      )}
      <p>{t('publishing.startPreviewReaders')}</p>
      {(preview.posts > 0 || preview.databases > 0) && (
        <p>{t('publishing.startPreviewStays', { posts: preview.posts, databases: preview.databases })}</p>
      )}
      {(preview.archived > 0 || preview.unpublished > 0) && (
        <p>
          {t('publishing.startPreviewLeft', { archived: preview.archived, unpublished: preview.unpublished })}
        </p>
      )}
      <p>{t('publishing.startPreviewTime', { count: Math.max(1, preview.seconds) })}</p>
    </div>
  );
}

/** The verification of every published page, in the background, with its report (PUB-920). */
function VerifySpace({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [job, setJob] = useState<string | null>(null);
  const start = useMutation({
    mutationFn: () => api.post<{ job_id: string }>(`/spaces/${space.id}/releases/verify`),
    onSuccess: (out) => setJob(out.job_id),
  });
  const followed = useJob(job);
  const failure = jobError(followed.data);
  const report = jobResult<SpaceVerification>(followed.data);
  const going = !!job && !report && !failure;
  const differing = (report?.pages ?? []).filter((p) => p.outcome !== 'verified');
  return (
    <>
      <h2>{t('publishing.verifySpace')}</h2>
      <p className="muted">{t('publishing.verifySpaceHelp')}</p>
      <ErrorNote error={start.error ?? followed.error} />
      {failure && <p className="note note-error">{failure}</p>}
      <div className="row">
        <button type="button" disabled={start.isPending || going} onClick={() => start.mutate()}>
          {t('publishing.verifySpace')}
        </button>
        {going && <span role="status">{t('publishing.verifying')}</span>}
      </div>
      {report && (
        <div role="status">
          <p>
            {t('publishing.verifyCounts', {
              verified: report.counts.verified ?? 0,
              differs: report.counts.differs ?? 0,
              draftChanged: report.counts.draft_changed ?? 0,
              cannot: report.counts.cannot_verify ?? 0,
              purged: report.counts.draft_purged ?? 0,
            })}
          </p>
          {report.hidden > 0 && (
            <p className="muted">{t('publishing.verifyHidden', { count: report.hidden })}</p>
          )}
          {differing.length > 0 && (
            <TableBox>
              <table>
                <caption className="visually-hidden">{t('publishing.verifyListCaption')}</caption>
                <thead>
                  <tr>
                    <th>{t('publishing.colPage')}</th>
                    <th>{t('publishing.colOutcome')}</th>
                  </tr>
                </thead>
                <tbody>
                  {differing.map((p) => (
                    <tr key={p.twin_page_id}>
                      <td>
                        <Link href={`/pages/${p.twin_page_id}`}>{pageTitle(t, p.title)}</Link>
                      </td>
                      <td>{t(`publishing.verify_${p.outcome}`)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </TableBox>
          )}
        </div>
      )}
    </>
  );
}

function Health({ health }: { health: NonNullable<NonNullable<PublishingState['pair']>['health']> }) {
  const { t } = useTranslation();
  return (
    <>
      <h2>{t('publishing.health')}</h2>
      {health.drafts_readable && <p className="note">{t('publishing.healthReadable')}</p>}
      <p>
        {health.out_of_step > 0 && health.since ? (
          <>
            {t('publishing.healthOutOfStep', { count: health.out_of_step })}{' '}
            <RelativeTime value={health.since} />
          </>
        ) : (
          t('publishing.healthInStep')
        )}
      </p>
    </>
  );
}

function PairSettingsForm({
  space,
  settings,
  onSaved,
}: {
  space: Space;
  settings: PairSettings;
  onSaved: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState(settings);
  const save = useMutation({
    mutationFn: () => api.put(`/spaces/${space.id}/publishing/settings`, form),
    onSuccess: onSaved,
  });
  return (
    <form
      className="settings-form"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <h2>{t('publishing.settings')}</h2>
      <ErrorNote error={save.error} />
      <label className="check">
        <input
          type="checkbox"
          checked={form.notify_watchers}
          onChange={(e) => setForm({ ...form, notify_watchers: e.target.checked })}
        />{' '}
        {t('publishing.notifyWatchers')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={form.mirror_removals}
          onChange={(e) => setForm({ ...form, mirror_removals: e.target.checked })}
        />{' '}
        {t('publishing.mirrorRemovals')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={form.only_approval_releases}
          aria-describedby="only-approval-help"
          onChange={(e) => setForm({ ...form, only_approval_releases: e.target.checked })}
        />{' '}
        {t('publishing.onlyApprovalReleases')}
      </label>
      <small id="only-approval-help" className="muted">
        {t('publishing.onlyApprovalReleasesHelp')}
      </small>
      <label className="check">
        <input
          type="checkbox"
          checked={form.show_approvers}
          aria-describedby="show-approvers-help"
          onChange={(e) => setForm({ ...form, show_approvers: e.target.checked })}
        />{' '}
        {t('publishing.showApprovers')}
      </label>
      <small id="show-approvers-help" className="muted">
        {t('publishing.showApproversHelp')}
      </small>
      <Field label={t('publishing.defaultNote')}>
        <textarea
          value={form.default_note}
          maxLength={2000}
          rows={2}
          onChange={(e) => setForm({ ...form, default_note: e.target.value })}
        />
      </Field>
      <button type="submit" className="primary" disabled={save.isPending}>
        {t('common.save')}
      </button>
    </form>
  );
}

type Choice = 'new' | 'existing';

function LinkDialog({
  space,
  onClose,
  onLinked,
}: {
  space: Space;
  onClose: () => void;
  onLinked: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [choice, setChoice] = useState<Choice>('new');
  const [key, setKey] = useState('');
  const [name, setName] = useState('');
  const [target, setTarget] = useState('');
  const [preview, setPreview] = useState<LinkPreview | null>(null);
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  const body = () =>
    choice === 'new' ? { new_space: { key, name } } : { published_space_id: target === '' ? null : target };
  const ask = useMutation({
    mutationFn: () => api.post<LinkPreview>(`/spaces/${space.id}/publishing/link-preview`, body()),
    onSuccess: setPreview,
  });
  const link = useMutation({
    mutationFn: () => api.post(`/spaces/${space.id}/publishing/link`, body()),
    onSuccess: async () => {
      await onLinked();
      onClose();
    },
  });
  const candidates = (spaces.data?.spaces ?? []).filter(
    (s) => s.id !== space.id && !s.pair_role && s.kind !== 'personal',
  );
  const change = (fn: () => void) => {
    fn();
    setPreview(null);
  };
  return (
    <Dialog title={t('publishing.linkTitle')} onClose={onClose}>
      <ErrorNote error={ask.error ?? link.error} />
      <fieldset>
        <legend className="visually-hidden">{t('publishing.linkTitle')}</legend>
        <label className="check">
          <input
            type="radio"
            name="choice"
            checked={choice === 'new'}
            onChange={() => change(() => setChoice('new'))}
          />{' '}
          {t('publishing.makeNew')}
        </label>
        <label className="check">
          <input
            type="radio"
            name="choice"
            checked={choice === 'existing'}
            onChange={() => change(() => setChoice('existing'))}
          />{' '}
          {t('publishing.useExisting')}
        </label>
      </fieldset>
      {choice === 'new' ? (
        <>
          <Field label={t('publishing.newName')}>
            <input value={name} onChange={(e) => change(() => setName(e.target.value))} required />
          </Field>
          <Field label={t('publishing.newKey')}>
            <input value={key} onChange={(e) => change(() => setKey(e.target.value))} required />
          </Field>
        </>
      ) : (
        <Field label={t('publishing.existing')}>
          <select value={target} onChange={(e) => change(() => setTarget(e.target.value))}>
            <option value="">{t('publishing.chooseSpace')}</option>
            {candidates.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </Field>
      )}
      {preview && <LinkPreviewWords preview={preview} />}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        {preview ? (
          <button type="button" className="primary" disabled={link.isPending} onClick={() => link.mutate()}>
            {t('publishing.confirmLink')}
          </button>
        ) : (
          <button type="button" className="primary" disabled={ask.isPending} onClick={() => ask.mutate()}>
            {t('publishing.preview')}
          </button>
        )}
      </div>
    </Dialog>
  );
}

function LinkPreviewWords({ preview }: { preview: LinkPreview }) {
  const { t } = useTranslation();
  const losing = preview.losing_editors;
  return (
    <div role="status" className="note">
      <p>{t('publishing.previewManaged')}</p>
      {preview.new_space && <p>{t('publishing.previewNew')}</p>}
      {losing.everyone && <p>{t('publishing.previewLosingEveryone')}</p>}
      {(losing.people > 0 || losing.groups > 0) && (
        <p>{t('publishing.previewLosing', { people: losing.people, groups: losing.groups })}</p>
      )}
      {(losing.names ?? []).length > 0 && (
        <p className="muted">
          {t('publishing.previewNames', { names: (losing.names ?? []).slice(0, NAMES_SHOWN).join(', ') })}
        </p>
      )}
      {preview.drafts_readable && <p>{t('publishing.previewReadable')}</p>}
      {preview.restores > 0 && <p>{t('publishing.restores', { count: preview.restores })}</p>}
      {preview.edited_by_hand > 0 && <p>{t('publishing.editedByHand', { count: preview.edited_by_hand })}</p>}
      {(preview.sites > 0 || preview.links > 0) && (
        <p>{t('publishing.previewSites', { sites: preview.sites, links: preview.links })}</p>
      )}
      <p>{t('publishing.previewNoRelease')}</p>
    </div>
  );
}

/** The pages of other spaces that link into this draft space, as far as the person may see
 * them; a release never changes them (PUB-522). */
function LinkingIn({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['linking-in', space.id],
    queryFn: () =>
      api.get<{ pages: { page_id: string; title: string; space_name: string }[] }>(
        `/spaces/${space.id}/publishing/linking-in`,
      ),
  });
  return (
    <>
      <h2>{t('publishing.linkingIn')}</h2>
      <p className="muted">{t('publishing.linkingInHelp')}</p>
      <ErrorNote error={q.error} />
      {q.data && q.data.pages.length === 0 && <p className="muted">{t('publishing.linkingInNone')}</p>}
      {q.data && q.data.pages.length > 0 && (
        <ul className="plain-list">
          {q.data.pages.map((p) => (
            <li key={p.page_id}>
              <Link href={`/pages/${p.page_id}`}>{pageTitle(t, p.title)}</Link>{' '}
              <span className="muted">· {p.space_name}</span>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
