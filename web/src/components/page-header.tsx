// SPDX-License-Identifier: Apache-2.0
// The title area of the editor: emoji, header image and page options sit at the title,
// quiet until they are needed (spec 017, F5). How a page looks is page data, so it
// applies at once and does not wait for publishing (spec 009, D4).
import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Attachment, type PageLook, type PageSummary } from '../api';
import emoji from './editor/emoji.json';
import { Menu, MenuItem } from './menu';
import { Dialog, ErrorNote, FilePicker } from './ui';

const EMOJI_SHOWN = 40;
const SAVE_DELAY_MS = 400;
const MAX_POSITION = 100;
const IMAGE_TYPES = 'image/png,image/jpeg,image/gif,image/webp';
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

/** Moving the image by this many pixels gives this position, for an image cut to fit its box. */
export function positionAfterDrag(start: number, moved: number, hiddenHeight: number): number {
  if (hiddenHeight <= 0) return start;
  const position = Math.round(start - (moved / hiddenHeight) * MAX_POSITION);
  return Math.min(MAX_POSITION, Math.max(0, position));
}

export function usePageLook(page: PageSummary) {
  const client = useQueryClient();
  const [look, setLook] = useState(() => lookOf(page));
  const latest = useRef(look);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const save = useMutation({
    mutationFn: (next: PageLook) => api.patch(`/pages/${page.id}/look`, next),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', page.id] });
      await client.invalidateQueries({ queryKey: ['tree', page.space_id] });
    },
  });
  const apply = (part: Partial<PageLook>) => {
    latest.current = { ...latest.current, ...part };
    setLook(latest.current);
    clearTimeout(timer.current);
  };
  const flush = useRef(() => undefined as void);
  flush.current = () => {
    if (timer.current === undefined) return;
    clearTimeout(timer.current);
    timer.current = undefined;
    save.mutate(latest.current);
  };
  useEffect(() => () => flush.current(), []);
  return {
    look,
    error: save.error,
    change: (part: Partial<PageLook>) => {
      apply(part);
      timer.current = undefined;
      save.mutate(latest.current);
    },
    /** For what moves in small steps: one drag should be one change in the audit log. */
    changeSoon: (part: Partial<PageLook>) => {
      apply(part);
      timer.current = setTimeout(() => flush.current(), SAVE_DELAY_MS);
    },
  };
}

type Look = ReturnType<typeof usePageLook>;

function EmojiDialog({
  value,
  onPick,
  onClose,
}: {
  value: string;
  onPick: (emoji: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  return (
    <Dialog title={t('look.emoji')} onClose={onClose}>
      <input
        type="search"
        autoFocus
        className="dialog-filter"
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
    </Dialog>
  );
}

function HeaderImage({
  id,
  position,
  onMove,
  onFile,
  onRemove,
}: {
  id: string;
  position: number;
  onMove: (position: number) => void;
  onFile: (file: File) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const image = useRef<HTMLImageElement>(null);
  const drag = useRef<{ from: number; start: number } | null>(null);

  useEffect(() => {
    image.current?.style.setProperty('--header-position', `${position}%`);
  }, [position]);

  const moved = (e: PointerEvent<HTMLImageElement>) => {
    const box = image.current;
    if (!drag.current || !box?.naturalWidth) return;
    const shownHeight = (box.clientWidth * box.naturalHeight) / box.naturalWidth;
    onMove(
      positionAfterDrag(drag.current.start, e.clientY - drag.current.from, shownHeight - box.clientHeight),
    );
  };
  return (
    <div className="header-edit">
      <img
        ref={image}
        className="page-header-image"
        src={`/api/v1/attachments/${id}/content`}
        alt=""
        draggable={false}
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture(e.pointerId);
          drag.current = { from: e.clientY, start: position };
        }}
        onPointerMove={moved}
        onPointerUp={() => {
          drag.current = null;
        }}
      />
      <div className="header-tools">
        <label className="header-position">
          {t('look.headerPosition')}
          <input
            type="range"
            min={0}
            max={MAX_POSITION}
            value={position}
            onChange={(e) => onMove(Number(e.target.value))}
          />
        </label>
        <FilePicker label={t('look.changeHeader')} accept={IMAGE_TYPES} onPick={onFile} />
        <button type="button" onClick={onRemove}>
          {t('look.removeHeader')}
        </button>
      </div>
    </div>
  );
}

/** Wraps the title field, with what belongs to it. */
export function PageHeader({ page, look, title }: { page: PageSummary; look: Look; title: ReactNode }) {
  const { t } = useTranslation();
  const [choosingEmoji, setChoosingEmoji] = useState(false);
  const { look: value, change, changeSoon } = look;
  const upload = useMutation({
    mutationFn: (file: File) =>
      api.upload<Attachment>(`/pages/${page.id}/attachments?filename=${encodeURIComponent(file.name)}`, file),
    onSuccess: (attachment) => {
      if (attachment.inline) change({ header_attachment_id: attachment.id });
    },
  });
  const centred = value.title_align === 'center';
  return (
    <div className="page-head">
      <ErrorNote error={look.error ?? upload.error} />
      {value.header_attachment_id && (
        <HeaderImage
          id={value.header_attachment_id}
          position={value.header_position}
          onMove={(header_position) => changeSoon({ header_position })}
          onFile={(file) => upload.mutate(file)}
          onRemove={() => change({ header_attachment_id: null })}
        />
      )}
      <div className="title-tools">
        {!value.emoji && (
          <button type="button" className="quiet" onClick={() => setChoosingEmoji(true)}>
            {t('look.addEmoji')}
          </button>
        )}
        {!value.header_attachment_id && (
          <FilePicker
            label={t('look.addHeader')}
            accept={IMAGE_TYPES}
            className="quiet"
            onPick={(file) => upload.mutate(file)}
          />
        )}
        <Menu label={t('look.options')} className="quiet" align="start" trigger={t('look.options')}>
          <MenuItem
            role="menuitemcheckbox"
            aria-checked={centred}
            onClick={() => change({ title_align: centred ? 'start' : 'center' })}
          >
            {t('look.centreTitle')}
          </MenuItem>
          <MenuItem
            role="menuitemcheckbox"
            aria-checked={!value.hide_children}
            onClick={() => change({ hide_children: !value.hide_children })}
          >
            {t('look.showChildren')}
          </MenuItem>
        </Menu>
      </div>
      <div className={centred ? 'title-row title-center' : 'title-row'}>
        {value.emoji && (
          <button
            type="button"
            className="title-emoji-button"
            aria-label={`${t('look.changeEmoji')}: ${value.emoji}`}
            title={t('look.changeEmoji')}
            onClick={() => setChoosingEmoji(true)}
          >
            {value.emoji}
          </button>
        )}
        {title}
      </div>
      {choosingEmoji && (
        <EmojiDialog
          value={value.emoji}
          onPick={(picked) => {
            setChoosingEmoji(false);
            change({ emoji: picked });
          }}
          onClose={() => setChoosingEmoji(false)}
        />
      )}
    </div>
  );
}
