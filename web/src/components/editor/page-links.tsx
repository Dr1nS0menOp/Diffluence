// SPDX-License-Identifier: Apache-2.0
// Links that follow their page: a title, a tag or a card (EDT-057, 065, 129). The document
// holds the id alone, so what shows is looked up per reader, and a reader who may not see
// the page gets neutral words with nothing of the page in them (EDT-066, spec 023).
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Node as DocumentNode } from '@tiptap/core';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import { api } from '../../api';
import { navigate } from '../../router';
import { IconPage } from '../icons';
import { useToast } from '../toast';
import { RelativeTime } from '../ui';
import { dataAttribute } from './dynamic/nodes';

const TAG_ICON_SIZE = 14;
const FACT_FRESH_MS = 60_000;
const MAX_FACTS_PER_ASK = 500;

export interface LinkFact {
  id: string;
  title: string;
  emoji: string;
  space_key: string;
  space_name: string;
  excerpt: string;
  updated_at: string;
}

interface Waiting {
  resolve: (fact: LinkFact | null) => void;
  reject: (error: unknown) => void;
}

// A private queue: a page with fifty links asks once, not fifty times.
const waiting = new Map<string, Waiting[]>();

async function askForFacts(asked: Map<string, Waiting[]>) {
  const ids = [...asked.keys()];
  for (let start = 0; start < ids.length; start += MAX_FACTS_PER_ASK) {
    const part = ids.slice(start, start + MAX_FACTS_PER_ASK);
    try {
      const { pages } = await api.post<{ pages: LinkFact[] }>('/pages/link-facts', { ids: part });
      const found = new Map(pages.map((page) => [page.id, page]));
      part.forEach((id) => asked.get(id)?.forEach((one) => one.resolve(found.get(id) ?? null)));
    } catch (error) {
      part.forEach((id) => asked.get(id)?.forEach((one) => one.reject(error)));
    }
  }
}

function factOf(id: string): Promise<LinkFact | null> {
  return new Promise((resolve, reject) => {
    if (waiting.size === 0) {
      queueMicrotask(() => {
        const asked = new Map(waiting);
        waiting.clear();
        void askForFacts(asked);
      });
    }
    waiting.set(id, [...(waiting.get(id) ?? []), { resolve, reject }]);
  });
}

/** What this reader may see of a page. An error, such as not being logged in, counts as nothing. */
export function useLinkFact(id: string | null) {
  const asked = useQuery({
    queryKey: ['link-fact', id],
    queryFn: () => factOf(id ?? ''),
    enabled: id !== null,
    staleTime: FACT_FRESH_MS,
    retry: false,
  });
  return { fact: asked.data ?? null, isPending: asked.isPending && id !== null };
}

export const pageAddress = (id: string, fragment?: string | null) =>
  fragment ? `/pages/${id}#${fragment}` : `/pages/${id}`;

function HiddenPage() {
  const { t } = useTranslation();
  return (
    <span className="link-gone" title={t('page.linkGone')}>
      {t('page.linkHidden')}
    </span>
  );
}

function PageLinkView({ node, editor }: ReactNodeViewProps) {
  const {
    page_id: id,
    fragment,
    display,
  } = node.attrs as {
    page_id: string;
    fragment: string | null;
    display: string | null;
  };
  const { fact, isPending } = useLinkFact(id);
  const className = display === 'tag' ? 'page-tag' : undefined;
  return (
    <NodeViewWrapper as="span" contentEditable={false}>
      {isPending ? (
        <span className="muted">…</span>
      ) : fact ? (
        <a
          href={pageAddress(id, fragment)}
          className={className}
          onClick={(e) => {
            if (editor.isEditable) e.preventDefault();
          }}
        >
          {display === 'tag' && <IconPage size={TAG_ICON_SIZE} />}
          {fact.title}
        </a>
      ) : (
        <HiddenPage />
      )}
    </NodeViewWrapper>
  );
}

const pageAttributes = {
  page_id: dataAttribute('page_id'),
};

export const PageLink = DocumentNode.create({
  name: 'pageLink',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return { ...pageAttributes, fragment: dataAttribute('fragment'), display: dataAttribute('display') };
  },
  parseHTML() {
    return [{ tag: 'span[data-page-link]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['span', { ...HTMLAttributes, 'data-page-link': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(PageLinkView);
  },
});

function PageCardView({ node, editor }: ReactNodeViewProps) {
  const id = node.attrs.page_id as string;
  const { fact, isPending } = useLinkFact(id);
  if (isPending) return <NodeViewWrapper className="page-card" contentEditable={false} />;
  if (!fact) {
    return (
      <NodeViewWrapper className="page-card" contentEditable={false}>
        <HiddenPage />
      </NodeViewWrapper>
    );
  }
  return (
    <NodeViewWrapper className="page-card" contentEditable={false}>
      <a
        href={pageAddress(id)}
        className="page-card-title"
        onClick={(e) => {
          if (editor.isEditable) e.preventDefault();
        }}
      >
        <IconPage size={TAG_ICON_SIZE} />
        {fact.title}
      </a>
      {fact.excerpt && <p className="page-card-excerpt">{fact.excerpt}</p>}
      <p className="muted page-card-facts">
        {fact.space_name} · <RelativeTime value={fact.updated_at} />
      </p>
    </NodeViewWrapper>
  );
}

export const PageCard = DocumentNode.create({
  name: 'pageCard',
  group: 'block',
  atom: true,
  draggable: true,
  addAttributes() {
    return pageAttributes;
  },
  parseHTML() {
    return [{ tag: 'div[data-page-card]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', { ...HTMLAttributes, 'data-page-card': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(PageCardView);
  },
});

interface Followed {
  title: string;
  page_id: string;
}

function usePlaceholderTarget(sourcePage: string | undefined, title: string) {
  const followed = useQuery({
    queryKey: ['placeholders', sourcePage],
    queryFn: () => api.get<{ placeholders: Followed[] }>(`/pages/${sourcePage}/placeholders`),
    enabled: sourcePage !== undefined,
    retry: false,
  });
  return followed.data?.placeholders.find((one) => one.title === title)?.page_id;
}

function FollowButton({ sourcePage, title }: { sourcePage: string; title: string }) {
  const { t } = useTranslation();
  const toast = useToast();
  const follow = useMutation({
    mutationFn: () =>
      api.post<{ page_id: string }>(`/pages/${sourcePage}/placeholders`, {
        title,
      }),
    onSuccess: ({ page_id: page }) => navigate(`/pages/${page}/edit`),
    onError: (error) => toast({ message: error instanceof Error ? error.message : t('app.error') }),
  });
  return (
    <button
      type="button"
      className="link-placeholder"
      title={t('page.placeholderHelp')}
      disabled={follow.isPending}
      onClick={() => follow.mutate()}
    >
      {title}
    </button>
  );
}

function PlaceholderView({ node, editor, extension }: ReactNodeViewProps) {
  const title = node.attrs.title as string;
  const { sourcePage } = extension.options as PlaceholderOptions;
  const reading = !editor.isEditable && sourcePage !== undefined;
  const target = usePlaceholderTarget(reading ? sourcePage : undefined, title);
  return (
    <NodeViewWrapper as="span" contentEditable={false}>
      {target ? (
        <a href={pageAddress(target)}>{title}</a>
      ) : reading ? (
        <FollowButton sourcePage={sourcePage} title={title} />
      ) : (
        <span className="link-placeholder">{title}</span>
      )}
    </NodeViewWrapper>
  );
}

interface PlaceholderOptions {
  /** The page this document belongs to. Only a published page can start another. */
  sourcePage?: string;
}

export const PlaceholderLink = DocumentNode.create<PlaceholderOptions>({
  name: 'placeholderLink',
  group: 'inline',
  inline: true,
  atom: true,
  addOptions() {
    return { sourcePage: undefined };
  },
  addAttributes() {
    return { title: dataAttribute('title') };
  },
  parseHTML() {
    return [{ tag: 'span[data-placeholder-link]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['span', { ...HTMLAttributes, 'data-placeholder-link': '' }];
  },
  renderText({ node }) {
    return node.attrs.title as string;
  },
  addNodeView() {
    return ReactNodeViewRenderer(PlaceholderView);
  },
});
