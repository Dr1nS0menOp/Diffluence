// SPDX-License-Identifier: Apache-2.0
// Links that lead nowhere for this reader (EDT-059, EDT-060). A hidden page and a deleted
// page look exactly the same, so the mark says nothing about a page someone may not see.
import { Extension } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import type { Json } from '../../api';

const PAGE_ADDRESS = /^\/pages\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:[/?#]|$)/i;
const MAX_LINKS_CHECKED = 500;

export const refreshGoneLinks = new PluginKey<DecorationSet>('goneLinks');

interface JsonNode {
  marks?: { type: string; attrs?: { href?: string } }[];
  content?: JsonNode[];
}

export const pageOfAddress = (href: unknown) =>
  typeof href === 'string' ? PAGE_ADDRESS.exec(href)?.[1]?.toLowerCase() : undefined;

/** The pages this document links to, each once. */
export function linkedPages(document: Json): string[] {
  const pages = new Set<string>();
  const visit = (node: JsonNode) => {
    node.marks?.forEach((mark) => {
      const page = mark.type === 'link' ? pageOfAddress(mark.attrs?.href) : undefined;
      if (page) pages.add(page);
    });
    node.content?.forEach(visit);
  };
  visit(document as JsonNode);
  return [...pages].slice(0, MAX_LINKS_CHECKED);
}

interface GoneLinkOptions {
  gone: () => Set<string>;
  explanation: string;
}

function explanationAfter(text: string) {
  const note = document.createElement('span');
  note.className = 'visually-hidden';
  note.textContent = ` (${text})`;
  return note;
}

function decorationsOf(doc: ProseNode, { gone, explanation }: GoneLinkOptions): DecorationSet {
  const pages = gone();
  const found: Decoration[] = [];
  doc.descendants((node, pos) => {
    const link = node.marks.find((mark) => mark.type.name === 'link');
    const page = pageOfAddress(link?.attrs.href);
    if (!page || !pages.has(page)) return;
    const end = pos + node.nodeSize;
    found.push(Decoration.inline(pos, end, { class: 'link-gone', title: explanation }));
    found.push(Decoration.widget(end, () => explanationAfter(explanation), { side: -1 }));
  });
  return DecorationSet.create(doc, found);
}

export const GoneLinks = Extension.create<GoneLinkOptions>({
  name: 'goneLinks',
  addOptions() {
    return { gone: () => new Set(), explanation: '' };
  },
  addProseMirrorPlugins() {
    const options = this.options;
    return [
      new Plugin<DecorationSet>({
        key: refreshGoneLinks,
        state: {
          init: (_, state) => decorationsOf(state.doc, options),
          apply: (tr, old) => (tr.getMeta(refreshGoneLinks) ? decorationsOf(tr.doc, options) : old),
        },
        props: {
          decorations: (state) => refreshGoneLinks.getState(state),
        },
      }),
    ];
  },
});
