// SPDX-License-Identifier: Apache-2.0
// Choosing the language of a code block. There are far too many for a plain list.
import { useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import type { Editor } from '@tiptap/react';
import { Dialog, Field, Loading } from '../ui';
import { isLanguageName, languageNames, languagesOf, loadAllLanguages } from './code';
import { orderLanguages } from './languages';

export function LanguageDialog({
  editor,
  onPick,
  onClose,
}: {
  editor: Editor;
  onPick: (language: string | null) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const names = useQuery({
    queryKey: ['code-languages'],
    queryFn: async () => {
      await loadAllLanguages();
      return languageNames();
    },
    staleTime: Infinity,
  });
  const wanted = filter.trim().toLowerCase();
  const matches = orderLanguages(
    names.data ?? [],
    languagesOf(editor.state.doc),
    wanted,
    names.data ? isLanguageName(wanted) : false,
  );
  const showPlain = wanted === '' || t('editor.plainText').toLowerCase().includes(wanted);
  const pickFirst = (e: KeyboardEvent) => {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const first = matches[0];
    if (first) onPick(first);
    else if (showPlain) onPick(null);
  };
  return (
    <Dialog title={t('editor.codeLanguage')} onClose={onClose}>
      <Field label={t('editor.languageFilter')}>
        <input autoFocus value={filter} onChange={(e) => setFilter(e.target.value)} onKeyDown={pickFirst} />
      </Field>
      {names.isPending && <Loading />}
      {names.isError && (
        <p role="alert" className="note note-error">
          {t('editor.languagesFailed')}
        </p>
      )}
      {names.data && matches.length === 0 && !showPlain && (
        <p className="muted">{t('editor.languageNone')}</p>
      )}
      <ul className="plain-list insert-list">
        {showPlain && (
          <li>
            <button type="button" className="suggestion" onClick={() => onPick(null)}>
              <span className="suggestion-name">{t('editor.plainText')}</span>
            </button>
          </li>
        )}
        {matches.map((language) => (
          <li key={language}>
            <button type="button" className="suggestion" onClick={() => onPick(language)}>
              <span className="suggestion-name">{language}</span>
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}
