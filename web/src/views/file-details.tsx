// SPDX-License-Identifier: Apache-2.0
// One file up close: its versions, its name and, for text, what it says.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Attachment } from '../api';
import { Dialog, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { useDates } from '../dates';

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
      <ul className="plain-list">
        {q.data?.versions.map((version) => (
          <li key={version.number} className="row">
            <span>{t('attachments.version', { number: version.number })}</span>
            <span className="muted">
              {formatSize(version.size)} · {version.uploaded_by} · {dates.day(version.created_at)}
            </span>
            {version.current && <span className="muted">{t('files.current')}</span>}
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

function Rename({ file, onChanged }: { file: Attachment; onChanged: () => Promise<void> }) {
  const { t } = useTranslation();
  const [filename, setFilename] = useState(file.filename);
  const [comment, setComment] = useState('');
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

const TEXT_FILE = /^text\/|json|xml|yaml/;

function TextPreview({ file }: { file: Attachment }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['file-text', file.id, file.version],
    queryFn: () => api.get<{ text: string; cut: boolean }>(`/attachments/${file.id}/text`),
  });
  return (
    <section aria-label={t('files.preview')}>
      <h3>{t('files.preview')}</h3>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && <pre className="file-text">{q.data.text}</pre>}
      {q.data?.cut && <p className="muted">{t('files.cut')}</p>}
    </section>
  );
}

export function FileDetailsDialog({
  file,
  canChange,
  canRemove,
  onChanged,
  onClose,
}: {
  file: Attachment;
  canChange: boolean;
  canRemove: boolean;
  onChanged: () => Promise<void>;
  onClose: () => void;
}) {
  return (
    <Dialog title={file.filename} onClose={onClose}>
      {canChange && <Rename file={file} onChanged={onChanged} />}
      <Versions file={file} canChange={canChange} canRemove={canRemove} onChanged={onChanged} />
      {TEXT_FILE.test(file.content_type) && <TextPreview file={file} />}
    </Dialog>
  );
}
