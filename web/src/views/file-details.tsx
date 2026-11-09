// SPDX-License-Identifier: Apache-2.0
// One file up close: its versions, its name, its labels and, for text, what it says.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Attachment, type FileRow, type SearchResult } from '../api';
import { OpenInCraft } from '../components/craft-frame';
import { isTextFile, TextPreview } from '../components/file-preview';
import { IconClose } from '../components/icons';
import { useCopy } from '../components/toast';
import { Dialog, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { useCapabilities } from '../capabilities';
import { useDates } from '../dates';

const REMOVE_ICON_SIZE = 12;

interface FileVersion {
  number: number;
  size: number;
  uploaded_by: string;
  created_at: string;
  current: boolean;
}

function Versions({
  file,
  canChange,
  canRemove,
  onChanged,
}: {
  file: Attachment;
  canChange: boolean;
  canRemove: boolean;
  onChanged: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const dates = useDates();
  const copy = useCopy();
  const copyLink = (address: string) =>
    void copy(`${window.location.origin}${address}`, t('files.linkCopied'));
  const latest = `/api/v1/attachments/${file.id}/content?download=1`;
  const key = ['file-versions', file.id];
  const client = useQueryClient();
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ versions: FileVersion[] }>(`/attachments/${file.id}/versions`),
  });
  const settle = async () => {
    await client.invalidateQueries({ queryKey: key });
    await onChanged();
  };
  const makeCurrent = useMutation({
    mutationFn: (number: number) => api.post(`/attachments/${file.id}/versions/${number}/current`),
    onSettled: settle,
  });
  const remove = useMutation({
    mutationFn: (number: number) => api.del(`/attachments/${file.id}/versions/${number}`),
    onSettled: settle,
  });
  return (
    <section aria-label={t('files.versions')}>
      <h3>{t('files.versions')}</h3>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? makeCurrent.error ?? remove.error} />
      <p>
        <button type="button" className="ghost" onClick={() => copyLink(latest)}>
          {t('files.copyLatest')}
        </button>
      </p>
      <ul className="plain-list">
        {q.data?.versions.map((version) => (
          <li key={version.number} className="row">
            <span>{t('attachments.version', { number: version.number })}</span>
            <span className="muted">
              {formatSize(version.size)} · {version.uploaded_by} · {dates.day(version.created_at)}
            </span>
            {version.current && <span className="muted">{t('files.current')}</span>}
            <a href={`/api/v1/attachments/${file.id}/versions/${version.number}/content`}>
              {t('attachments.download')}
            </a>
            <button
              type="button"
              className="ghost"
              onClick={() => copyLink(`/api/v1/attachments/${file.id}/versions/${version.number}/content`)}
            >
              {t('files.copyVersion')}
            </button>
            {!version.current && canChange && (
              <button type="button" className="ghost" onClick={() => makeCurrent.mutate(version.number)}>
                {t('files.makeCurrent')}
              </button>
            )}
            {!version.current && canRemove && (
              <button type="button" className="ghost" onClick={() => remove.mutate(version.number)}>
                {t('common.delete')}
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

function Rename({ file, onChanged }: { file: FileRow; onChanged: () => Promise<void> }) {
  const { t } = useTranslation();
  const [filename, setFilename] = useState(file.filename);
  const [comment, setComment] = useState(file.comment);
  const save = useMutation({
    mutationFn: () => api.patch(`/attachments/${file.id}`, { filename, comment }),
    onSuccess: onChanged,
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  return (
    <form onSubmit={submit}>
      <ErrorNote error={save.error} />
      <Field label={t('files.name')}>
        <input value={filename} onChange={(e) => setFilename(e.target.value)} />
      </Field>
      <Field label={t('files.comment')}>
        <input value={comment} onChange={(e) => setComment(e.target.value)} />
      </Field>
      <button type="submit">{t('common.save')}</button>
    </form>
  );
}

function FileLabels({
  file,
  canChange,
  onChanged,
}: {
  file: FileRow;
  canChange: boolean;
  onChanged: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const address = `/attachments/${file.id}/labels`;
  const add = useMutation({
    mutationFn: () => api.post(address, { label: text }),
    onSuccess: async () => {
      setText('');
      await onChanged();
    },
  });
  const remove = useMutation({
    mutationFn: (label: string) => api.del(`${address}/${encodeURIComponent(label)}`),
    onSuccess: onChanged,
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    add.mutate();
  };
  return (
    <section aria-label={t('files.labels')}>
      <h3>{t('files.labels')}</h3>
      <ErrorNote error={add.error ?? remove.error} />
      {file.labels.length === 0 && !canChange && <p className="muted">{t('files.noLabels')}</p>}
      <ul className="plain-list row">
        {file.labels.map((label) => (
          <li key={label} className="row">
            <span className="lozenge">{label}</span>
            {canChange && (
              <button
                type="button"
                className="icon"
                aria-label={t('files.removeLabel', { label })}
                onClick={() => remove.mutate(label)}
              >
                <IconClose size={REMOVE_ICON_SIZE} />
              </button>
            )}
          </li>
        ))}
      </ul>
      {canChange && (
        <form className="row" onSubmit={submit}>
          <Field label={t('files.addLabel')}>
            <input value={text} onChange={(e) => setText(e.target.value)} />
          </Field>
          <button type="submit" disabled={!text.trim()}>
            {t('files.add')}
          </button>
        </form>
      )}
    </section>
  );
}

const MAX_PAGE_RESULTS = 8;

/** Moving asks more of a person than copying, so the server decides each of them. */
function MoveOrCopy({
  file,
  canMove,
  onChanged,
}: {
  file: Attachment;
  canMove: boolean;
  onChanged: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [target, setTarget] = useState<SearchResult | null>(null);
  const [copied, setCopied] = useState('');
  const pages = useQuery({
    queryKey: ['file-target', search],
    queryFn: () =>
      api.get<{ results: SearchResult[] }>(
        `/search?q=${encodeURIComponent(search)}&limit=${MAX_PAGE_RESULTS}`,
      ),
    enabled: search.trim().length > 0 && !target,
  });
  const send = useMutation({
    mutationFn: (how: 'move' | 'copy') =>
      api.post(`/attachments/${file.id}/${how}`, { page_id: target?.page_id }),
    onSuccess: async (_answer, how) => {
      setCopied(how === 'copy' ? (target?.title ?? '') : '');
      setTarget(null);
      setSearch('');
      await onChanged();
    },
  });
  const results = (pages.data?.results ?? []).filter((result) => result.page_id !== file.page_id);
  return (
    <section aria-label={t('files.moveTitle')}>
      <h3>{t('files.moveTitle')}</h3>
      <ErrorNote error={send.error ?? pages.error} />
      {copied && (
        <p role="status" className="note">
          {t('files.copied', { title: copied })}
        </p>
      )}
      {target ? (
        <div className="row">
          <span>{target.title}</span>
          {canMove && (
            <button type="button" onClick={() => send.mutate('move')}>
              {t('files.move')}
            </button>
          )}
          <button type="button" onClick={() => send.mutate('copy')}>
            {t('files.copy')}
          </button>
          <button type="button" className="ghost" onClick={() => setTarget(null)}>
            {t('common.cancel')}
          </button>
        </div>
      ) : (
        <>
          <Field label={t('files.movePage')}>
            <input value={search} onChange={(e) => setSearch(e.target.value)} />
          </Field>
          <ul className="plain-list">
            {results.map((result) => (
              <li key={result.page_id}>
                <button type="button" className="ghost" onClick={() => setTarget(result)}>
                  {result.title}
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

function OpenAsPDF({ file }: { file: Attachment }) {
  const { t } = useTranslation();
  const capabilities = useCapabilities();
  if (!file.office || !capabilities.office_preview) return null;
  return <a href={`/api/v1/attachments/${file.id}/preview.pdf`}>{t('files.openAsPdf')}</a>;
}

export function FileDetailsDialog({
  file,
  canChange,
  canRemove,
  canMove,
  onChanged,
  onClose,
}: {
  file: FileRow;
  canChange: boolean;
  canRemove: boolean;
  canMove: boolean;
  onChanged: () => Promise<void>;
  onClose: () => void;
}) {
  return (
    <Dialog title={file.filename} onClose={onClose}>
      <OpenAsPDF file={file} />
      <OpenInCraft file={file} address={`/api/v1/attachments/${file.id}/content`} />
      {canChange && <Rename file={file} onChanged={onChanged} />}
      <FileLabels file={file} canChange={canChange} onChanged={onChanged} />
      <Versions file={file} canChange={canChange} canRemove={canRemove} onChanged={onChanged} />
      {isTextFile(file.content_type) && <TextPreview file={file} />}
      <MoveOrCopy file={file} canMove={canMove} onChanged={onChanged} />
    </Dialog>
  );
}
