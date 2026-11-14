// SPDX-License-Identifier: Apache-2.0
// Drawings made from data (ADR-026 allows SVG here). The diagram of a workflow is read-only
// and drawn from its form (spec 064, DOC-911): each state a box, each transition an arrow
// with the name of its button. It is hidden from screen readers, because the list of
// transitions beside it says the same in words.
import { useId } from 'react';
import type { WorkflowDefinition } from '../api';

const BOX_WIDTH = 180;
const BOX_HEIGHT = 40;
const ROW_GAP = 36;
const LANE = 26;
const MARGIN = 12;
const LABEL_WIDTH = 170;
const LABEL_RISE = 4;
const TEXT_INSET = 10;
const MAX_LABEL = 26;
const ELLIPSIS = '…';
const ARROW_SIZE = 6;

function shorten(text: string): string {
  return text.length > MAX_LABEL ? `${text.slice(0, MAX_LABEL - 1)}${ELLIPSIS}` : text;
}

/** The states in a column, and the transitions as arcs on the side, one lane each. */
export function WorkflowDiagram({ definition }: { definition: WorkflowDefinition }) {
  const marker = `workflow-arrow-${useId().replace(/:/g, '')}`;
  const row = new Map(definition.states.map((s, i) => [s.id, i]));
  const top = (i: number) => MARGIN + i * (BOX_HEIGHT + ROW_GAP);
  const arrows = definition.transitions.filter((tr) => row.has(tr.from) && row.has(tr.to));
  const width = MARGIN * 2 + BOX_WIDTH + (arrows.length + 1) * LANE + LABEL_WIDTH;
  const height = Math.max(top(definition.states.length) - ROW_GAP + MARGIN, BOX_HEIGHT + MARGIN * 2);
  const edge = MARGIN + BOX_WIDTH;
  return (
    <svg className="workflow-diagram" viewBox={`0 0 ${width} ${height}`} aria-hidden="true" focusable="false">
      <defs>
        <marker
          id={marker}
          viewBox={`0 0 ${ARROW_SIZE * 2} ${ARROW_SIZE * 2}`}
          refX={ARROW_SIZE * 2}
          refY={ARROW_SIZE}
          markerWidth={ARROW_SIZE}
          markerHeight={ARROW_SIZE}
          orient="auto-start-reverse"
        >
          <path
            className="workflow-arrow-head"
            d={`M0,0 L${ARROW_SIZE * 2},${ARROW_SIZE} L0,${ARROW_SIZE * 2} z`}
          />
        </marker>
      </defs>
      {arrows.map((tr, i) => {
        const fromY = top(row.get(tr.from) ?? 0) + BOX_HEIGHT / 2;
        const toY = top(row.get(tr.to) ?? 0) + BOX_HEIGHT / 2;
        const out = edge + (i + 1) * LANE;
        const labelY = (fromY + toY) / 2 - LABEL_RISE;
        return (
          <g key={tr.id} className={tr.outcome ? 'workflow-step-system' : 'workflow-step'}>
            <path
              className="workflow-line"
              d={`M${edge},${fromY} H${out} V${toY} H${edge}`}
              markerEnd={`url(#${marker})`}
            />
            <text className="workflow-label" x={out + LABEL_RISE} y={labelY}>
              {shorten(tr.name)}
            </text>
          </g>
        );
      })}
      {definition.states.map((s, i) => (
        <g key={s.id} className={`workflow-box workflow-kind-${s.kind}`}>
          <rect x={MARGIN} y={top(i)} width={BOX_WIDTH} height={BOX_HEIGHT} rx={LABEL_RISE} />
          <text x={MARGIN + TEXT_INSET} y={top(i) + BOX_HEIGHT / 2 + LABEL_RISE}>
            {shorten(s.name)}
          </text>
        </g>
      ))}
    </svg>
  );
}
