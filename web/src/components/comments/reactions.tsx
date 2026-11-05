// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Reaction, type Reactor } from '../../api';
import common from './common-reactions.json';
import emoji from '../editor/emoji.json';
import { IconEmoji } from '../icons';
import { ErrorNote } from '../ui';

const MATCHES_SHOWN = 24;
const WHO_DELAY_MS = 400;
const ADD_ICON_SIZE = 18;

function WhoCard({ who, emojiName }: { who: Reactor[]; emojiName: string }) {
  const { t } = useTranslation();
  const line = who.find((r) => r.emoji === emojiName);
  if (!line) return null;
  return (
    <span role="tooltip" className="reaction-who">
      {line.names.join(', ')}
      {line.more > 0 && ` ${t('reactions.more', { count: line.more })}`}
    </span>
  );
}

function Counter({
  reaction,
  who,
  readOnly,
  onToggle,
}: {
  reaction: Reaction;
  who: Reactor[];
  readOnly?: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const show = () => {
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setOpen(true), WHO_DELAY_MS);
  };
  const hide = () => {
    clearTimeout(timer.current);
    setOpen(false);
  };
  useEffect(() => () => clearTimeout(timer.current), []);
  return (
    <span className="reaction" onMouseEnter={show} onMouseLeave={hide}>
      <button
        type="button"
        className="reaction-count"
        aria-pressed={reaction.mine}
        aria-label={t('reactions.toggle', { emoji: reaction.emoji, count: reaction.count })}
        disabled={readOnly}
        onFocus={show}
        onBlur={hide}
        onKeyDown={(e) => {
          if (e.key === 'Escape') hide();
        }}
        onClick={onToggle}
      >
        {reaction.emoji} {reaction.count}
      </button>
      {open && <WhoCard who={who} emojiName={reaction.emoji} />}
    </span>
  );
}

function Picker({ onPick, onClose }: { onPick: (glyph: string) => void; onClose: () => void }) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const matches = emoji
    .filter((entry) => entry.words.includes(filter.trim().toLowerCase()))
    .slice(0, MATCHES_SHOWN);
  return (
    <div className="reaction-picker" onKeyDown={(e) => e.key === 'Escape' && onClose()}>
      <div className="emoji-grid">
        {common.map((glyph) => (
          <button key={glyph} type="button" className="ghost" onClick={() => onPick(glyph)}>
            {glyph}
          </button>
        ))}
      </div>
      <input
        type="search"
        autoFocus
        value={filter}
        aria-label={t('reactions.search')}
        placeholder={t('reactions.search')}
        onChange={(e) => setFilter(e.target.value)}
      />
      {filter.trim() !== '' && (
        <div className="emoji-grid">
          {matches.map((entry) => (
            <button
              key={entry.glyph}
              type="button"
              className="ghost"
              aria-label={entry.words}
              onClick={() => onPick(entry.glyph)}
            >
              {entry.glyph}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** A row of counters, for a page or a comment. */
export function Reactions({
  path,
  queryKey,
  initial,
  readOnly,
}: {
  path: string;
  queryKey: unknown[];
  initial?: Reaction[];
  readOnly?: boolean;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [picking, setPicking] = useState(false);
  const q = useQuery({
    queryKey: ['reactions', path],
    queryFn: () => api.get<{ reactions: Reaction[]; who: Reactor[] }>(`${path}/reactions`),
  });
  const set = useMutation({
    mutationFn: (v: { glyph: string; on: boolean }) => {
      const address = `${path}/reactions/${encodeURIComponent(v.glyph)}`;
      return v.on ? api.put(address) : api.del(address);
    },
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['reactions', path] });
      await client.invalidateQueries({ queryKey });
    },
  });
  const list = q.data?.reactions ?? initial ?? [];
  const who = q.data?.who ?? [];
  return (
    <div className="reactions" aria-label={t('reactions.title')} role="group">
      {list.map((r) => (
        <Counter
          key={r.emoji}
          reaction={r}
          who={who}
          readOnly={readOnly}
          onToggle={() => set.mutate({ glyph: r.emoji, on: !r.mine })}
        />
      ))}
      {!readOnly && (
        <button
          type="button"
          className="icon reaction-add"
          aria-label={t('reactions.add')}
          title={t('reactions.add')}
          aria-expanded={picking}
          onClick={() => setPicking(!picking)}
        >
          <IconEmoji size={ADD_ICON_SIZE} />
        </button>
      )}
      {picking && (
        <Picker
          onClose={() => setPicking(false)}
          onPick={(glyph) => {
            setPicking(false);
            set.mutate({ glyph, on: true });
          }}
        />
      )}
      <ErrorNote error={set.error} />
    </div>
  );
}
