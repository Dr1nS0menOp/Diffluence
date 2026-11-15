// SPDX-License-Identifier: Apache-2.0
// A section of a template's text that a condition shows (spec 074, TPL-422, TPL-1013): a quiet
// outline with its condition written as a sentence on its first line. When pages are made, a
// true section is unwrapped and a false one removed, so it never reaches a reader.
import { createContext, useContext } from 'react';
import { useTranslation } from 'react-i18next';
import { mergeAttributes, Node as DocumentNode, type Editor } from '@tiptap/core';
import {
  NodeViewContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type ReactNodeViewProps,
} from '@tiptap/react';
import { conditionSentence, type Condition } from '../../template-conditions';
import { sentenceWords, type ConditionScope } from '../condition-editor';

/** What a section needs from the editor around it: the scope of its condition, and a way to change it. */
export interface SectionEditing {
  scope?: ConditionScope;
  edit: (pos: number) => void;
}

export const SectionContext = createContext<SectionEditing | null>(null);

function readCondition(raw: string | null): Condition | null {
  if (!raw) return null;
  try {
    return JSON.parse(raw) as Condition;
  } catch {
    // A condition that does not read is no condition; the server refuses it on save.
    return null;
  }
}

function ConditionalView({ node, editor, getPos }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const editing = useContext(SectionContext);
  const condition = node.attrs.condition as Condition | null;
  const sentence = condition
    ? conditionSentence(condition, sentenceWords(editing?.scope ?? { names: {} }))
    : t('templates.condition.sectionNoCondition');
  return (
    <NodeViewWrapper
      className="conditional-section"
      role="group"
      aria-label={t('templates.condition.sectionLabel')}
    >
      <p className="conditional-head" contentEditable={false}>
        <span>{sentence}</span>
        {editor.isEditable && editing && (
          <button
            type="button"
            className="ghost"
            onClick={() => {
              const pos = getPos();
              if (pos !== undefined) editing.edit(pos);
            }}
          >
            {t('templates.condition.change')}
          </button>
        )}
      </p>
      <NodeViewContent className="conditional-body" />
    </NodeViewWrapper>
  );
}

export const Conditional = DocumentNode.create({
  name: 'conditional',
  group: 'block',
  content: 'block+',
  defining: true,
  addAttributes() {
    return {
      condition: {
        default: null,
        parseHTML: (el: HTMLElement) => readCondition(el.getAttribute('data-condition')),
        renderHTML: (attrs: Record<string, unknown>) =>
          attrs.condition ? { 'data-condition': JSON.stringify(attrs.condition) } : {},
      },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-conditional]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', mergeAttributes(HTMLAttributes, { 'data-conditional': '' }), 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ConditionalView);
  },
});

/** Puts the selected blocks in a section with a condition. */
export function wrapInSection(editor: Editor, condition: Condition) {
  editor.chain().focus().wrapIn('conditional', { condition }).run();
}

/** Gives the section at pos another condition, or, with none, keeps its text and drops it. */
export function changeSection(editor: Editor, pos: number, condition: Condition | undefined) {
  const node = editor.state.doc.nodeAt(pos);
  if (node?.type.name !== 'conditional') return;
  const { tr } = editor.state;
  if (condition) tr.setNodeAttribute(pos, 'condition', condition);
  else tr.replaceWith(pos, pos + node.nodeSize, node.content);
  editor.view.dispatch(tr);
  editor.commands.focus();
}
