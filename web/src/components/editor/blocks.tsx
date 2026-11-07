// SPDX-License-Identifier: Apache-2.0
// Blocks that give a page structure: panels, collapsible sections, columns and decisions.
import type { KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  mergeAttributes,
  Node as DocumentNode,
  textblockTypeInputRule,
  wrappingInputRule,
} from '@tiptap/core';
import {
  NodeViewContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type ReactNodeViewProps,
} from '@tiptap/react';
import {
  IconDecision,
  IconPanelError,
  IconPanelInfo,
  IconPanelNote,
  IconPanelSuccess,
  IconPanelWarning,
} from '../icons';
import { PANEL_TINT, type PanelKind } from './palette';

const PANEL_ICONS = {
  info: IconPanelInfo,
  note: IconPanelNote,
  success: IconPanelSuccess,
  warning: IconPanelWarning,
  error: IconPanelError,
};

function PanelView({ node }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const kind = node.attrs.kind as PanelKind | 'neutral';
  if (kind === 'neutral') {
    const { emoji, color } = node.attrs as { emoji: string | null; color: string | null };
    return (
      <NodeViewWrapper
        className={color ? `panel mark-${color}` : 'panel panel-neutral'}
        role="note"
        aria-label={t('editor.panels.neutral')}
      >
        <span className="panel-icon" contentEditable={false}>
          {emoji}
        </span>
        <NodeViewContent className="panel-body" />
      </NodeViewWrapper>
    );
  }
  const Icon = PANEL_ICONS[kind];
  return (
    <NodeViewWrapper
      className={`panel mark-${PANEL_TINT[kind]}`}
      role="note"
      aria-label={t(`editor.panels.${kind}`)}
    >
      <span className="panel-icon" contentEditable={false}>
        <Icon />
      </span>
      <NodeViewContent className="panel-body" />
    </NodeViewWrapper>
  );
}

export const Panel = DocumentNode.create({
  name: 'panel',
  group: 'block',
  content: 'block+',
  defining: true,
  addAttributes() {
    return {
      kind: {
        default: 'info',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-panel'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-panel': attrs.kind }),
      },
      emoji: textAttribute('emoji'),
      color: textAttribute('color'),
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-panel]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes), 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(PanelView);
  },
  addInputRules() {
    return [wrappingInputRule({ find: /^!!\s$/, type: this.type })];
  },
});

const textAttribute = (name: string) => ({
  default: null,
  parseHTML: (el: HTMLElement) => el.getAttribute(`data-${name}`),
  renderHTML: (attrs: Record<string, string | null>) =>
    attrs[name] ? { [`data-${name}`]: attrs[name] } : {},
});

function SpotlightView({ node, editor }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const {
    color,
    button_text: text,
    button_href: href,
  } = node.attrs as {
    color: string | null;
    button_text: string | null;
    button_href: string | null;
  };
  return (
    <NodeViewWrapper
      className={color ? `spotlight mark-${color}` : 'spotlight'}
      role="group"
      aria-label={t('insert.spotlight.name')}
    >
      <NodeViewContent className="spotlight-body" />
      {text && href && (
        <a
          className="button spotlight-button"
          contentEditable={false}
          href={editor.isEditable ? undefined : href}
          rel="noopener noreferrer"
        >
          {text}
        </a>
      )}
    </NodeViewWrapper>
  );
}

/** A wide band for an important message, with an optional button (EDT-123). */
export const Spotlight = DocumentNode.create({
  name: 'spotlight',
  group: 'block',
  content: 'block+',
  defining: true,
  addAttributes() {
    return {
      color: textAttribute('color'),
      button_text: textAttribute('button_text'),
      button_href: textAttribute('button_href'),
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-spotlight]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-spotlight': '' }), 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(SpotlightView);
  },
});

function UnknownBlockView({ node }: ReactNodeViewProps) {
  const { t } = useTranslation();
  return (
    <NodeViewWrapper className="unknown-block" contentEditable={false}>
      {t('editor.unknownBlock', { name: node.attrs.name as string })}
    </NodeViewWrapper>
  );
}

/** A block from a newer release: shown as a note, and kept as it came (EDT-158, spec 027, D18). */
export const UnknownBlock = DocumentNode.create({
  name: 'unknownBlock',
  group: 'block',
  atom: true,
  draggable: true,
  addAttributes() {
    return { name: { default: '', rendered: false }, original: { default: '', rendered: false } };
  },
  parseHTML() {
    return [{ tag: 'div[data-unknown-block]' }];
  },
  renderHTML() {
    return ['div', { 'data-unknown-block': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(UnknownBlockView);
  },
});

interface StoredNode {
  type?: string;
  content?: StoredNode[];
  [key: string]: unknown;
}

/** Keeps what this release cannot show, so that opening a document never loses a block. */
export function keepUnknownBlocks(node: StoredNode, known: (type: string) => boolean): StoredNode {
  if (!node.content) return node;
  const content = node.content.map((child) => {
    if (child.type && known(child.type)) return keepUnknownBlocks(child, known);
    return {
      type: 'unknownBlock',
      attrs: { name: String(child.type ?? ''), original: JSON.stringify(child) },
    };
  });
  return { ...node, content };
}

function ExpandView({ node, editor, getPos, updateAttributes }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const { title, open } = node.attrs as { title: string | null; open: boolean };
  const enterContent = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const pos = getPos();
    if (pos !== undefined) editor.commands.focus(pos + 2);
  };
  if (editor.isEditable) {
    return (
      <NodeViewWrapper className="expand editing">
        <input
          className="expand-title"
          value={title ?? ''}
          aria-label={t('editor.expandTitle')}
          placeholder={t('editor.expandTitle')}
          onChange={(e) => updateAttributes({ title: e.target.value || null })}
          onKeyDown={enterContent}
        />
        <NodeViewContent className="expand-body" />
      </NodeViewWrapper>
    );
  }
  return (
    <NodeViewWrapper className="expand">
      <details open={open}>
        <summary>{title || t('editor.expandDefault')}</summary>
        <NodeViewContent className="expand-body" />
      </details>
    </NodeViewWrapper>
  );
}

export const Expand = DocumentNode.create({
  name: 'expand',
  group: 'block',
  content: 'block+',
  defining: true,
  isolating: true,
  addAttributes() {
    return {
      title: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-title'),
        renderHTML: (attrs: Record<string, string | null>) =>
          attrs.title ? { 'data-title': attrs.title } : {},
      },
      open: {
        default: false,
        parseHTML: (el: HTMLElement) => el.hasAttribute('data-open'),
        renderHTML: (attrs: Record<string, boolean>) => (attrs.open ? { 'data-open': '' } : {}),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-expand]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-expand': '' }), 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ExpandView);
  },
});

export const Columns = DocumentNode.create({
  name: 'columns',
  group: 'block',
  content: 'column{2,5}',
  defining: true,
  isolating: true,
  addAttributes() {
    return {
      layout: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-layout'),
        renderHTML: (attrs: Record<string, string | null>) =>
          attrs.layout ? { 'data-layout': attrs.layout } : {},
      },
      width: textAttribute('width'),
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-columns]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-columns': '', class: 'doc-columns' }), 0];
  },
});

export const Column = DocumentNode.create({
  name: 'column',
  content: 'block+',
  isolating: true,
  parseHTML() {
    return [{ tag: 'div[data-column]' }];
  },
  renderHTML() {
    return ['div', { 'data-column': '', class: 'doc-column' }, 0];
  },
});

function DecisionView() {
  return (
    <NodeViewWrapper className="decision">
      <span className="decision-icon" contentEditable={false}>
        <IconDecision />
      </span>
      <NodeViewContent className="decision-text" />
    </NodeViewWrapper>
  );
}

export const Decision = DocumentNode.create({
  name: 'decision',
  group: 'block',
  content: 'inline*',
  defining: true,
  parseHTML() {
    return [{ tag: 'div[data-decision]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-decision': '' }), 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(DecisionView);
  },
  addInputRules() {
    return [textblockTypeInputRule({ find: /^<>\s$/, type: this.type })];
  },
  addKeyboardShortcuts() {
    return {
      Enter: () => {
        const { $from, empty } = this.editor.state.selection;
        const isEmptyDecision = empty && $from.parent.type === this.type && $from.parent.childCount === 0;
        return isEmptyDecision && this.editor.commands.setParagraph();
      },
    };
  },
});
