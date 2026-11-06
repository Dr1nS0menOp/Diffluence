// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import type { Editor } from '@tiptap/react';
import { api, type Json, type ListedPage, type SearchResult } from '../../api';
import { Dialog, ErrorNote, Field } from '../ui';
import { pageOfAddress } from './gone-links';
import {
  addressOf,
  currentLink,
  insertPageLink,
  insertPlaceholder,
  insertWordsLink,
  targetsOfDocument,
  unlink,
  type LinkDisplay,
  type LinkTarget,
} from './link-commands';

const MAX_PAGE_RESULTS = 8;
const ALLOWED_ADDRESS = /^(https?:\/\/\S+|mailto:\S+|\/\S*|#\S*)$/i;
const DISPLAYS: Exclude<LinkDisplay, 'words'>[] = ['title', 'tag', 'card'];

interface PageChoice {
  id: string;
  title: string;
}

interface PageTargets {
  headings: { slug: string; text: string; level: number }[];
  anchors: string[];
}

function usePageChoices(search: string) {
  const found = useQuery({
    queryKey: ['link-search', search],
    queryFn: () =>
      api.get<{ results: SearchResult[] }>(
        `/search?q=${encodeURIComponent(search)}&limit=${MAX_PAGE_RESULTS}`,
      ),
    enabled: search.trim().length > 0,
  });
  const recent = useQuery({
    queryKey: ['/me/recent'],
    queryFn: () => api.get<{ pages: ListedPage[] }>('/me/recent'),
    enabled: search.trim().length === 0,
  });
  const choices: PageChoice[] =
    search.trim().length > 0
      ? (found.data?.results ?? []).map((result) => ({ id: result.page_id, title: result.title }))
      : (recent.data?.pages ?? []).map((page) => ({ id: page.id, title: page.title }));
  return { choices: choices.slice(0, MAX_PAGE_RESULTS), error: found.error ?? recent.error };
}

function useTargets(page: string | null): LinkTarget[] {
  const asked = useQuery({
    queryKey: ['link-targets', page],
    queryFn: () => api.get<PageTargets>(`/pages/${page}/targets`),
    enabled: page !== null,
  });
  return [
    ...(asked.data?.headings ?? []).map((h) => ({ fragment: h.slug, label: h.text, level: h.level })),
    ...(asked.data?.anchors ?? []).map((name) => ({ fragment: name, label: name, level: 0 })),
  ];
}

function TargetField({
  targets,
  value,
  allowTop,
  onChange,
}: {
  targets: LinkTarget[];
  value: string;
  allowTop: boolean;
  onChange: (fragment: string) => void;
}) {
  const { t } = useTranslation();
  if (targets.length === 0) return null;
  return (
    <Field label={t('editor.linkTarget')}>
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {allowTop && <option value="">{t('editor.linkTargetTop')}</option>}
        {targets.map((target) => (
          <option key={target.fragment} value={target.fragment}>
            {target.level === 0 ? t('editor.linkTargetAnchor', { name: target.label }) : target.label}
          </option>
        ))}
      </select>
    </Field>
  );
}

const withoutFragment = (href: string) => href.split('#')[0] ?? '';
const fragmentOf = (href: string) => href.split('#')[1] ?? '';

/** Links to pages hold the id, so they survive a rename or a move. */
export function LinkDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const { from, to, empty } = editor.state.selection;
  const current = currentLink(editor);
  const [href, setHref] = useState((current && addressOf(current)) ?? '');
  const [text, setText] = useState(
    empty || current?.kind === 'page' ? '' : editor.state.doc.textBetween(from, to, ' '),
  );
  const [display, setDisplay] = useState<Exclude<LinkDisplay, 'words'>>(
    current?.kind === 'page' && current.display !== 'words' ? current.display : 'title',
  );
  const [search, setSearch] = useState('');
  const [invalid, setInvalid] = useState(false);
  const { choices, error } = usePageChoices(search);
  const linkedPage = pageOfAddress(href) ?? null;
  const onThisPage = href.startsWith('#');
  const otherTargets = useTargets(linkedPage);
  const ownTargets = targetsOfDocument(editor.getJSON() as Json);
  const firstOwnTarget = ownTargets[0];
  const newTitle = search.trim();

  const pick = (choice: PageChoice) => {
    setHref(`/pages/${choice.id}`);
    setSearch('');
    setInvalid(false);
  };
  const apply = (e: FormEvent) => {
    e.preventDefault();
    const address = href.trim();
    if (!ALLOWED_ADDRESS.test(address)) {
      setInvalid(true);
      return;
    }
    onClose();
    const words = text.trim();
    if (linkedPage && (!words || display !== 'title')) {
      insertPageLink(editor, linkedPage, fragmentOf(address) || null, display);
      return;
    }
    insertWordsLink(editor, address, words || address);
  };
  const remove = () => {
    onClose();
    unlink(editor, current?.kind === 'placeholder' ? current.title : text);
  };
  const addPlaceholder = () => {
    onClose();
    insertPlaceholder(editor, newTitle);
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
              const first = choices[0];
              if (e.key !== 'Enter' || !first) return;
              e.preventDefault();
              pick(first);
            }}
          />
        </Field>
        <ErrorNote error={error} />
        {choices.length > 0 && (
          <>
            {newTitle === '' && <p className="muted">{t('editor.linkRecent')}</p>}
            <ul className="plain-list insert-list">
              {choices.map((choice) => (
                <li key={choice.id}>
                  <button type="button" className="suggestion" onClick={() => pick(choice)}>
                    <span className="suggestion-name">{choice.title}</span>
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
        {newTitle !== '' && !choices.some((choice) => choice.title === newTitle) && (
          <button type="button" className="quiet" onClick={addPlaceholder}>
            {t('editor.linkPlaceholder', { title: newTitle })}
          </button>
        )}
        {newTitle === '' && firstOwnTarget && !onThisPage && (
          <button type="button" className="quiet" onClick={() => setHref(`#${firstOwnTarget.fragment}`)}>
            {t('editor.linkThisPage')}
          </button>
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
        {linkedPage && (
          <TargetField
            targets={otherTargets}
            value={fragmentOf(href)}
            allowTop
            onChange={(fragment) =>
              setHref(fragment ? `${withoutFragment(href)}#${fragment}` : withoutFragment(href))
            }
          />
        )}
        {onThisPage && (
          <TargetField
            targets={ownTargets}
            value={fragmentOf(href)}
            allowTop={false}
            onChange={(fragment) => setHref(`#${fragment}`)}
          />
        )}
        {linkedPage && (
          <Field label={t('editor.linkDisplay')}>
            <select
              value={display}
              onChange={(e) => setDisplay(e.target.value as Exclude<LinkDisplay, 'words'>)}
            >
              {DISPLAYS.map((option) => (
                <option key={option} value={option}>
                  {t(`editor.linkDisplay_${option}`)}
                </option>
              ))}
            </select>
          </Field>
        )}
        {display === 'title' && (
          <Field label={t('editor.linkText')} help={linkedPage ? t('editor.linkTextHelp') : undefined}>
            <input value={text} onChange={(e) => setText(e.target.value)} />
          </Field>
        )}
        <div className="dialog-actions">
          {current && (
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
