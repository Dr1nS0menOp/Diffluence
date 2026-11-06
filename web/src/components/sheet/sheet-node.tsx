// SPDX-License-Identifier: Apache-2.0
// A sheet inside a document, for what shows a stored version: history, compare and
// preview. It is always for reading. Changing one happens in SheetEditor.
import { useId } from 'react';
import { Node as DocumentNode } from '@tiptap/core';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import { useTranslation } from 'react-i18next';
import { SheetGrid } from './grid';
import { sheetOfAttrs } from './model';

function SheetBlock({ node }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <NodeViewWrapper contentEditable={false}>
      <SheetGrid id={id} label={t('sheet.label')} sheet={sheetOfAttrs(node.attrs)} />
    </NodeViewWrapper>
  );
}

export const SheetNode = DocumentNode.create({
  name: 'sheet',
  group: 'block',
  atom: true,
  addAttributes() {
    return {
      columns: { default: [], rendered: false },
      rows: { default: [], rendered: false },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-sheet]' }];
  },
  renderHTML() {
    return ['div', { 'data-sheet': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(SheetBlock, { stopEvent: () => true });
  },
});
