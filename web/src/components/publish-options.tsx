// SPDX-License-Identifier: Apache-2.0
// What the arrow beside Publish offers (spec 011, D7).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Json, type PageDetail, type Stage } from '../api';
import { DocumentView } from './editor/editor';
import { Dialog, ErrorNote, Field } from './ui';

export interface PublishChoices {
  silent: boolean;
  stage: Stage | null;
}

interface DocNode {
  type?: string;
  content?: DocNode[];
}

/** What a reader will see: instruction text is left out. */
export function forReaders(document: Json): Json {
  const strip = (node: DocNode): DocNode => ({
    ...node,
    ...(node.content ? { content: node.content.filter((c) => c.type !== 'instruction').map(strip) } : {}),
  });
  return strip(document as DocNode) as Json;
}

export function PreviewDialog({ document, onClose }: { document: Json; onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <Dialog title={t('organise.preview')} onClose={onClose} wide>
      <DocumentView document={forReaders(document)} plain />
    </Dialog>
  );
}

export function PublishOptionsDialog({
  page,
  choices,
  onChange,
  onPreview,
  onMove,
  onClose,
}: {
  page: PageDetail;
  choices: PublishChoices;
  onChange: (next: PublishChoices) => void;
  onPreview: () => void;
  onMove: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [at, setAt] = useState('');
  const statuses = useQuery({
    queryKey: ['statuses', page.space_id],
    queryFn: () => api.get<{ statuses: Stage[]; free: boolean }>(`/spaces/${page.space_id}/statuses`),
  });
  const refresh = () => client.invalidateQueries({ queryKey: ['page', page.id] });
  const schedule = useMutation({
    mutationFn: () =>
      api.put(`/pages/${page.id}/schedule`, { at: new Date(at).toISOString(), silent: choices.silent }),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  const cancel = useMutation({ mutationFn: () => api.del(`/pages/${page.id}/schedule`), onSuccess: refresh });
  return (
    <Dialog title={t('organise.publishOptions')} onClose={onClose}>
      <ErrorNote error={schedule.error ?? cancel.error} />
      <label className="check">
        <input
          type="checkbox"
          checked={!choices.silent}
          onChange={(e) => onChange({ ...choices, silent: !e.target.checked })}
        />
        {t('organise.notifyWatchers')}
      </label>
      <Field label={t('organise.status')}>
        <select
          value={choices.stage?.text ?? ''}
          onChange={(e) =>
            onChange({
              ...choices,
              stage: statuses.data?.statuses.find((s) => s.text === e.target.value) ?? null,
            })
          }
        >
          <option value="">{t('organise.keepStatus')}</option>
          {statuses.data?.statuses.map((s) => (
            <option key={s.text} value={s.text}>
              {s.text}
            </option>
          ))}
        </select>
      </Field>
      <Field label={t('organise.publishAt')} help={t('organise.publishAtHelp')}>
        <input type="datetime-local" value={at} onChange={(e) => setAt(e.target.value)} />
      </Field>
      <div className="row">
        <button type="button" onClick={onPreview}>
          {t('organise.preview')}
        </button>
        <button type="button" onClick={onMove}>
          {t('organise.location')}
        </button>
      </div>
      <div className="dialog-actions">
        {page.scheduled && (
          <button type="button" className="quiet" onClick={() => cancel.mutate()}>
            {t('organise.cancelSchedule')}
          </button>
        )}
        <button type="button" disabled={!at} onClick={() => schedule.mutate()}>
          {t('organise.schedule')}
        </button>
        <button type="button" className="primary" onClick={onClose}>
          {t('organise.done')}
        </button>
      </div>
    </Dialog>
  );
}
