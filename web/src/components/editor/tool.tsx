// SPDX-License-Identifier: Apache-2.0
import type { ReactNode } from 'react';
import { useEditorState, type Editor } from '@tiptap/react';

export const TOOL_ICON_SIZE = 18;

/** A button in the row. Pressing it never takes the selection away from the text. */
export function Tool({
  editor,
  label,
  active,
  pressed,
  run,
  children,
}: {
  editor: Editor;
  label: string;
  active?: string;
  pressed?: boolean;
  run: () => void;
  children: ReactNode;
}) {
  const markIsOn = useEditorState({
    editor,
    selector: (s) => (active ? s.editor.isActive(active) : false),
  });
  const on = pressed ?? markIsOn;
  const isToggle = active !== undefined || pressed !== undefined;
  return (
    <button
      type="button"
      className={on ? 'tool on' : 'tool'}
      aria-label={label}
      aria-pressed={isToggle ? on : undefined}
      title={label}
      onMouseDown={(e) => e.preventDefault()}
      onClick={run}
    >
      {children}
    </button>
  );
}

export function ToolSeparator() {
  return <span className="tool-separator" />;
}
