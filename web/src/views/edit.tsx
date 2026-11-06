// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useRef, useState, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import type { EditorView } from '@tiptap/pm/view';
import {
  api,
  ApiError,
  type CommentAnchor,
  type Draft,
  type Json,
  type PageDetail,
  type PageRef,
  type Space,
} from '../api';
import { Link, navigate } from '../router';
import { anchorOfSelection } from '../components/comments/anchor';
import { NewThread } from '../components/comments/selection-bar';
import { DocumentEditor } from '../components/editor/editor';
import { IconArrowBack, IconChevronDown } from '../components/icons';
import { PageHeader, usePageLook } from '../components/page-header';
import { LabelBar } from '../components/page-labels';
import { PreviewDialog, PublishOptionsDialog, type PublishChoices } from '../components/publish-options';
import { useDelayed } from '../components/quick-search';
import { ShareDialog } from '../components/share-dialog';
import { TemplateGallery } from '../components/template-gallery';
import { useToast } from '../components/toast';
import { ConfirmButton, Dialog, ErrorNote, Loading, RelativeTime } from '../components/ui';
import { MoveDialog } from './move-dialogs';

const AUTOSAVE_MS = 1500;
const EDITORS_CHECK_MS = 60_000;
const BACKUP_KEY_PREFIX = 'diffluence.backup.';

interface Loaded {
  page: PageDetail;
  title: string;
  document: Json;
  base: number;
}

interface Backup {
  title: string;
  document: Json;
  at: string;
}

/** What the browser kept of a text that never reached the server. */
export function readBackup(stored: string | null): Backup | null {
  if (!stored) return null;
  try {
    const value = JSON.parse(stored) as Partial<Backup>;
    if (typeof value.title !== 'string' || typeof value.at !== 'string' || !value.document) return null;
    return { title: value.title, document: value.document, at: value.at };
  } catch {
    // Something else wrote here: there is no text to offer back.
    return null;
  }
}

async function load(id: string): Promise<Loaded> {
  const page = await api.get<PageDetail>(`/pages/${id}`);
  if (page.has_draft) {
    const d = await api.get<Draft>(`/pages/${id}/draft`);
    return { page, title: d.title || page.title, document: d.document, base: d.base_version };
  }
  return { page, title: page.title, document: page.document, base: page.current_version };
}

function isEmptyDocument(document: Json): boolean {
  const blocks = (document.content ?? []) as { content?: unknown[] }[];
  return blocks.every((block) => !block.content || block.content.length === 0);
}

function EditorNotes({ page, title }: { page: PageDetail; title: string }) {
  const { t } = useTranslation();
  const wanted = useDelayed(title.trim());
  const others = useQuery({
    queryKey: ['editors', page.id],
    queryFn: () => api.get<{ editors: { name: string; at: string }[] }>(`/pages/${page.id}/editors`),
    refetchInterval: EDITORS_CHECK_MS,
  });
  const same = useQuery({
    queryKey: ['same-title', page.space_id, wanted, page.id],
    queryFn: () => {
      const params = new URLSearchParams({ space_id: page.space_id, title: wanted, except: page.id });
      return api.get<{ page: PageRef | null }>(`/same-title?${params.toString()}`);
    },
    enabled: wanted !== '',
  });
  const names = others.data?.editors.map((e) => e.name).join(', ');
  return (
    <>
      {names && (
        <p role="status" className="note">
          {t('editor.othersEditing', { names })}
        </p>
      )}
      {same.data?.page && (
        <p role="status" className="note">
          {t('editor.sameTitle')} <Link href={`/pages/${same.data.page.id}`}>{same.data.page.title}</Link>
        </p>
      )}
    </>
  );
}

function BackupNote({
  backup,
  onRestore,
  onDiscard,
}: {
  backup: Backup;
  onRestore: () => void;
  onDiscard: () => void;
}) {
  const { t } = useTranslation();
  return (
    <p role="status" className="note">
      {t('editor.backupFound')} <RelativeTime value={backup.at} />{' '}
      <button type="button" className="ghost" onClick={onRestore}>
        {t('editor.backupRestore')}
      </button>
      <button type="button" className="ghost" onClick={onDiscard}>
        {t('editor.backupDiscard')}
      </button>
    </p>
  );
}

function TemplateOffer({ page }: { page: PageDetail }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const space = useQuery({
    queryKey: ['space-id', page.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.space_id}`),
  });
  return (
    <p className="muted">
      {t('editor.emptyPage')}{' '}
      <button type="button" className="ghost" onClick={() => setOpen(true)}>
        {t('organise.fromTemplate')}
      </button>
      {open && space.data && (
        <TemplateGallery
          space={space.data}
          parent={page.parent_id}
          replaces={page.id}
          onClose={() => setOpen(false)}
        />
      )}
    </p>
  );
}

function Form({ data }: { data: Loaded }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const { page } = data;
  const look = usePageLook(page);
  const backupKey = `${BACKUP_KEY_PREFIX}${page.id}`;
  const [title, setTitle] = useState(data.title);
  const [summary, setSummary] = useState('');
  const [state, setState] = useState<'idle' | 'saving' | 'saved' | 'failed'>('idle');
  const [error, setError] = useState<unknown>(null);
  const [conflict, setConflict] = useState(false);
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [moving, setMoving] = useState(false);
  const [closeFailed, setCloseFailed] = useState(false);
  const [choices, setChoices] = useState<PublishChoices>({ silent: false, stage: null });
  const [backup, setBackup] = useState(() => readBackup(localStorage.getItem(backupKey)));
  const [start, setStart] = useState({ document: data.document, generation: 0 });
  const [typing, setTyping] = useState(false);
  const [commenting, setCommenting] = useState<CommentAnchor | null>(null);
  const [sharing, setSharing] = useState(false);
  const editorView = useRef<EditorView | null>(null);
  const keepView = useCallback((view: EditorView) => {
    editorView.current = view;
  }, []);
  const doc = useRef(data.document);
  const titleRef = useRef(title);
  titleRef.current = title;
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const dirty = useRef(false);

  const keepInBrowser = useCallback(() => {
    const kept: Backup = { title: titleRef.current, document: doc.current, at: new Date().toISOString() };
    localStorage.setItem(backupKey, JSON.stringify(kept));
  }, [backupKey]);

  const save = useCallback(async () => {
    if (!dirty.current) return true;
    dirty.current = false;
    setState('saving');
    keepInBrowser();
    try {
      await api.put(`/pages/${page.id}/draft`, {
        title: titleRef.current,
        document: doc.current,
        base_version: data.base,
      });
      localStorage.removeItem(backupKey);
      setState('saved');
      return true;
    } catch {
      // The text stays in the browser, and the status line says the save failed.
      dirty.current = true;
      setState('failed');
      return false;
    }
  }, [page.id, data.base, backupKey, keepInBrowser]);

  const schedule = useCallback(() => {
    dirty.current = true;
    setTyping(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => void save(), AUTOSAVE_MS);
  }, [save]);

  // Leaving is one click away in the tree, so what was typed last must not depend on a
  // timer. If saving fails, the text stays in the browser and a message leads back to it.
  const saveOnLeaving = useRef(() => undefined as void);
  saveOnLeaving.current = () => {
    clearTimeout(timer.current);
    if (!dirty.current) return;
    dirty.current = false;
    keepInBrowser();
    const draft = { title: titleRef.current, document: doc.current, base_version: data.base };
    api.put(`/pages/${page.id}/draft`, draft).then(
      () => localStorage.removeItem(backupKey),
      () =>
        toast({
          message: t('editor.notSavedOnLeaving'),
          action: { label: t('editor.backToEditor'), run: () => navigate(`/pages/${page.id}/edit`) },
        }),
    );
  };
  useEffect(() => () => saveOnLeaving.current(), []);

  useEffect(() => {
    const keep = () => {
      if (dirty.current) keepInBrowser();
    };
    window.addEventListener('pagehide', keep);
    return () => window.removeEventListener('pagehide', keep);
  }, [keepInBrowser]);

  const finish = async () => {
    await client.invalidateQueries({ queryKey: ['page', page.id] });
    await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    await client.invalidateQueries({ queryKey: ['versions', page.id] });
    navigate(`/pages/${page.id}`);
  };

  const startComment = () => {
    const view = editorView.current;
    const anchor = view && anchorOfSelection(view, page.current_version);
    if (anchor) setCommenting(anchor);
    else toast({ message: t('comments.selectFirst') });
  };

  const publish = async (overwrite: boolean) => {
    clearTimeout(timer.current);
    dirty.current = false;
    setError(null);
    try {
      await api.post(`/pages/${page.id}/publish`, {
        title,
        document: doc.current,
        base_version: data.base,
        summary,
        overwrite,
        silent: choices.silent,
        ...(choices.stage ? { stage: choices.stage } : {}),
      });
      localStorage.removeItem(backupKey);
      await finish();
    } catch (e) {
      if (e instanceof ApiError && e.code === 'version_conflict') {
        dirty.current = true;
        await save();
        setConflict(true);
      } else {
        dirty.current = true;
        setError(e);
      }
    }
  };

  const discard = async () => {
    clearTimeout(timer.current);
    dirty.current = false;
    localStorage.removeItem(backupKey);
    await api.del(`/pages/${page.id}/draft`);
    await finish();
  };

  /** Close waits for the save, or asks when the save failed. */
  const close = async (e: MouseEvent) => {
    e.preventDefault();
    clearTimeout(timer.current);
    if (await save()) await finish();
    else setCloseFailed(true);
  };

  const restoreBackup = (kept: Backup) => {
    doc.current = kept.document;
    setTitle(kept.title);
    setStart({ document: kept.document, generation: start.generation + 1 });
    setBackup(null);
    schedule();
  };

  const published = page.status === 'published';
  const changed = published && (page.has_draft || state !== 'idle');
  const showOffer = !published && !typing && isEmptyDocument(data.document) && page.kind === 'page';
  return (
    <>
      <div className="edit-bar">
        <Link className="button quiet edit-close" href={`/pages/${page.id}`} onClick={(e) => void close(e)}>
          <IconArrowBack size={18} /> {t('editor.close')}
        </Link>
        {changed && <span className="lozenge">{t('editor.unpublishedChanges')}</span>}
        <span role="status" className="muted edit-status">
          {state === 'saving' && t('editor.saving')}
          {state === 'saved' && t('editor.saved')}
          {state === 'failed' && t('editor.saveFailed')}
        </span>
        <input
          className="edit-summary"
          value={summary}
          onChange={(e) => setSummary(e.target.value)}
          maxLength={500}
          aria-label={t('editor.summary')}
          placeholder={t('editor.summary')}
        />
        {page.status === 'published' && (
          <button
            type="button"
            className="quiet"
            onMouseDown={(e) => e.preventDefault()}
            onClick={startComment}
          >
            {t('comments.comment')}
          </button>
        )}
        <ConfirmButton
          label={published ? t('editor.revert') : t('editor.discard')}
          message={published ? t('editor.confirmRevert') : t('editor.confirmDiscard')}
          className="quiet"
          onConfirm={() => void discard()}
        />
        <span className="publish-group">
          <button type="button" className="primary" onClick={() => void publish(false)}>
            {t('editor.publish')}
          </button>
          <button
            type="button"
            className="primary publish-more"
            aria-label={t('organise.publishOptions')}
            title={t('organise.publishOptions')}
            onClick={() => setOptionsOpen(true)}
          >
            <IconChevronDown size={16} />
          </button>
        </span>
      </div>
      <ErrorNote error={error} />
      {backup && (
        <BackupNote
          backup={backup}
          onRestore={() => restoreBackup(backup)}
          onDiscard={() => {
            localStorage.removeItem(backupKey);
            setBackup(null);
          }}
        />
      )}
      <EditorNotes page={page} title={title} />
      {showOffer && <TemplateOffer page={page} />}
      <DocumentEditor
        key={start.generation}
        pageId={page.id}
        initial={start.document}
        canAttach={page.can['attachment.add'] ?? false}
        fullWidth={look.look.full_width}
        onFullWidth={() => look.change({ full_width: !look.look.full_width })}
        onChange={(d) => {
          doc.current = d;
          schedule();
        }}
        onView={keepView}
        onShare={() => setSharing(true)}
        onPublish={() => void publish(false)}
        onSave={() => {
          dirty.current = true;
          void save();
        }}
        header={
          <PageHeader
            page={page}
            look={look}
            title={
              <input
                className="title-input"
                value={title}
                aria-label={t('page.title')}
                placeholder={t('editor.titlePlaceholder')}
                onChange={(e) => {
                  setTitle(e.target.value);
                  schedule();
                }}
                required
              />
            }
          />
        }
        footer={<LabelBar page={page} />}
      />
      {optionsOpen && (
        <PublishOptionsDialog
          page={page}
          choices={choices}
          onChange={setChoices}
          onPreview={() => setPreviewing(true)}
          onMove={() => setMoving(true)}
          onClose={() => setOptionsOpen(false)}
        />
      )}
      {previewing && <PreviewDialog document={doc.current} onClose={() => setPreviewing(false)} />}
      {moving && <MoveDialog page={page} onClose={() => setMoving(false)} />}
      {sharing && <ShareDialog page={page} onClose={() => setSharing(false)} />}
      {commenting && (
        <NewThread page={page} anchor={commenting} editorsOnly onDone={() => setCommenting(null)} />
      )}
      {closeFailed && (
        <Dialog title={t('editor.closeFailedTitle')} onClose={() => setCloseFailed(false)}>
          <p>{t('editor.closeFailedText')}</p>
          <div className="dialog-actions">
            <button type="button" className="quiet" onClick={() => void finish()}>
              {t('editor.closeAnyway')}
            </button>
            <button type="button" className="primary" onClick={() => setCloseFailed(false)}>
              {t('editor.keepEditing')}
            </button>
          </div>
        </Dialog>
      )}
      {conflict && (
        <Dialog title={t('editor.conflictTitle')} onClose={() => setConflict(false)}>
          <p>{t('editor.conflictText')}</p>
          <div className="dialog-actions">
            <Link className="button" href={`/pages/${page.id}/history`}>
              {t('editor.compare')}
            </Link>
            <button type="button" className="danger" onClick={() => void publish(true)}>
              {t('editor.overwrite')}
            </button>
            <button type="button" className="primary" onClick={() => setConflict(false)}>
              {t('editor.keepEditing')}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}

export function EditView({ pageId }: { pageId: string }) {
  // Loaded once per visit: a refetch must not replace what the user is typing.
  const q = useQuery({
    queryKey: ['edit', pageId],
    queryFn: () => load(pageId),
    staleTime: Infinity,
    gcTime: 0,
    refetchOnMount: 'always',
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return <Form key={pageId} data={q.data} />;
}
