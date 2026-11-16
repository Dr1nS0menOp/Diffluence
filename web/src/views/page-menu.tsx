// SPDX-License-Identifier: Apache-2.0
// More on a page (spec 079, PAG-148 to PAG-151): four groups divided by lines and without
// headings, an icon and words on every row, sub-menus for what has choices, and no entry in
// the danger colour, because the dialog that asks first carries that (PAG-149).
import { Fragment, useEffect, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageDetail, type ReleaseStatus } from '../api';
import { offeredNames, useCapabilities } from '../capabilities';
import { mayCreate } from '../create-permission';
import { commentsKey } from '../components/comments/thread';
import {
  IconArchive,
  IconAttach,
  IconClock,
  IconComment,
  IconCopy,
  IconDownload,
  IconEye,
  IconEyeFilled,
  IconFolder,
  IconInfo,
  IconMore,
  IconMove,
  IconPage,
  IconPencil,
  IconPresent,
  IconPrint,
  IconReader,
  IconRename,
  IconStar,
  IconStarFilled,
  IconTrash,
  IconVerified,
} from '../components/icons';
import { Menu, MenuItem, MenuLink, MenuSeparator, MenuSub } from '../components/menu';
import { useShortcut } from '../components/shortcuts';
import { useToast } from '../components/toast';

const MENU_ICON_SIZE = 18;

/** The dialogs a page opens; `export` carries the format chosen in the sub-menu. */
export type PageDialog =
  | 'attachments'
  | 'export'
  | 'public-link'
  | 'restrictions'
  | 'move'
  | 'rename'
  | 'copy'
  | 'delete'
  | 'share'
  | 'approvals'
  | 'verify'
  | 'archive'
  | 'template'
  | 'folder'
  | 'page'
  | 'post'
  | 'release'
  | 'leave_out'
  | 'take_off';

export type WatchScope = 'page' | 'branch' | 'space';
const WATCH_SCOPES: WatchScope[] = ['page', 'branch', 'space'];

type WatchState = Record<WatchScope, boolean>;

/** Whether you watch a page, its branch or its space, with `w` as the way to change the page. */
export function usePageWatch(pageId: string, spaceId: string | undefined) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const state = useQuery({
    queryKey: ['watch', pageId],
    queryFn: () => api.get<WatchState>(`/pages/${pageId}/watch`),
  });
  const set = useMutation({
    mutationFn: (v: { scope: WatchScope; on: boolean }) => {
      const path =
        v.scope === 'space' ? `/spaces/${spaceId}/watch` : `/pages/${pageId}/watch?scope=${v.scope}`;
      return v.on ? api.put(path) : api.del(path);
    },
    onSuccess: (_, v) => toast({ message: v.on ? t('watching.started') : t('watching.stopped') }),
    onError: (e) => toast({ message: e.message }),
    onSettled: () => client.invalidateQueries({ queryKey: ['watch', pageId] }),
  });
  useShortcut('watch', () => set.mutate({ scope: 'page', on: !state.data?.page }));
  return {
    watching: state.data,
    watch: (scope: WatchScope, on: boolean) => set.mutate({ scope, on }),
  };
}

/** Sits in the open menu: the server starts a watch a moment after a publish. */
function FreshWatch({ pageId }: { pageId: string }) {
  const client = useQueryClient();
  useEffect(() => {
    void client.invalidateQueries({ queryKey: ['watch', pageId] });
  }, [client, pageId]);
  return null;
}

function Entry({ icon, onClick, children }: { icon: ReactNode; onClick: () => void; children: ReactNode }) {
  return (
    <MenuItem onClick={onClick}>
      {icon} {children}
    </MenuItem>
  );
}

/** Watch: three choices, each on or off, and a filled eye while any is on (PAG-151). */
function WatchSub({
  watching,
  onWatch,
}: {
  watching: WatchState | undefined;
  onWatch: (scope: WatchScope, on: boolean) => void;
}) {
  const { t } = useTranslation();
  const any = WATCH_SCOPES.some((scope) => watching?.[scope]);
  return (
    <MenuSub
      label={t('watching.watch')}
      icon={any ? <IconEyeFilled size={MENU_ICON_SIZE} /> : <IconEye size={MENU_ICON_SIZE} />}
    >
      {WATCH_SCOPES.map((scope) => (
        <MenuItem
          key={scope}
          role="menuitemcheckbox"
          aria-checked={watching?.[scope] ?? false}
          onClick={() => onWatch(scope, !watching?.[scope])}
        >
          {t(`watching.choice_${scope}`)}
        </MenuItem>
      ))}
    </MenuSub>
  );
}

/** Rules someone can start by hand, with their names, in a sub-menu of More. */
function ManualRules({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const toast = useToast();
  const q = useQuery({
    queryKey: ['page-rules', pageId],
    queryFn: () => api.get<{ rules: { id: string; name: string }[] }>(`/pages/${pageId}/rules`),
  });
  const run = useMutation({
    mutationFn: (id: string) => api.post(`/rules/${id}/run`, { page_id: pageId }),
    onSuccess: () => toast({ message: t('rules2.started') }),
  });
  if (!q.data?.rules.length) return null;
  return (
    <MenuSub label={t('rules2.runRule')} icon={<IconPage size={MENU_ICON_SIZE} />}>
      {q.data.rules.map((rule) => (
        <MenuItem key={rule.id} onClick={() => run.mutate(rule.id)}>
          {rule.name}
        </MenuItem>
      ))}
    </MenuSub>
  );
}

/** Export: one entry per format the instance offers, each opening the dialog with it chosen. */
function ExportSub({ onExport }: { onExport: (format: string) => void }) {
  const { t } = useTranslation();
  const formats = useQuery({
    queryKey: ['export-formats'],
    queryFn: () => api.get<{ formats: string[] }>('/export-formats'),
  });
  // A format whose helper is off or does not answer is not offered (ARC-071).
  const offered = offeredNames(
    (formats.data?.formats ?? []).filter((f) => f !== 'bundle'),
    useCapabilities(),
  );
  return (
    <MenuSub label={t('exchange.export')} icon={<IconDownload size={MENU_ICON_SIZE} />}>
      {offered.map((format) => (
        <MenuItem key={format} onClick={() => onExport(format)}>
          {t(`exchange.format_${format}`)}
        </MenuItem>
      ))}
    </MenuSub>
  );
}

/** Puts a line between groups that have entries, and leaves out a group that has none. */
function Groups({ groups }: { groups: ReactNode[][] }) {
  const shown = groups.filter((group) => group.some(Boolean));
  return (
    <>
      {shown.map((group, at) => (
        <Fragment key={at}>
          {at > 0 && <MenuSeparator />}
          {group}
        </Fragment>
      ))}
    </>
  );
}

export function PageMenu({
  page,
  watching,
  release,
  onOpen,
  onExport,
  onDetails,
  onReader,
  onPresent,
  onComments,
  onWatch,
  onFavourite,
}: {
  page: PageDetail;
  watching: WatchState | undefined;
  release: ReleaseStatus | undefined;
  onOpen: (dialog: PageDialog) => void;
  onExport: (format: string) => void;
  onDetails: () => void;
  onReader: () => void;
  onPresent: () => void;
  onComments: () => void;
  onWatch: (scope: WatchScope, on: boolean) => void;
  onFavourite: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const canEdit = page.can['page.edit'] === true;
  const published = page.current_version > 0;
  const ask = useMutation({
    mutationFn: () => api.post(`/pages/${page.id}/access-requests`, { kind: 'edit' }),
    onSuccess: () => toast({ message: t('share.asked') }),
    onError: (e) => toast({ message: e.message }),
  });
  const closeComments = useMutation({
    mutationFn: () => api.patch(`/pages/${page.id}/comments-closed`, { closed: !page.comments_closed }),
    onError: (e) => toast({ message: e.message }),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      await client.invalidateQueries({ queryKey: commentsKey(page.id) });
    },
  });
  const icon = (Icon: (props: { size?: number }) => ReactNode) => <Icon size={MENU_ICON_SIZE} />;
  const kinds = convertibleKinds(page, canEdit, published);
  const mayArchive = page.can['page.archive'] === true && !page.archived_at;
  const mayDelete = page.can['page.delete'] === true;

  const viewing: ReactNode[] = [
    <MenuItem key="favourite" role="menuitemcheckbox" aria-checked={page.favourite} onClick={onFavourite}>
      {page.favourite ? icon(IconStarFilled) : icon(IconStar)} {t('page.favouriteEntry')}
    </MenuItem>,
    <WatchSub key="watch" watching={watching} onWatch={onWatch} />,
    <Entry key="present" icon={icon(IconPresent)} onClick={onPresent}>
      {t('page.present')}
    </Entry>,
    <Entry key="reader" icon={icon(IconReader)} onClick={onReader}>
      {t('page.readerMode')}
    </Entry>,
    <Entry key="print" icon={icon(IconPrint)} onClick={() => window.print()}>
      {t('page.print')}
    </Entry>,
  ];
  const about: ReactNode[] = [
    <MenuLink key="history" href={`/pages/${page.id}/history`}>
      {icon(IconClock)} {t('page.history')}
    </MenuLink>,
    <MenuSub key="details" label={t('page.moreDetails')} icon={icon(IconInfo)}>
      <MenuItem onClick={onDetails}>{t('page.information')}</MenuItem>
      <MenuItem onClick={() => onOpen('attachments')}>
        {icon(IconAttach)} {t('attachments.title')}
      </MenuItem>
    </MenuSub>,
    published && (
      <Entry key="comments" icon={icon(IconComment)} onClick={onComments}>
        {t('comments.title')}
      </Entry>
    ),
    <Entry key="approvals" icon={icon(IconPage)} onClick={() => onOpen('approvals')}>
      {t('organise.approvals')}
    </Entry>,
    page.can['page.verify'] && !page.verified && (
      <Entry key="verify" icon={icon(IconVerified)} onClick={() => onOpen('verify')}>
        {t('organise.verify')}
      </Entry>
    ),
    !canEdit && published && (
      <Entry key="ask" icon={icon(IconPencil)} onClick={() => ask.mutate()}>
        {t('share.askToEdit')}
      </Entry>
    ),
    canEdit && published && (
      <Entry key="close-comments" icon={icon(IconComment)} onClick={() => closeComments.mutate()}>
        {page.comments_closed ? t('comments.openAgain') : t('comments.closePage')}
      </Entry>
    ),
    <ManualRules key="rules" pageId={page.id} />,
  ];
  const organising: ReactNode[] = [
    canEdit && (
      <Entry key="rename" icon={icon(IconRename)} onClick={() => onOpen('rename')}>
        {t('page.rename')}
      </Entry>
    ),
    mayCreate(page.can, page.kind) && published && (
      <Entry key="copy" icon={icon(IconCopy)} onClick={() => onOpen('copy')}>
        {t('page.copy')}
      </Entry>
    ),
    canEdit && (
      <Entry key="move" icon={icon(IconMove)} onClick={() => onOpen('move')}>
        {t('page.move')}
      </Entry>
    ),
    kinds.length > 0 && (
      <MenuSub key="convert" label={t('organise.convert')} icon={icon(IconFolder)}>
        {kinds.map((kind) => (
          <MenuItem key={kind} onClick={() => onOpen(kind)}>
            {t(`organise.convertTo_${kind}`)}
          </MenuItem>
        ))}
      </MenuSub>
    ),
    page.can['space.templates'] && published && (
      <Entry key="template" icon={icon(IconPage)} onClick={() => onOpen('template')}>
        {t('organise.saveAsTemplate')}
      </Entry>
    ),
    page.can['space.export'] && published && <ExportSub key="export" onExport={onExport} />,
    ...releaseEntries(release, page.id, onOpen, t),
    (mayArchive || mayDelete) && (
      <MenuSub key="remove" label={t('page.archiveOrDelete')} icon={icon(IconTrash)}>
        {mayArchive && (
          <MenuItem onClick={() => onOpen('archive')}>
            {icon(IconArchive)} {t('organise.archivePage')}
          </MenuItem>
        )}
        {mayDelete && (
          <MenuItem onClick={() => onOpen('delete')}>
            {icon(IconTrash)} {t('page.delete')}
          </MenuItem>
        )}
      </MenuSub>
    ),
  ];
  return (
    <Menu label={t('page.moreActions')} trigger={<IconMore />}>
      <FreshWatch pageId={page.id} />
      <Groups groups={[viewing, about, organising]} />
    </Menu>
  );
}

/** The kinds this page may change into, as Convert offers them (PAG-131). */
function convertibleKinds(
  page: PageDetail,
  canEdit: boolean,
  published: boolean,
): ('folder' | 'page' | 'post')[] {
  if (!canEdit) return [];
  if (page.kind === 'page') {
    return !published && mayCreate(page.can, 'post') ? ['folder', 'post'] : ['folder'];
  }
  if (page.kind === 'folder') return ['page'];
  if (page.kind === 'post' && !published && mayCreate(page.can, 'page')) return ['page'];
  return [];
}

/** The steps of a paired draft space (spec 073), where the person may release. */
function releaseEntries(
  release: ReleaseStatus | undefined,
  pageId: string,
  onOpen: (dialog: PageDialog) => void,
  t: (key: string) => string,
): ReactNode[] {
  if (!release?.can_release) return [];
  const icon = <IconPage size={MENU_ICON_SIZE} />;
  return [
    !release.left_out && (
      <Entry key="release" icon={icon} onClick={() => onOpen('release')}>
        {t('publishing.release')}
      </Entry>
    ),
    <Entry key="leave-out" icon={icon} onClick={() => onOpen('leave_out')}>
      {release.left_out?.page_id === pageId ? t('publishing.include') : t('publishing.leaveOut')}
    </Entry>,
    release.twin_page_id && (
      <Entry key="take-off" icon={icon} onClick={() => onOpen('take_off')}>
        {t('publishing.takeOff')}
      </Entry>
    ),
  ];
}
