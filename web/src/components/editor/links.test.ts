// SPDX-License-Identifier: Apache-2.0
// Spec 023: the kinds of link, and where a link can lead.
import { describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import { NodeSelection } from '@tiptap/pm/state';
import '../../i18n';
import type { Json } from '../../api';
import { documentExtensions } from './extensions';
import { linkedPages } from './gone-links';
import {
  addressOf,
  currentLink,
  insertPageLink,
  insertWordsLink,
  linkedPageOf,
  targetsOfDocument,
  unlink,
} from './link-commands';
import { pageAddress } from './page-links';

const page = '11111111-1111-4111-8111-111111111111';

function editorWith(...inline: object[]) {
  return new Editor({
    extensions: documentExtensions(),
    content: { type: 'doc', content: [{ type: 'paragraph', content: inline }] },
  });
}

const pageNode = (attrs: object = {}) => ({ type: 'pageLink', attrs: { page_id: page, ...attrs } });
const firstInline = (editor: Editor) => editor.state.doc.firstChild?.firstChild;

describe('the link at the cursor', () => {
  it('knows a page link, its place and how it shows', () => {
    const editor = editorWith(pageNode({ display: 'tag', fragment: 'steps' }));
    editor.view.dispatch(editor.state.tr.setSelection(NodeSelection.create(editor.state.doc, 1)));
    const link = currentLink(editor);
    expect(link).toEqual({ kind: 'page', page, fragment: 'steps', display: 'tag' });
    expect(link && addressOf(link)).toBe(`/pages/${page}#steps`);
  });

  it('reads a link with words as a link to a page too', () => {
    expect(linkedPageOf({ kind: 'mark', href: `/pages/${page}#steps` })).toEqual({ page, fragment: 'steps' });
    expect(linkedPageOf({ kind: 'mark', href: 'https://example.com' })).toBeNull();
  });

  it('has no address for a page that does not exist yet', () => {
    expect(addressOf({ kind: 'placeholder', title: 'Notes' })).toBeNull();
  });

  it('builds an address without a place', () => {
    expect(pageAddress(page)).toBe(`/pages/${page}`);
    expect(pageAddress(page, null)).toBe(`/pages/${page}`);
  });
});

describe('changing a link', () => {
  it('stores the id and never the title', () => {
    const editor = editorWith({ type: 'text', text: 'x' });
    insertPageLink(editor, page, null, 'title');
    const stored = JSON.stringify(editor.getJSON());
    expect(stored).toContain(page);
    expect(stored).not.toContain('title":"');
  });

  it('turns a link with words into a tag', () => {
    const editor = editorWith({
      type: 'text',
      text: 'words',
      marks: [{ type: 'link', attrs: { href: `/pages/${page}` } }],
    });
    editor.commands.setTextSelection(3);
    insertPageLink(editor, page, null, 'tag');
    expect(firstInline(editor)?.type.name).toBe('pageLink');
    expect(firstInline(editor)?.attrs.display).toBe('tag');
    expect(editor.state.doc.textContent).toBe('');
  });

  it('turns a link into a card, which is a block', () => {
    const editor = editorWith(pageNode());
    editor.view.dispatch(editor.state.tr.setSelection(NodeSelection.create(editor.state.doc, 1)));
    insertPageLink(editor, page, null, 'card');
    const types: string[] = [];
    editor.state.doc.forEach((node) => types.push(node.type.name));
    expect(types).toContain('pageCard');
  });

  it('keeps words typed by the writer as a plain link', () => {
    const editor = editorWith();
    insertWordsLink(editor, `/pages/${page}`, 'see the plan');
    expect(editor.state.doc.textContent).toBe('see the plan');
  });

  it('leaves the words when a selected page link goes', () => {
    const editor = editorWith(pageNode());
    editor.view.dispatch(editor.state.tr.setSelection(NodeSelection.create(editor.state.doc, 1)));
    unlink(editor, 'Plans');
    expect(editor.state.doc.textContent).toBe('Plans');
    expect(firstInline(editor)?.type.name).toBe('text');
  });

  it('takes only the link away from text', () => {
    const editor = editorWith({
      type: 'text',
      text: 'words',
      marks: [{ type: 'link', attrs: { href: 'https://example.com' } }],
    });
    editor.commands.setTextSelection(3);
    unlink(editor, 'unused');
    expect(editor.state.doc.textContent).toBe('words');
    expect(editor.isActive('link')).toBe(false);
  });
});

describe('where a link can lead on the page being written', () => {
  it('lists headings and anchors, with the addresses readers get', () => {
    const doc = {
      type: 'doc',
      content: [
        { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'Next steps' }] },
        { type: 'paragraph', content: [{ type: 'anchor', attrs: { name: 'the-list' } }] },
      ],
    };
    expect(targetsOfDocument(doc as Json).map((target) => target.fragment)).toEqual([
      'next-steps',
      'the-list',
    ]);
  });
});

describe('the links a reader is checked for', () => {
  it('leaves the id of a page link to the facts, not to the check for words', () => {
    const doc = { type: 'doc', content: [{ type: 'paragraph', content: [pageNode()] }] };
    expect(linkedPages(doc as Json)).toEqual([]);
  });
});
