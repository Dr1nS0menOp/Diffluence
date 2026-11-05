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
  const kind = node.attrs.kind as PanelKind;
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
