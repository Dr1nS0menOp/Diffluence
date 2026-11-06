// SPDX-License-Identifier: Apache-2.0
// The other blocks of wave E: page facts, excerpts, anchors, charts and media.
import { useTranslation } from 'react-i18next';
import { Node as DocumentNode, type Editor } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import {
  NodeViewContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  type ReactNodeViewProps,
} from '@tiptap/react';
import { Chart, chartData } from '../../chart';

const dataAttribute = (name: string) => ({
  default: null,
  parseHTML: (el: HTMLElement) => el.getAttribute(`data-${name}`),
  renderHTML: (attrs: Record<string, string | null>) =>
    attrs[name] ? { [`data-${name}`]: attrs[name] } : {},
});

const switchAttribute = (name: string) => ({
  default: false,
  parseHTML: (el: HTMLElement) => el.hasAttribute(`data-${name}`),
  renderHTML: (attrs: Record<string, boolean>) => (attrs[name] ? { [`data-${name}`]: '' } : {}),
});

function FactsView({ node, editor, updateAttributes }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const { hidden, facts_id: id } = node.attrs as { hidden: boolean; facts_id: string | null };
  if (hidden && !editor.isEditable) return <NodeViewWrapper className="page-facts hidden-block" />;
  return (
    <NodeViewWrapper className="page-facts" role="group" aria-label={t('insert.block_facts.name')}>
      {editor.isEditable && (
        <div className="dynamic-head" contentEditable={false}>
          <span>{t('insert.block_facts.name')}</span>
          <input
            aria-label={t('blocks.factsId')}
            placeholder={t('blocks.factsId')}
            value={id ?? ''}
            onChange={(e) => updateAttributes({ facts_id: e.target.value || null })}
          />
          <label className="check">
            <input
              type="checkbox"
              checked={hidden}
              onChange={(e) => updateAttributes({ hidden: e.target.checked })}
            />
            {t('blocks.hideFacts')}
          </label>
        </div>
      )}
      <NodeViewContent />
    </NodeViewWrapper>
  );
}

export const PageFacts = DocumentNode.create({
  name: 'pageFacts',
  group: 'block',
  content: 'table',
  defining: true,
  addAttributes() {
    return { facts_id: dataAttribute('facts_id'), hidden: switchAttribute('hidden') };
  },
  parseHTML() {
    return [{ tag: 'div[data-page-facts]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', { ...HTMLAttributes, 'data-page-facts': '' }, 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(FactsView);
  },
});

function ExcerptView({ node, editor, updateAttributes }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const { hidden, name } = node.attrs as { hidden: boolean; name: string };
  if (hidden && !editor.isEditable) return <NodeViewWrapper className="hidden-block" />;
  return (
    <NodeViewWrapper className={editor.isEditable ? 'excerpt editing' : 'excerpt'}>
      {editor.isEditable && (
        <div className="dynamic-head" contentEditable={false}>
          <span>{t('insert.block_excerpt.name')}</span>
          <input
            aria-label={t('blocks.excerptName')}
            value={name}
            onChange={(e) => updateAttributes({ name: e.target.value })}
          />
          <label className="check">
            <input
              type="checkbox"
              checked={hidden}
              onChange={(e) => updateAttributes({ hidden: e.target.checked })}
            />
            {t('blocks.hideExcerpt')}
          </label>
        </div>
      )}
      <NodeViewContent />
    </NodeViewWrapper>
  );
}

export const Excerpt = DocumentNode.create({
  name: 'excerpt',
  group: 'block',
  content: 'block+',
  defining: true,
  addAttributes() {
    return {
      name: {
        default: 'excerpt',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-excerpt'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-excerpt': attrs.name }),
      },
      hidden: switchAttribute('hidden'),
      synced: switchAttribute('synced'),
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-excerpt]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', HTMLAttributes, 0];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ExcerptView);
  },
});

/** An anchor name ends up in an address, so it keeps to letters, digits and dashes. */
export function anchorName(text: string): string {
  return text.toLowerCase().replace(/[^a-z0-9-]+/g, '-');
}

function AnchorView({ node, editor, updateAttributes }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const name = node.attrs.name as string;
  return (
    <NodeViewWrapper as="span" id={name} className="anchor-mark" contentEditable={false}>
      {editor.isEditable && (
        <input
          aria-label={t('blocks.anchorName')}
          value={name}
          onChange={(e) => updateAttributes({ name: anchorName(e.target.value) })}
        />
      )}
    </NodeViewWrapper>
  );
}

export const Anchor = DocumentNode.create({
  name: 'anchor',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      name: {
        default: 'anchor',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-anchor'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-anchor': attrs.name }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-anchor]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['span', HTMLAttributes];
  },
  addNodeView() {
    return ReactNodeViewRenderer(AnchorView);
  },
});

function rowsOfTable(table: ProseNode): string[][] {
  const rows: string[][] = [];
  table.forEach((row) => {
    const cells: string[] = [];
    row.forEach((cell) => cells.push(cell.textContent));
    rows.push(cells);
  });
  return rows;
}

/** A chart points at a table by its block id, and reads it from the document. */
function findTable(editor: Editor, blockId: string): ProseNode | null {
  let found: ProseNode | null = null;
  editor.state.doc.descendants((node) => {
    if (node.type.name === 'table' && node.attrs.block_id === blockId) found = node;
    return found === null;
  });
  return found;
}

const CHART_KINDS = ['bar', 'line', 'area', 'pie'];
const BLOCK_ID_LENGTH = 10;

function newBlockId(): string {
  const letters = 'abcdefghijklmnopqrstuvwxyz';
  const bytes = crypto.getRandomValues(new Uint8Array(BLOCK_ID_LENGTH));
  return Array.from(bytes, (byte) => letters[byte % letters.length]).join('');
}

/** Puts a chart under the table the cursor is in, and gives that table a block id. */
export function addChart(editor: Editor): boolean {
  const { $from } = editor.state.selection;
  for (let depth = $from.depth; depth > 0; depth -= 1) {
    const node = $from.node(depth);
    if (node.type.name === 'table') {
      const table = (node.attrs.block_id as string | null) ?? newBlockId();
      return editor
        .chain()
        .focus()
        .command(({ tr }) => {
          tr.setNodeAttribute($from.before(depth), 'block_id', table);
          return true;
        })
        .insertContentAt($from.after(depth), { type: 'chart', attrs: { table } })
        .run();
    }
  }
  return false;
}

function ChartView({ node, editor, updateAttributes }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const attrs = node.attrs as {
    table: string;
    kind: string;
    title: string | null;
    axis: number | null;
    series: number[] | null;
  };
  const table = findTable(editor, attrs.table);
  return (
    <NodeViewWrapper contentEditable={false}>
      {editor.isEditable && (
        <div className="dynamic-head">
          <input
            aria-label={t('chart.title')}
            placeholder={t('chart.title')}
            value={attrs.title ?? ''}
            onChange={(e) => updateAttributes({ title: e.target.value || null })}
          />
          <select
            aria-label={t('chart.kind')}
            value={attrs.kind}
            onChange={(e) => updateAttributes({ kind: e.target.value })}
          >
            {CHART_KINDS.map((kind) => (
              <option key={kind} value={kind}>
                {t(`chart.kind_${kind}`)}
              </option>
            ))}
          </select>
        </div>
      )}
      {table ? (
        <Chart
          data={chartData(rowsOfTable(table), attrs.axis ?? 0, attrs.series)}
          kind={attrs.kind}
          title={attrs.title ?? ''}
        />
      ) : (
        <p className="muted">{t('chart.noTable')}</p>
      )}
    </NodeViewWrapper>
  );
}

const plainAttribute = (fallback: unknown = null) => ({ default: fallback, rendered: false });

export const ChartNode = DocumentNode.create({
  name: 'chart',
  group: 'block',
  atom: true,
  draggable: true,
  addAttributes() {
    return {
      table: plainAttribute(''),
      kind: plainAttribute('bar'),
      title: plainAttribute(),
      axis: plainAttribute(),
      series: plainAttribute(),
      legend: plainAttribute(),
      smooth: plainAttribute(),
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-chart]' }];
  },
  renderHTML() {
    return ['div', { 'data-chart': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ChartView);
  },
});

function MediaView({ node }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const attrs = node.attrs as { attachment_id: string; kind: string; caption: string | null };
  const src = `/api/v1/attachments/${attrs.attachment_id}/content`;
  const Player = attrs.kind === 'audio' ? 'audio' : 'video';
  return (
    <NodeViewWrapper as="figure" className="doc-media" contentEditable={false}>
      <Player src={src} controls preload="metadata">
        <a href={`${src}?download=1`}>{t('blocks.downloadMedia')}</a>
      </Player>
      {attrs.caption && <figcaption>{attrs.caption}</figcaption>}
    </NodeViewWrapper>
  );
}

export const Media = DocumentNode.create({
  name: 'media',
  group: 'block',
  atom: true,
  draggable: true,
  addAttributes() {
    return {
      attachment_id: plainAttribute(),
      kind: plainAttribute('video'),
      caption: plainAttribute(),
    };
  },
  parseHTML() {
    return [{ tag: 'figure[data-media]' }];
  },
  renderHTML() {
    return ['figure', { 'data-media': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(MediaView);
  },
});
