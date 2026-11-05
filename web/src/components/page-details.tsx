// SPDX-License-Identifier: Apache-2.0
// One panel for the facts around a page (spec 009, D7). It sits on top of the page
// without taking over the screen, so reading goes on beside it.
import { useEffect, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Attachment, type PageDetail, type PageFacts, type PageRef } from '../api';
import { Link } from '../router';
import { countWords, readingMinutes } from './editor/outline';
import { IconClose } from './icons';
import { PageMark } from './topbar';
import { ErrorNote, formatDate, formatSize, Loading } from './ui';

function PageLinks({ title, pages, empty }: { title: string; pages: PageRef[]; empty: string }) {
  return (
    <section>
      <h3>{title}</h3>
      {pages.length === 0 && <p className="muted">{empty}</p>}
      <ul className="plain-list details-links">
        {pages.map((p) => (
          <li key={p.id}>
            <PageMark page={p} />
            <Link href={`/pages/${p.id}`}>{p.title}</Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Fact({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div>
      <dt>{name}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function Files({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['attachments', pageId],
    queryFn: () => api.get<{ attachments: Attachment[] }>(`/pages/${pageId}/attachments`),
  });
  return (
    <section>
      <h3>{t('attachments.title')}</h3>
      <ErrorNote error={q.error} />
      {q.data?.attachments.length === 0 && <p className="muted">{t('attachments.none')}</p>}
      <ul className="plain-list details-links">
        {q.data?.attachments.map((a) => (
          <li key={a.id}>
            <a href={`/api/v1/attachments/${a.id}/content?download=1`}>{a.filename}</a>
            <span className="muted">{formatSize(a.size)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function DetailsPanel({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const heading = useRef<HTMLHeadingElement>(null);
  const q = useQuery({
    queryKey: ['page-details', page.id, page.current_version],
    queryFn: () => api.get<PageFacts>(`/pages/${page.id}/details`),
  });

  useEffect(() => {
    const opener = document.activeElement;
    heading.current?.focus();
    return () => {
      if (opener instanceof HTMLElement) opener.focus();
    };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const words = countWords(page.document);
  const minutes = readingMinutes(words);
  const facts = q.data;
  return createPortal(
    <aside className="details-panel" aria-labelledby="details-title">
      <header className="dialog-head">
        <h2 id="details-title" ref={heading} tabIndex={-1}>
          {t('details.title')}
        </h2>
        <button type="button" className="icon" aria-label={t('common.close')} onClick={onClose}>
          <IconClose />
        </button>
      </header>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {facts && (
        <>
          <dl className="details-facts">
            <Fact name={t('details.created')}>
              {facts.created_by_name || t('common.unknown')}, {formatDate(facts.created_at)}
            </Fact>
            <Fact name={t('details.changed')}>
              {facts.updated_by_name || t('common.unknown')}, {formatDate(facts.updated_at)}
            </Fact>
            <Fact name={t('details.length')}>{t('details.words', { count: words })}</Fact>
            {minutes !== null && <Fact name={t('details.readTime')}>{t('page.readTime', { minutes })}</Fact>}
            <Fact name={t('details.address')}>
              <span className="details-address">{`${window.location.origin}/pages/${page.id}`}</span>
            </Fact>
          </dl>
          <section id="details-contributors">
            <h3>{t('details.contributors')}</h3>
            <ul className="plain-list details-links">
              {facts.contributors.map((c) => (
                <li key={c.id}>
                  {c.name || t('common.unknown')}
                  <span className="muted">{t('details.versions', { count: c.versions })}</span>
                </li>
              ))}
            </ul>
          </section>
          <PageLinks title={t('details.incoming')} pages={facts.incoming} empty={t('details.noIncoming')} />
          <PageLinks title={t('details.outgoing')} pages={facts.outgoing} empty={t('details.noOutgoing')} />
          <Files pageId={page.id} />
        </>
      )}
    </aside>,
    document.body,
  );
}

/** The pages below this one, at the bottom of the page. */
export function ChildPages({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['page-children', pageId],
    queryFn: () => api.get<PageFacts>(`/pages/${pageId}/details`),
    select: (facts) => facts.children,
  });
  if (!q.data || q.data.length === 0) return null;
  return (
    <section className="child-pages" aria-label={t('page.childPages')}>
      <h2>{t('page.childPages')}</h2>
      <ul className="page-list">
        {q.data.map((p) => (
          <li key={p.id}>
            <PageMark page={p} />
            <Link href={`/pages/${p.id}`}>{p.title}</Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
