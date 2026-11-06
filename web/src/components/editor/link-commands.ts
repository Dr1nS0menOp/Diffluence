// SPDX-License-Identifier: Apache-2.0
// What a link is and what can become of it, for the link dialog and the link tools.
import type { Editor, JSONContent } from '@tiptap/core';
import { NodeSelection } from '@tiptap/pm/state';
import type { Json } from '../../api';
import { pageOfAddress } from './gone-links';
import { headingsOf } from './outline';
import { pageAddress } from './page-links';

export type LinkDisplay = 'words' | 'title' | 'tag' | 'card';

export type CurrentLink =
  | { kind: 'mark'; href: string }
  | { kind: 'page'; page: string; fragment: string | null; display: LinkDisplay }
  | { kind: 'placeholder'; title: string };

/** The link at the cursor or in the selection, whatever kind it is. */
export function currentLink(editor: Editor): CurrentLink | null {
  const { selection } = editor.state;
  const node = selection instanceof NodeSelection ? selection.node : null;
  if (node?.type.name === 'pageLink') {
    const {
      page_id: page,
      fragment,
      display,
    } = node.attrs as {
      page_id: string;
      fragment: string | null;
      display: string | null;
    };
    return { kind: 'page', page, fragment, display: display === 'tag' ? 'tag' : 'title' };
  }
  if (node?.type.name === 'pageCard') {
    return { kind: 'page', page: node.attrs.page_id as string, fragment: null, display: 'card' };
  }
  if (node?.type.name === 'placeholderLink')
    return { kind: 'placeholder', title: node.attrs.title as string };
  const href = editor.getAttributes('link').href as string | undefined;
  return href ? { kind: 'mark', href } : null;
}

/** Where a link leads, for opening or copying it. */
export function addressOf(link: CurrentLink): string | null {
  if (link.kind === 'mark') return link.href;
  return link.kind === 'page' ? pageAddress(link.page, link.fragment) : null;
}

/** The page and the place on it, when a link leads to a page of this instance. */
export function linkedPageOf(link: CurrentLink | null): { page: string; fragment: string | null } | null {
  if (link?.kind === 'page') return { page: link.page, fragment: link.fragment };
  if (link?.kind !== 'mark') return null;
  const page = pageOfAddress(link.href);
  if (!page) return null;
  return { page, fragment: link.href.split('#')[1] ?? null };
}

function nodeFor(page: string, fragment: string | null, display: LinkDisplay): JSONContent {
  if (display === 'card') return { type: 'pageCard', attrs: { page_id: page } };
  return {
    type: 'pageLink',
    attrs: { page_id: page, fragment, display: display === 'tag' ? 'tag' : null },
  };
}

/** Puts a link that follows its page in place of the selection or the link at the cursor. */
export function insertPageLink(editor: Editor, page: string, fragment: string | null, display: LinkDisplay) {
  const chain = editor.chain().focus();
  if (editor.isActive('link')) chain.extendMarkRange('link');
  chain.insertContent(nodeFor(page, fragment, display)).run();
}

export function insertWordsLink(editor: Editor, href: string, words: string) {
  const link = { type: 'link', attrs: { href } };
  const chain = editor.chain().focus();
  if (editor.isActive('link')) chain.extendMarkRange('link');
  chain.insertContent({ type: 'text', text: words, marks: [link] }).run();
}

export function insertPlaceholder(editor: Editor, title: string) {
  editor.chain().focus().insertContent({ type: 'placeholderLink', attrs: { title } }).run();
}

/** Takes a link away, and leaves its words. A page link only has words for this person. */
export function unlink(editor: Editor, words: string) {
  const { selection } = editor.state;
  if (!(selection instanceof NodeSelection)) {
    editor.chain().focus().extendMarkRange('link').unsetLink().run();
    return;
  }
  editor.chain().focus().insertContent({ type: 'text', text: words }).run();
}

export interface LinkTarget {
  fragment: string;
  label: string;
  level: number;
}

interface JsonNode {
  type?: string;
  attrs?: { name?: string };
  content?: JsonNode[];
}

function anchorsIn(node: JsonNode): string[] {
  const own = node.type === 'anchor' && node.attrs?.name ? [node.attrs.name] : [];
  return [...own, ...(node.content ?? []).flatMap(anchorsIn)];
}

/** The headings and anchors of the page being written, as it will be published. */
export function targetsOfDocument(document: Json): LinkTarget[] {
  return [
    ...headingsOf(document).map((h) => ({ fragment: h.slug, label: h.text, level: h.level })),
    ...anchorsIn(document as JsonNode).map((name) => ({ fragment: name, label: name, level: 0 })),
  ];
}
