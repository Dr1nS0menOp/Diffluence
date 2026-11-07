// SPDX-License-Identifier: Apache-2.0
// Code blocks with a title, line numbers and a Copy button. This view is plain DOM
// because React node views force a text wrapping style on their content.
import { useEffect } from 'react';
import type { Editor, NodeViewRendererProps } from '@tiptap/core';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { common, createLowlight } from 'lowlight';
import i18n from '../../i18n';

const COPY_MESSAGE_MS = 2000;
const TAB_AS_SPACES = '  ';
const MAX_COLOURED_CHARACTERS = 200_000;
const MIN_DETECTED_CHARACTERS = 20;

const lowlight = createLowlight(common);
const COMMON_LANGUAGES = lowlight.listLanguages();

// Guessing tries every language it knows on each keystroke, so it stays with the
// common ones, whatever else was loaded later (spec 017, D13).
const highlighter = {
  ...lowlight,
  highlightAuto: (value: string) => lowlight.highlightAuto(value, { subset: COMMON_LANGUAGES }),
};

// A private cache: all languages are one large file, which loads once.
let allLanguages: Promise<void> | undefined;

/** Loads every language. They live in a file of their own, which most pages never need. */
export function loadAllLanguages(): Promise<void> {
  allLanguages ??= import('./code-languages').then(
    ({ all }) => lowlight.register(all),
    (error: unknown) => {
      allLanguages = undefined;
      throw error;
    },
  );
  return allLanguages;
}

/** What highlighting makes of a text: a tree, which is data and never markup. */
export type HighlightedText = ReturnType<typeof lowlight.highlight>;

/**
 * Colours a text file by its extension, which highlight.js knows as a language name.
 * Null means plain: an unknown type, or too long to colour in the browser (spec 024, D20).
 */
export async function highlightFile(filename: string, text: string): Promise<HighlightedText | null> {
  const dot = filename.lastIndexOf('.');
  if (dot <= 0 || text.length > MAX_COLOURED_CHARACTERS) return null;
  await loadAllLanguages();
  const extension = filename.slice(dot + 1).toLowerCase();
  return lowlight.registered(extension) ? lowlight.highlight(extension, text) : null;
}

export const languageNames = (): string[] => lowlight.listLanguages();
export const isLanguageName = (name: string): boolean => lowlight.registered(name);

/** The languages a document uses, in order of first use. */
export function languagesOf(doc: ProseNode): string[] {
  const found = new Set<string>();
  doc.descendants((node) => {
    const { language } = node.attrs as { language?: string | null };
    if (node.type.name === 'codeBlock' && language) found.add(language);
    return true;
  });
  return [...found];
}

/** Colours come by decorations, which only look again when a block changes. */
function recolour(editor: Editor) {
  const { tr, doc } = editor.state;
  doc.descendants((node, pos) => {
    if (node.type.name === 'codeBlock') tr.setNodeMarkup(pos, undefined, node.attrs);
  });
  editor.view.dispatch(tr.setMeta('addToHistory', false).setMeta('preventUpdate', true));
}

/** A block in a language that is not loaded yet shows plain, then gets its colours. */
export function useCodeColours(editor: Editor | null) {
  useEffect(() => {
    if (!editor) return;
    let done = false;
    const check = () => {
      if (done || languagesOf(editor.state.doc).every(isLanguageName)) return;
      done = true;
      loadAllLanguages().then(
        () => {
          if (!editor.isDestroyed) recolour(editor);
        },
        () => {
          // The text stays readable without colours, and the next edit tries again.
          done = false;
        },
      );
    };
    check();
    editor.on('update', check);
    return () => {
      editor.off('update', check);
    };
  }, [editor]);
}

function element<K extends keyof HTMLElementTagNameMap>(tag: K, className = ''): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  el.className = className;
  return el;
}

function renderLineNumbers(list: HTMLElement, text: string, first: number) {
  const lines = text.split('\n').length;
  while (list.children.length > lines) list.lastElementChild?.remove();
  while (list.children.length < lines) list.append(element('li'));
  Array.from(list.children).forEach((item, i) => {
    item.textContent = String(first + i);
  });
}

/** Adds or takes away two spaces at the start of every selected line (EDT-112). */
export function indentCodeLines(editor: Editor, direction: 'in' | 'out'): boolean {
  const { selection, tr } = editor.state;
  const { $from, $to } = selection;
  if ($from.parent.type.name !== 'codeBlock' || !$from.sameParent($to)) return false;
  const start = $from.start();
  const text = $from.parent.textContent;
  const lineStarts = [0];
  for (let i = text.indexOf('\n'); i !== -1; i = text.indexOf('\n', i + 1)) lineStarts.push(i + 1);
  const from = selection.from - start;
  const to = selection.to - start;
  const touched = lineStarts.filter((line, i) => {
    const end = lineStarts[i + 1] ?? text.length + 1;
    return line < Math.max(to, from + 1) && end > from;
  });
  for (const line of [...touched].reverse()) {
    if (direction === 'in') tr.insertText(TAB_AS_SPACES, start + line);
    else if (text.startsWith(TAB_AS_SPACES, line))
      tr.delete(start + line, start + line + TAB_AS_SPACES.length);
  }
  editor.view.dispatch(tr);
  return true;
}

/** A name for what a block holds. Guessing needs some text, and is only a suggestion. */
export function detectLanguage(text: string): string | null {
  if (text.trim().length < MIN_DETECTED_CHARACTERS) return null;
  return highlighter.highlightAuto(text).data?.language ?? null;
}

/** A button for readers. It says so when the browser refuses, because that can happen. */
function copyButton(text: () => string): { button: HTMLButtonElement; message: HTMLElement } {
  const button = element('button', 'ghost');
  button.type = 'button';
  button.textContent = i18n.t('editor.copy');
  const message = element('span', 'muted code-message');
  message.setAttribute('role', 'status');
  button.addEventListener('click', () => {
    navigator.clipboard.writeText(text()).then(
      () => (message.textContent = i18n.t('editor.copied')),
      () => (message.textContent = i18n.t('editor.copyFailed')),
    );
    setTimeout(() => (message.textContent = ''), COPY_MESSAGE_MS);
  });
  return { button, message };
}

function codeView({ node: initial, editor }: NodeViewRendererProps) {
  let node: ProseNode = initial;
  const frame = element('div', 'code-frame');
  const head = element('div', 'code-head');
  const title = element('span', 'code-title');
  const language = element('span', 'muted');
  const body = element('div', 'code-body');
  const numbers = element('ol', 'code-lines');
  const pre = element('pre');
  const code = element('code');
  head.contentEditable = 'false';
  numbers.contentEditable = 'false';
  numbers.setAttribute('aria-hidden', 'true');
  pre.spellcheck = false;
  pre.append(code);
  body.append(numbers, pre);
  head.append(title, language);
  frame.append(head, body);
  if (!editor.isEditable) {
    const { button, message } = copyButton(() => node.textContent);
    head.append(message, button);
  }

  const render = () => {
    const attrs = node.attrs as {
      language: string | null;
      title: string | null;
      line_numbers: boolean;
      wrap: boolean;
      line_start: number | null;
      folded: boolean;
    };
    title.textContent = attrs.title ?? '';
    language.textContent = attrs.language ?? '';
    frame.classList.toggle('wrap', attrs.wrap);
    head.hidden = editor.isEditable && !attrs.title;
    numbers.hidden = !attrs.line_numbers;
    if (attrs.line_numbers) renderLineNumbers(numbers, node.textContent, attrs.line_start ?? 1);
    // Hidden text cannot be edited, so a block only folds for readers (spec 027, D19).
    if (!editor.isEditable && attrs.folded && !opened) fold();
  };
  const fold = () => {
    if (unfold.isConnected) return;
    body.hidden = true;
    unfold.textContent = i18n.t('editor.codeShow', { count: node.textContent.split('\n').length });
    frame.append(unfold);
  };
  let opened = false;
  const unfold = element('button', 'ghost code-unfold');
  unfold.type = 'button';
  unfold.addEventListener('click', () => {
    opened = true;
    body.hidden = false;
    unfold.remove();
  });
  render();

  return {
    dom: frame,
    contentDOM: code,
    update(updated: ProseNode) {
      if (updated.type !== node.type) return false;
      node = updated;
      render();
      return true;
    },
    ignoreMutation: (mutation: { target: Node }) => !code.contains(mutation.target),
    stopEvent: (event: Event) =>
      event.target instanceof Node && (head.contains(event.target) || unfold.contains(event.target)),
  };
}

export const CodeBlock = CodeBlockLowlight.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      title: { default: null },
      line_numbers: { default: false },
      wrap: { default: false },
      line_start: { default: null },
      folded: { default: false },
    };
  },
  addNodeView() {
    return codeView;
  },
  addKeyboardShortcuts() {
    return {
      ...this.parent?.(),
      // Escape moves to the tools (keys.ts), so Tab here is no keyboard trap.
      Tab: () => {
        if (!this.editor.isActive(this.name)) return false;
        if (!this.editor.state.selection.empty) return indentCodeLines(this.editor, 'in');
        return this.editor.commands.insertContent({ type: 'text', text: TAB_AS_SPACES });
      },
      'Shift-Tab': () => this.editor.isActive(this.name) && indentCodeLines(this.editor, 'out'),
    };
  },
}).configure({ lowlight: highlighter });
