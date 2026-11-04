// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Draft, type Json, type PageDetail } from '../api';
import { Link, navigate } from '../router';
import { DocumentEditor } from '../components/editor';
import { Dialog, ErrorNote, Field, Loading } from '../components/ui';

const AUTOSAVE_MS = 1500;

interface Loaded {
  page: PageDetail;
  title: string;
  document: Json;
  base: number;
}

async function load(id: string): Promise<Loaded> {
  const page = await api.get<PageDetail>(`/pages/${id}`);
  if (page.has_draft) {
    const d = await api.get<Draft>(`/pages/${id}/draft`);
    return { page, title: d.title || page.title, document: d.document, base: d.base_version };
  }
  return { page, title: page.title, document: page.document, base: page.current_version };
}

function Form({ data }: { data: Loaded }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { page } = data;
  const [title, setTitle] = useState(data.title);
  const [summary, setSummary] = useState('');
  const [state, setState] = useState<'idle' | 'saving' | 'saved' | 'failed'>('idle');
  const [error, setError] = useState<unknown>(null);
  const [conflict, setConflict] = useState(false);
  const [uploadFailed, setUploadFailed] = useState(false);
  const doc = useRef(data.document);
  const titleRef = useRef(title);
  titleRef.current = title;
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const dirty = useRef(false);

  const save = useCallback(async () => {
    if (!dirty.current) return;
    dirty.current = false;
    setState('saving');
    try {
      await api.put(`/pages/${page.id}/draft`, {
        title: titleRef.current,
        document: doc.current,
        base_version: data.base,
      });
      setState('saved');
    } catch {
      setState('failed');
    }
  }, [page.id, data.base]);

  const schedule = useCallback(() => {
    dirty.current = true;
    clearTimeout(timer.current);
    timer.current = setTimeout(() => void save(), AUTOSAVE_MS);
  }, [save]);

  useEffect(() => () => clearTimeout(timer.current), []);

  const finish = async () => {
    await client.invalidateQueries({ queryKey: ['page', page.id] });
    await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    await client.invalidateQueries({ queryKey: ['versions', page.id] });
    navigate(`/pages/${page.id}`);
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
      });
      await finish();
    } catch (e) {
      if (e instanceof ApiError && e.code === 'version_conflict') {
        dirty.current = true;
        await save();
        setConflict(true);
      } else {
        setError(e);
      }
    }
  };

  const discard = async () => {
    if (!window.confirm(t('editor.confirmDiscard'))) return;
    clearTimeout(timer.current);
    dirty.current = false;
    await api.del(`/pages/${page.id}/draft`);
    await finish();
  };

  return (
    <>
      <Field label={t('page.title')}>
        <input
          className="title-input"
          value={title}
          onChange={(e) => {
            setTitle(e.target.value);
            schedule();
          }}
          required
        />
      </Field>
      <DocumentEditor
        pageId={page.id}
        initial={data.document}
        onChange={(d) => {
          doc.current = d;
          schedule();
        }}
        onUploadError={() => setUploadFailed(true)}
      />
      {uploadFailed && (
        <p role="alert" className="note note-error">
          {t('editor.uploadFailed')}
        </p>
      )}
      <ErrorNote error={error} />
      <Field label={t('editor.summary')}>
        <input value={summary} onChange={(e) => setSummary(e.target.value)} maxLength={500} />
      </Field>
      <div className="row actions">
        <button type="button" className="primary" onClick={() => void publish(false)}>
          {t('editor.publish')}
        </button>
        <button type="button" onClick={() => void discard()}>
          {t('editor.discard')}
        </button>
        <span role="status" className="muted">
          {state === 'saving' && t('editor.saving')}
          {state === 'saved' && t('editor.saved')}
          {state === 'failed' && t('editor.saveFailed')}
        </span>
      </div>
      {conflict && (
        <Dialog title={t('editor.conflictTitle')} onClose={() => setConflict(false)}>
          <p>{t('editor.conflictText')}</p>
          <div className="row">
            <button type="button" className="primary" onClick={() => setConflict(false)}>
              {t('editor.keepEditing')}
            </button>
            <Link className="button" href={`/pages/${page.id}/history`}>
              {t('editor.compare')}
            </Link>
            <button type="button" className="danger" onClick={() => void publish(true)}>
              {t('editor.overwrite')}
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
