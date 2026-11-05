// SPDX-License-Identifier: Apache-2.0
// Code blocks with a title, line numbers and a Copy button. This view is plain DOM
// because React node views force a text wrapping style on their content.
import type { NodeViewRendererProps } from '@tiptap/core';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { common, createLowlight } from 'lowlight';
import i18n from '../../i18n';

const COPY_MESSAGE_MS = 2000;
const TAB_AS_SPACES = '  ';

const lowlight = createLowlight(common);

export const CODE_LANGUAGES = lowlight.listLanguages().sort();

function element<K extends keyof HTMLElementTagNameMap>(tag: K, className = ''): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  el.className = className;
  return el;
}

function renderLineNumbers(list: HTMLElement, text: string) {
  const lines = text.split('\n').length;
  while (list.children.length > lines) list.lastElementChild?.remove();
  while (list.children.length < lines) {
    const item = element('li');
    item.textContent = String(list.children.length + 1);
    list.append(item);
  }
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
    };
    title.textContent = attrs.title ?? '';
    language.textContent = attrs.language ?? '';
    frame.classList.toggle('wrap', attrs.wrap);
    head.hidden = editor.isEditable && !attrs.title;
    numbers.hidden = !attrs.line_numbers;
    if (attrs.line_numbers) renderLineNumbers(numbers, node.textContent);
  };
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
    stopEvent: (event: Event) => event.target instanceof Node && head.contains(event.target),
  };
}

export const CodeBlock = CodeBlockLowlight.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      title: { default: null },
      line_numbers: { default: false },
      wrap: { default: false },
    };
  },
  addNodeView() {
    return codeView;
  },
  addKeyboardShortcuts() {
    return {
      ...this.parent?.(),
      // Escape moves to the tools (keys.ts), so Tab here is no keyboard trap.
      Tab: () =>
        this.editor.isActive(this.name) &&
        this.editor.commands.insertContent({ type: 'text', text: TAB_AS_SPACES }),
    };
  },
}).configure({ lowlight });
