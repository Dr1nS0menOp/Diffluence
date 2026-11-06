// SPDX-License-Identifier: Apache-2.0
// How a page presents itself. It is page data, so it applies at once and does not
// wait for publishing (spec 009, D4).
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Attachment, type PageLook, type PageSummary } from '../api';
import emoji from './editor/emoji.json';
import { LabelBar } from './page-labels';
import { Dialog, ErrorNote, Field, FilePicker } from './ui';

const EMOJI_SHOWN = 40;
const LOOK_FIELDS: (keyof PageLook)[] = [
  'emoji',
  'full_width',
  'title_align',
  'hide_children',
  'header_attachment_id',
  'header_position',
];

function lookOf(page: PageSummary): PageLook {
  return Object.fromEntries(LOOK_FIELDS.map((key) => [key, page[key]])) as unknown as PageLook;
}

function EmojiChoice({ value, onPick }: { value: string; onPick: (emoji: string) => void }) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  return (
    <fieldset className="emoji-choice">
      <legend>{t('look.emoji')}</legend>
      <input
        type="search"
        value={filter}
        aria-label={t('look.emojiFilter')}
        placeholder={t('look.emojiFilter')}
        onChange={(e) => setFilter(e.target.value)}
      />
      <div className="emoji-grid">
        {emoji
          .filter((entry) => entry.words.includes(filter.trim().toLowerCase()))
          .slice(0, EMOJI_SHOWN)
          .map((entry) => (
            <button
              key={entry.glyph}
              type="button"
              className="ghost"
              aria-label={entry.words}
              aria-pressed={entry.glyph === value}
              onClick={() => onPick(entry.glyph)}
            >
              {entry.glyph}
            </button>
          ))}
      </div>
      {value && (
        <button type="button" className="quiet" onClick={() => onPick('')}>
          {t('look.noEmoji')}
        </button>
      )}
    </fieldset>
  );
}

export function PageLookDialog({ page, onClose }: { page: PageSummary; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [look, setLook] = useState(() => lookOf(page));
  const save = useMutation({
    mutationFn: (next: PageLook) => api.patch(`/pages/${page.id}/look`, next),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    },
  });
  const change = (part: Partial<PageLook>) => {
    const next = { ...look, ...part };
    setLook(next);
    save.mutate(next);
  };
  const upload = useMutation({
    mutationFn: (file: File) =>
      api.upload<Attachment>(`/pages/${page.id}/attachments?filename=${encodeURIComponent(file.name)}`, file),
    onSuccess: (attachment) => {
      if (attachment.inline) change({ header_attachment_id: attachment.id });
    },
  });
  return (
    <Dialog title={t('look.title')} onClose={onClose}>
      <ErrorNote error={save.error ?? upload.error} />
      <EmojiChoice value={look.emoji} onPick={(emoji) => change({ emoji })} />
      <label className="check">
        <input
          type="checkbox"
          checked={look.full_width}
          onChange={(e) => change({ full_width: e.target.checked })}
        />
        {t('look.fullWidth')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={!look.hide_children}
          onChange={(e) => change({ hide_children: !e.target.checked })}
        />
        {t('look.showChildren')}
      </label>
      <Field label={t('look.titleAlign')}>
        <select value={look.title_align} onChange={(e) => change({ title_align: e.target.value })}>
          <option value="start">{t('look.alignStart')}</option>
          <option value="center">{t('look.alignCenter')}</option>
        </select>
      </Field>
      <h3>{t('look.header')}</h3>
      <div className="row">
        <FilePicker
          label={look.header_attachment_id ? t('look.changeHeader') : t('look.addHeader')}
          accept="image/png,image/jpeg,image/gif,image/webp"
          onPick={(file) => upload.mutate(file)}
        />
        {look.header_attachment_id && (
          <button type="button" className="quiet" onClick={() => change({ header_attachment_id: null })}>
            {t('look.removeHeader')}
          </button>
        )}
      </div>
      {look.header_attachment_id && (
        <Field label={t('look.headerPosition')} help={t('look.headerPositionHelp')}>
          <input
            type="number"
            min={0}
            max={100}
            value={look.header_position}
            onChange={(e) => change({ header_position: Number(e.target.value) })}
          />
        </Field>
      )}
      <LabelBar page={page} />
    </Dialog>
  );
}
