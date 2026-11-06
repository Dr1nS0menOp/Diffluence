// SPDX-License-Identifier: Apache-2.0
// Find and replace inside the document (spec 008, A17).
import { useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Extension, type Editor } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey, TextSelection } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import { useEditorState } from '@tiptap/react';
import { IconChevronDown, IconChevronUp, IconClose } from '../icons';

// One character stands for any item without text, so positions stay right.
const LEAF_PLACEHOLDER = '￼';

interface Match {
  from: number;
  to: number;
}

interface FindOptions {
  matchCase: boolean;
  wholeWord: boolean;
}

interface FindState extends FindOptions {
  query: string;
  matches: Match[];
  current: number;
}

const NO_OPTIONS: FindOptions = { matchCase: false, wholeWord: false };
const EMPTY: FindState = { ...NO_OPTIONS, query: '', matches: [], current: 0 };
const WORD_CHARACTER = /[\p{L}\p{N}_]/u;

const isWordCharacter = (character: string | undefined) =>
  character !== undefined && WORD_CHARACTER.test(character);

const findKey = new PluginKey<FindState>('find');

/** Every place where the words occur. A match never crosses from one block to the next. */
export function findMatches(doc: ProseNode, query: string, options: FindOptions): Match[] {
  if (!query) return [];
  const { matchCase, wholeWord } = options;
  const needle = matchCase ? query : query.toLowerCase();
  const matches: Match[] = [];
  doc.descendants((node, pos) => {
    if (!node.isTextblock) return true;
    const raw = node.textBetween(0, node.content.size, undefined, LEAF_PLACEHOLDER);
    const text = matchCase ? raw : raw.toLowerCase();
    for (let at = text.indexOf(needle); at !== -1; at = text.indexOf(needle, at + needle.length)) {
      const inLongerWord = isWordCharacter(text[at - 1]) || isWordCharacter(text[at + needle.length]);
      if (wholeWord && inLongerWord) continue;
      matches.push({ from: pos + 1 + at, to: pos + 1 + at + needle.length });
    }
    return false;
  });
  return matches;
}

function decorations(doc: ProseNode, state: FindState): DecorationSet {
  const marks = state.matches.map((match, index) =>
    Decoration.inline(match.from, match.to, {
      class: index === state.current ? 'find-match find-current' : 'find-match',
    }),
  );
  return DecorationSet.create(doc, marks);
}

export const FindReplace = Extension.create<{ onOpen: () => void }>({
  name: 'findReplace',
  addOptions() {
    return { onOpen: () => undefined };
  },
  addKeyboardShortcuts() {
    return {
      'Mod-f': () => {
        this.options.onOpen();
        return true;
      },
    };
  },
  addProseMirrorPlugins() {
    return [
      new Plugin<FindState>({
        key: findKey,
        state: {
          init: () => EMPTY,
          apply(tr, value, _old, next) {
            const update = tr.getMeta(findKey) as Partial<FindState> | undefined;
            if (!update && !tr.docChanged) return value;
            const merged = { ...value, ...update };
            const matches = findMatches(next.doc, merged.query, merged);
            return { ...merged, matches, current: Math.min(merged.current, Math.max(matches.length - 1, 0)) };
          },
        },
        props: {
          decorations(state) {
            const value = findKey.getState(state);
            return value && value.matches.length > 0 ? decorations(state.doc, value) : null;
          },
        },
      }),
    ];
  },
});

const stateOf = (editor: Editor) => findKey.getState(editor.state) ?? EMPTY;

function search(editor: Editor, query: string, options: FindOptions) {
  editor.view.dispatch(editor.state.tr.setMeta(findKey, { ...options, query, current: 0 }));
}

function revealCurrent(editor: Editor) {
  const { matches, current } = stateOf(editor);
  const match = matches[current];
  if (!match) return;
  const { tr, doc } = editor.state;
  editor.view.dispatch(tr.setSelection(TextSelection.create(doc, match.from, match.to)).scrollIntoView());
}

function step(editor: Editor, by: 1 | -1) {
  const { matches, current } = stateOf(editor);
  if (matches.length === 0) return;
  const next = (current + by + matches.length) % matches.length;
  editor.view.dispatch(editor.state.tr.setMeta(findKey, { current: next }));
  revealCurrent(editor);
}

function replaceCurrent(editor: Editor, text: string) {
  const { matches, current } = stateOf(editor);
  const match = matches[current];
  if (match) editor.view.dispatch(editor.state.tr.insertText(text, match.from, match.to));
  revealCurrent(editor);
}

/** All at once and from the end, so earlier positions stay right and one Undo restores them. */
function replaceAll(editor: Editor, text: string) {
  const { matches } = stateOf(editor);
  const { tr } = editor.state;
  [...matches].reverse().forEach((match) => tr.insertText(text, match.from, match.to));
  editor.view.dispatch(tr);
}

export function FindBar({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [replacement, setReplacement] = useState('');
  const [options, setOptions] = useState(NO_OPTIONS);
  const found = useEditorState({
    editor,
    selector: (s) => {
      const { matches, current } = stateOf(s.editor);
      return { total: matches.length, at: current + 1 };
    },
  });
  const close = () => {
    search(editor, '', NO_OPTIONS);
    onClose();
    editor.commands.focus();
  };
  const changeOptions = (change: Partial<FindOptions>) => {
    const next = { ...options, ...change };
    setOptions(next);
    search(editor, query, next);
  };
  const keys = (e: KeyboardEvent) => {
    if (e.key === 'Escape') close();
    if (e.key !== 'Enter') return;
    e.preventDefault();
    step(editor, e.shiftKey ? -1 : 1);
  };
  return (
    <div className="find-bar" role="search" aria-label={t('editor.find')} onKeyDown={keys}>
      <input
        autoFocus
        className="find-input"
        value={query}
        aria-label={t('editor.find')}
        placeholder={t('editor.find')}
        onChange={(e) => {
          setQuery(e.target.value);
          search(editor, e.target.value, options);
          revealCurrent(editor);
        }}
      />
      <span role="status" className="muted find-count">
        {query && t('editor.findCount', { at: found.total ? found.at : 0, total: found.total })}
      </span>
      <button
        type="button"
        className="icon"
        aria-label={t('editor.findPrevious')}
        title={t('editor.findPrevious')}
        onClick={() => step(editor, -1)}
      >
        <IconChevronUp size={18} />
      </button>
      <button
        type="button"
        className="icon"
        aria-label={t('editor.findNext')}
        title={t('editor.findNext')}
        onClick={() => step(editor, 1)}
      >
        <IconChevronDown size={18} />
      </button>
      <label className="row find-option">
        <input
          type="checkbox"
          checked={options.matchCase}
          onChange={(e) => changeOptions({ matchCase: e.target.checked })}
        />
        {t('editor.matchCase')}
      </label>
      <label className="row find-option">
        <input
          type="checkbox"
          checked={options.wholeWord}
          onChange={(e) => changeOptions({ wholeWord: e.target.checked })}
        />
        {t('editor.wholeWord')}
      </label>
      <input
        className="find-input"
        value={replacement}
        aria-label={t('editor.replaceWith')}
        placeholder={t('editor.replaceWith')}
        onChange={(e) => setReplacement(e.target.value)}
      />
      <button type="button" className="ghost" onClick={() => replaceCurrent(editor, replacement)}>
        {t('editor.replace')}
      </button>
      <button type="button" className="ghost" onClick={() => replaceAll(editor, replacement)}>
        {t('editor.replaceAll')}
      </button>
      <button
        type="button"
        className="icon"
        aria-label={t('common.close')}
        title={t('common.close')}
        onClick={close}
      >
        <IconClose size={18} />
      </button>
    </div>
  );
}
