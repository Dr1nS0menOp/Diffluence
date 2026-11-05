// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import type { Editor } from '@tiptap/react';
import { api, type SearchResult } from '../../api';
import { Dialog, ErrorNote, Field } from '../ui';

const MAX_PAGE_RESULTS = 8;
const ALLOWED_ADDRESS = /^(https?:\/\/\S+|mailto:\S+|\/\S*|#\S*)$/i;

/** Links to pages hold the id, so they survive a rename or a move. */
export function LinkDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const { from, to, empty } = editor.state.selection;
  const isLink = editor.isActive('link');
  const [href, setHref] = useState((editor.getAttributes('link').href as string | undefined) ?? '');
  const [text, setText] = useState(empty ? '' : editor.state.doc.textBetween(from, to, ' '));
  const [search, setSearch] = useState('');
  const [invalid, setInvalid] = useState(false);
  const pages = useQuery({
    queryKey: ['link-search', search],
    queryFn: () =>
      api.get<{ results: SearchResult[] }>(
        `/search?q=${encodeURIComponent(search)}&limit=${MAX_PAGE_RESULTS}`,
      ),
    enabled: search.trim().length > 0,
  });
  const results = (pages.data?.results ?? []).slice(0, MAX_PAGE_RESULTS);

  const pick = (result: SearchResult) => {
    setHref(`/pages/${result.page_id}`);
    setText(text || result.title);
    setSearch('');
  };
  const apply = (e: FormEvent) => {
    e.preventDefault();
    if (!ALLOWED_ADDRESS.test(href.trim())) {
      setInvalid(true);
      return;
    }
    onClose();
    const words = text.trim() || href.trim();
    const link = { type: 'link', attrs: { href: href.trim() } };
    editor
      .chain()
      .focus()
      .extendMarkRange('link')
      .insertContent({ type: 'text', text: words, marks: [link] })
      .run();
  };
  const remove = () => {
    onClose();
    editor.chain().focus().extendMarkRange('link').unsetLink().run();
  };
  return (
    <Dialog title={t('editor.link')} onClose={onClose}>
      <form onSubmit={apply}>
        <Field label={t('editor.linkSearch')}>
          <input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              const first = results[0];
              if (e.key !== 'Enter' || !first) return;
              e.preventDefault();
              pick(first);
            }}
          />
        </Field>
        <ErrorNote error={pages.error} />
        {results.length > 0 && (
          <ul className="plain-list insert-list">
            {results.map((result) => (
              <li key={result.page_id}>
                <button type="button" className="suggestion" onClick={() => pick(result)}>
                  <span className="suggestion-name">{result.title}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        <Field label={t('editor.linkPrompt')}>
          <input
            value={href}
            onChange={(e) => {
              setHref(e.target.value);
              setInvalid(false);
            }}
            required
          />
        </Field>
        {invalid && (
          <p role="alert" className="note note-error">
            {t('editor.linkInvalid')}
          </p>
        )}
        <Field label={t('editor.linkText')}>
          <input value={text} onChange={(e) => setText(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          {isLink && (
            <button type="button" className="quiet" onClick={remove}>
              {t('editor.linkRemove')}
            </button>
          )}
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
