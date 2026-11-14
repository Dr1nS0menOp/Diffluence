// SPDX-License-Identifier: Apache-2.0
// Space settings, group Content, section "Published space" (spec 073, PUB-1200): in a space
// in no pair, what a pair is and the way to link one, with its preview; in a draft space the
// settings of the pair, who may release and its health; in a published space where it comes
// from. Ending a pair stays here for the administrators of both spaces.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type LinkPreview, type PairSettings, type PublishingState, type Space } from '../api';
import { Link } from '../router';
import { ConfirmButton, Dialog, ErrorNote, Field, Loading, RelativeTime } from '../components/ui';

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
          {state.can.link && (
            <button type="button" className="primary" onClick={() => setLinking(true)}>
              {t('publishing.link')}
            </button>
          )}
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
        </div>
      )}
      {linking && <LinkDialog space={space} onClose={() => setLinking(false)} onLinked={refresh} />}
    </section>
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
      {(preview.sites > 0 || preview.links > 0) && (
        <p>{t('publishing.previewSites', { sites: preview.sites, links: preview.links })}</p>
      )}
      <p>{t('publishing.previewNoRelease')}</p>
    </div>
  );
}
