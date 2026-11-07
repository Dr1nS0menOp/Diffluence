// SPDX-License-Identifier: Apache-2.0
// The other blocks of wave E: page facts, excerpts, anchors, charts and media.
import { useTranslation } from 'react-i18next';
import { Node as DocumentNode, type Editor } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import {
  NodeViewContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  useEditorState,
  type ReactNodeViewProps,
} from '@tiptap/react';
import { Chart, chartData, type ChartLook, type ChartTotal } from '../../chart';

export const dataAttribute = (name: string) => ({
  default: null,
  parseHTML: (el: HTMLElement) => el.getAttribute(`data-${name}`),
  renderHTML: (attrs: Record<string, string | null>) =>
    attrs[name] ? { [`data-${name}`]: attrs[name] } : {},
});

export const switchAttribute = (name: string) => ({
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

/** The charts that read the table at the cursor, last first, so that removing them keeps positions. */
export function chartsOfTable(editor: Editor): number[] {
  const { $from } = editor.state.selection;
  let id: unknown = null;
  for (let depth = $from.depth; depth > 0 && !id; depth -= 1) {
    if ($from.node(depth).type.name === 'table') id = $from.node(depth).attrs.block_id;
  }
  const found: number[] = [];
  if (!id) return found;
  editor.state.doc.descendants((node, pos) => {
    if (node.type.name === 'chart' && node.attrs.table === id) found.unshift(pos);
    return true;
  });
  return found;
}

export function removeCharts(editor: Editor) {
  const { tr, doc } = editor.state;
  chartsOfTable(editor).forEach((pos) => {
    const chart = doc.nodeAt(pos);
    if (chart) tr.delete(pos, pos + chart.nodeSize);
  });
  editor.view.dispatch(tr);
}

interface ChartAttrs extends ChartLook {
  table: string;
  kind: string;
  title: string | null;
  axis: number | null;
  series: number[] | null;
  total: ChartTotal | null;
}

const CHART_TOTALS: ChartTotal[] = ['sum', 'average', 'count'];

/** The settings of a chart. The table decides which columns there are to choose from. */
function ChartOptions({
  attrs,
  head,
  onChange,
}: {
  attrs: ChartAttrs;
  head: string[];
  onChange: (values: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const axis = attrs.axis ?? 0;
  const chosen = attrs.series ?? head.map((_, i) => i).filter((i) => i !== axis);
  const toggle = (column: number) => {
    const next = chosen.includes(column) ? chosen.filter((i) => i !== column) : [...chosen, column].sort();
    onChange({ series: next.length > 0 ? next : null });
  };
  const text = (key: 'x_title' | 'y_title', label: string) => (
    <input
      aria-label={label}
      placeholder={label}
      value={attrs[key] ?? ''}
      onChange={(e) => onChange({ [key]: e.target.value || null })}
    />
  );
  return (
    <details className="chart-options">
      <summary>{t('chart.options')}</summary>
      <label>
        {t('chart.axis')}
        <select value={axis} onChange={(e) => onChange({ axis: Number(e.target.value), series: null })}>
          {head.map((name, i) => (
            <option key={`${name}-${i}`} value={i}>
              {name || t('chart.column', { number: i + 1 })}
            </option>
          ))}
        </select>
      </label>
      <fieldset>
        <legend>{t('chart.data')}</legend>
        {head.map(
          (name, i) =>
            i !== axis && (
              <label key={`${name}-${i}`} className="check">
                <input type="checkbox" checked={chosen.includes(i)} onChange={() => toggle(i)} />
                {name || t('chart.column', { number: i + 1 })}
              </label>
            ),
        )}
      </fieldset>
      <label>
        {t('chart.total')}
        <select value={attrs.total ?? ''} onChange={(e) => onChange({ total: e.target.value || null })}>
          <option value="">{t('chart.total_none')}</option>
          {CHART_TOTALS.map((total) => (
            <option key={total} value={total}>
              {t(`chart.total_${total}`)}
            </option>
          ))}
        </select>
      </label>
      {text('x_title', t('chart.xTitle'))}
      {text('y_title', t('chart.yTitle'))}
      <label className="check">
        <input
          type="checkbox"
          checked={!attrs.no_legend}
          onChange={(e) => onChange({ no_legend: !e.target.checked })}
        />
        {t('chart.legend')}
      </label>
      <label>
        {t('chart.legendPosition')}
        <select
          value={attrs.legend_position ?? 'bottom'}
          onChange={(e) => onChange({ legend_position: e.target.value })}
        >
          {['top', 'bottom', 'start', 'end'].map((position) => (
            <option key={position} value={position}>
              {t(`chart.legend_${position}`)}
            </option>
          ))}
        </select>
      </label>
      <label>
        {t('chart.height')}
        <select value={attrs.height ?? ''} onChange={(e) => onChange({ height: e.target.value || null })}>
          <option value="small">{t('chart.height_small')}</option>
          <option value="">{t('chart.height_medium')}</option>
          <option value="large">{t('chart.height_large')}</option>
        </select>
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={attrs.smooth ?? false}
          onChange={(e) => onChange({ smooth: e.target.checked })}
        />
        {t('chart.smooth')}
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={attrs.points ?? false}
          onChange={(e) => onChange({ points: e.target.checked })}
        />
        {t('chart.points')}
      </label>
    </details>
  );
}

function ChartView({ node, editor, updateAttributes }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const attrs = node.attrs as ChartAttrs;
  // The chart is a node of its own, so a change in the table reaches it only through the editor.
  const shown = useEditorState({
    editor,
    selector: (s) => {
      const found = findTable(s.editor, attrs.table);
      return found ? JSON.stringify(rowsOfTable(found)) : null;
    },
  });
  const table = shown !== null;
  const rows = shown === null ? [] : (JSON.parse(shown) as string[][]);
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
          {table && <ChartOptions attrs={attrs} head={rows[0] ?? []} onChange={updateAttributes} />}
        </div>
      )}
      {table ? (
        <Chart
          data={chartData(rows, attrs.axis ?? 0, attrs.series, attrs.total)}
          kind={attrs.kind}
          title={attrs.title ?? ''}
          look={attrs}
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
      no_legend: plainAttribute(),
      smooth: plainAttribute(),
      points: plainAttribute(),
      total: plainAttribute(),
      height: plainAttribute(),
      legend_position: plainAttribute(),
      x_title: plainAttribute(),
      y_title: plainAttribute(),
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
