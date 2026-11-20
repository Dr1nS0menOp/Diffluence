// SPDX-License-Identifier: Apache-2.0
// The settings of a space, one entry at a time at `?section=` (spec 083). Which entries exist,
// where they sit and who may open them is the registry in space-sections.ts; this file draws the
// entry that is open.
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageRef, type Space } from '../api';
import { useCapabilities } from '../capabilities';
import { Link, navigate, useLocation } from '../router';
import { spaceByKeyQuery } from '../components/frame';
import { Notice } from '../components/notice';
import { SettingsSection } from '../components/settings-section';
import { ShortcutsForm } from '../components/shortcut-links';
import { TableBox } from '../components/table-box';
import { ConfirmDialog, ErrorNote, Loading } from '../components/ui';
import { pageTitle } from '../page-name';
import { SpaceRolesPanel } from './access-roles';
import { ExportSection, ImportSection, PDFDesignSection } from './exchange';
import { FileList, SpaceUsage } from './files';
import { ArchiveList } from './organise';
import { DetailsSection } from './space-details';
import {
  DeleteSpaceDialog,
  FeaturesSection,
  LabelsSection,
  LayoutSection,
  PictureSection,
} from './space-organise';
import { PublishingSection } from './space-publishing';
import { SpaceSettingsFrame } from './space-settings-frame';
import { StatusesAndApprovals } from './space-statuses';
import { WorkflowsSection } from './space-workflows';
import { entriesOpen, entryHref, entryIntroKey, entryNameKey, spaceEntryAt } from './space-sections';

/** Details opens first when the person may open it (D18). */
const LANDING = 'details';

export function SpaceSettingsView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const capabilities = useCapabilities();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  const asked = useLocation().searchParams.get('section') ?? LANDING;
  const space = q.data;
  const entry = spaceEntryAt(asked);
  const open = space ? entriesOpen(space, capabilities) : [];
  // A screen of its own is opened at its own address, so here only the sections count.
  const sections = open.filter((e) => !e.path);
  // Where this address leads instead, replacing it in the history so Back does not loop (D7):
  // an old id to its new place, a screen of its own to its address, and someone who may open
  // only screens of their own to the first of them (D18).
  const elsewhere =
    !space || open.length === 0
      ? null
      : entry?.path && open.includes(entry)
        ? entryHref(space.key, entry)
        : entry && entry.id !== asked
          ? `/spaces/${space.key}/settings?section=${entry.id}`
          : sections.length === 0 && open[0]
            ? entryHref(space.key, open[0])
            : null;
  useEffect(() => {
    if (elsewhere) navigate(elsewhere, true);
  }, [elsewhere]);
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space) return <Loading />;
  const isOpen = (id: string) => open.some((e) => e.id === id);
  const section =
    entry && !entry.path && isOpen(entry.id)
      ? entry
      : (sections.find((e) => e.id === LANDING) ?? sections[0] ?? null);
  // Someone who may open no entry gets what a missing page gives (SPC-134).
  if (open.length === 0) {
    return (
      <main id="main" className="plain reading">
        <p role="alert">{t('app.notFound')}</p>
      </main>
    );
  }
  // Only screens of their own: the effect above is on its way to the first of them.
  if (!section) return <Loading />;
  return (
    <SpaceSettingsFrame space={space} current={section.id}>
      <SettingsSection
        title={t(entryNameKey(section.id))}
        intro={t(entryIntroKey(section.id))}
        doc={section.doc}
        notices={space.status === 'archived' && section.id !== 'archive' && <ArchivedNotice />}
      >
        <SectionContent id={section.id} space={space} />
      </SettingsSection>
    </SpaceSettingsFrame>
  );
}

function ArchivedNotice() {
  const { t } = useTranslation();
  return <Notice kind="info">{t('space.archived')}</Notice>;
}

function SectionContent({ id, space }: { id: string; space: Space }) {
  switch (id) {
    case 'people':
      return <SpaceRolesPanel space={space} />;
    case 'details':
      return <DetailsSection space={space} />;
    case 'features':
      return <FeaturesSection space={space} />;
    case 'archive':
      return <ArchiveSection space={space} />;
    case 'delete':
      return <DeleteSection space={space} />;
    case 'export':
      return <ExportSection space={space} />;
    case 'import':
      return <ImportSection space={space} />;
    case 'picture':
      return <PictureSection space={space} />;
    case 'statuses':
      return <StatusesAndApprovals space={space} />;
    case 'workflows':
      return <WorkflowsSection space={space} />;
    case 'placeholders':
      return <PlaceholdersSection space={space} />;
    case 'files':
      return (
        <>
          <FileList space={space} />
          <SpaceUsage space={space} />
        </>
      );
    case 'archived':
      return <ArchiveList space={space} />;
    case 'labels':
      return <LabelsSection space={space} />;
    case 'shortcuts':
      return <ShortcutsForm space={space} />;
    case 'published':
      return <PublishingSection space={space} />;
    case 'layout':
      return <LayoutSection space={space} />;
    case 'pdf':
      return <PDFDesignSection space={space} />;
    default:
      return null;
  }
}

function useSpaceChange(space: Space) {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['space', space.key] });
    await client.invalidateQueries({ queryKey: ['space-id', space.id] });
    await client.invalidateQueries({ queryKey: ['spaces'] });
    await client.invalidateQueries({ queryKey: ['my-spaces'] });
  };
}

/** Archive the space, or restore it: an action section with one primary button (SPC-129, SPC-130). */
function ArchiveSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const refresh = useSpaceChange(space);
  const [asking, setAsking] = useState(false);
  const change = useMutation({
    mutationFn: (to: 'archive' | 'unarchive') => api.post(`/spaces/${space.id}/${to}`),
    onSuccess: refresh,
  });
  const archived = space.status === 'archived';
  return (
    <section className="action-section">
      <Notice kind="info">{archived ? t('spaceArchive.archived') : t('spaceArchive.info')}</Notice>
      <ErrorNote error={change.error} />
      <div>
        {archived ? (
          <button type="button" className="primary" onClick={() => change.mutate('unarchive')}>
            {t('spaceArchive.restore')}
          </button>
        ) : (
          <button type="button" className="primary" onClick={() => setAsking(true)}>
            {t('spaceArchive.archive')}
          </button>
        )}
      </div>
      {asking && (
        <ConfirmDialog
          title={t('spaceArchive.archive')}
          message={t('space.confirmArchive')}
          confirmLabel={t('spaceArchive.archive')}
          tone="primary"
          onConfirm={() => change.mutate('archive')}
          onClose={() => setAsking(false)}
        />
      )}
    </section>
  );
}

/** Delete the space: the explanation, something milder first, then the danger button (SPC-131). */
function DeleteSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [asking, setAsking] = useState(false);
  const remove = useMutation({
    mutationFn: () => api.del(`/spaces/${space.id}`),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
      navigate('/');
    },
  });
  const base = `/spaces/${space.key}/settings`;
  return (
    <section className="action-section">
      <p>{t('spaceDelete.explain')}</p>
      <p>
        {t('spaceDelete.milder')}{' '}
        <Link href={`${base}?section=archive`}>{t('spaceDelete.archiveInstead')}</Link>
        {', '}
        <Link href={`${base}?section=export`}>{t('spaceDelete.exportFirst')}</Link>.
      </p>
      <ErrorNote error={remove.error} />
      <div>
        <button type="button" className="danger" onClick={() => setAsking(true)}>
          {t('spaceDelete.delete')}
        </button>
      </div>
      {asking && (
        <DeleteSpaceDialog space={space} onDelete={() => remove.mutate()} onClose={() => setAsking(false)} />
      )}
    </section>
  );
}

/** Titles that pages link to and nobody started, as a table that stays when empty (SPC-126, SPC-127). */
function PlaceholdersSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['placeholders-of-space', space.id],
    queryFn: () =>
      api.get<{ placeholders: { title: string; source: PageRef }[] }>(`/spaces/${space.id}/placeholders`),
  });
  const mayCreate = space.can?.['page.create'] ?? false;
  if (!q.data) return q.error ? <ErrorNote error={q.error} /> : <Loading />;
  const rows = q.data.placeholders;
  if (rows.length === 0) return <p className="muted">{t('spacePlaceholders.none')}</p>;
  return (
    <TableBox>
      <table>
        <thead>
          <tr>
            <th>{t('spacePlaceholders.title')}</th>
            <th>{t('spacePlaceholders.linkedFrom')}</th>
            {mayCreate && <th>{t('common.actions')}</th>}
          </tr>
        </thead>
        <tbody>
          {rows.map((one) => (
            <tr key={`${one.source.id}-${one.title}`}>
              <td>{one.title}</td>
              <td>
                <Link href={`/pages/${one.source.id}`}>{pageTitle(t, one.source.title)}</Link>
              </td>
              {mayCreate && (
                <td>
                  <Link
                    className="button"
                    href={`/spaces/${space.key}/new?title=${encodeURIComponent(one.title)}`}
                  >
                    {t('spacePlaceholders.create')}
                  </Link>
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}
