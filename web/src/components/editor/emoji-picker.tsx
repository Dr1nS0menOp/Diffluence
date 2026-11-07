// SPDX-License-Identifier: Apache-2.0
// The emoji tool: groups, recent ones and search (EDT-127). Emoji are plain text, so no
// images are needed.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { readRecentEmoji, rememberEmoji, usePerson } from '../../personal';
import { Dialog, Field } from '../ui';
import emoji from './emoji.json';

// The list in emoji.json runs group after group, so each group is where it ends.
const EMOJI_GROUPS = [
  { id: 'faces', end: 18 },
  { id: 'hands', end: 30 },
  { id: 'symbols', end: 53 },
  { id: 'work', end: 82 },
  { id: 'nature', end: 91 },
  { id: 'other', end: emoji.length },
];

/** The groups with what is in them. */
export function emojiGroups(): { id: string; glyphs: string[] }[] {
  let start = 0;
  return EMOJI_GROUPS.map(({ id, end }) => {
    const glyphs = emoji.slice(start, end).map((entry) => entry.glyph);
    start = end;
    return { id, glyphs };
  });
}

function Glyphs({ glyphs, onPick }: { glyphs: string[]; onPick: (glyph: string) => void }) {
  return (
    <div className="emoji-grid">
      {glyphs.map((glyph) => (
        <button key={glyph} type="button" className="quiet" onClick={() => onPick(glyph)}>
          {glyph}
        </button>
      ))}
    </div>
  );
}

export function EmojiPicker({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const person = usePerson();
  const [query, setQuery] = useState('');
  const needle = query.trim().toLowerCase();
  const found = emoji.filter((entry) => entry.words.includes(needle)).map((entry) => entry.glyph);
  const recent = readRecentEmoji(person);
  const pick = (glyph: string) => {
    rememberEmoji(person, glyph);
    onClose();
    editor.chain().focus().insertContent({ type: 'text', text: glyph }).run();
  };
  return (
    <Dialog title={t('editor.emojiPicker')} onClose={onClose}>
      <Field label={t('editor.emojiSearch')}>
        <input autoFocus value={query} onChange={(e) => setQuery(e.target.value)} />
      </Field>
      {needle ? (
        <Glyphs glyphs={found} onPick={pick} />
      ) : (
        <>
          {recent.length > 0 && (
            <section aria-label={t('editor.emojiGroups.recent')}>
              <h3 className="suggestion-group">{t('editor.emojiGroups.recent')}</h3>
              <Glyphs glyphs={recent} onPick={pick} />
            </section>
          )}
          {emojiGroups().map((group) => (
            <section key={group.id} aria-label={t(`editor.emojiGroups.${group.id}`)}>
              <h3 className="suggestion-group">{t(`editor.emojiGroups.${group.id}`)}</h3>
              <Glyphs glyphs={group.glyphs} onPick={pick} />
            </section>
          ))}
        </>
      )}
    </Dialog>
  );
}
