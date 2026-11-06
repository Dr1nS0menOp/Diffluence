// SPDX-License-Identifier: Apache-2.0
// The row of tools: one Tab stop, with arrow keys inside it (ARIA toolbar pattern).
import { useLayoutEffect, useRef, type FocusEvent, type KeyboardEvent, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { useEditorState, type Editor } from '@tiptap/react';
import {
  IconAlignCenter,
  IconAlignEnd,
  IconAlignStart,
  IconAttach,
  IconBold,
  IconBullet,
  IconClear,
  IconCode,
  IconCodeBlock,
  IconFullWidth,
  IconHighlight,
  IconIndent,
  IconItalic,
  IconLink,
  IconMore,
  IconNumbered,
  IconOutdent,
  IconPlus,
  IconQuote,
  IconRedo,
  IconSearch,
  IconStrike,
  IconSubscript,
  IconSuperscript,
  IconTable,
  IconTask,
  IconTextColor,
  IconUnderline,
  IconUndo,
} from '../icons';
import { Menu, MenuItem } from '../menu';
import { toggleList } from './block-commands';
import type { EditorActions } from './keys';
import { indentBlock, setAlignment, setTextStyle } from './marks';
import { MARK_COLORS, TEXT_COLORS } from './palette';
import { SelectionTools } from './selection-tools';
import { Tool, ToolSeparator, TOOL_ICON_SIZE } from './tool';

const HEADING_LEVELS = [1, 2, 3, 4, 5, 6] as const;
// Everything that can take focus in the row, except what sits in an open menu.
const CONTROLS = 'button:not(:disabled), select, input';

function controlsIn(root: HTMLElement | null): HTMLElement[] {
  const all = Array.from(root?.querySelectorAll<HTMLElement>(CONTROLS) ?? []);
  return all.filter((control) => !control.closest('[role=menu]'));
}

/** One Tab stop for the whole row. Tools come and go with the selection, so it runs every time. */
function useRovingFocus(root: RefObject<HTMLDivElement | null>, onLeave: () => void) {
  const current = useRef<HTMLElement | null>(null);

  useLayoutEffect(() => {
    const controls = controlsIn(root.current);
    const active = current.current && controls.includes(current.current) ? current.current : controls[0];
    controls.forEach((control) => (control.tabIndex = control === active ? 0 : -1));
    current.current = active ?? null;
  });

  const moveTo = (control: HTMLElement | undefined) => {
    if (!control) return;
    controlsIn(root.current).forEach((other) => (other.tabIndex = other === control ? 0 : -1));
    current.current = control;
    control.focus();
  };

  return {
    onFocus: (e: FocusEvent<HTMLDivElement>) => {
      if (e.target.closest('[role=menu]')) return;
      current.current = e.target;
    },
    onKeyDown: (e: KeyboardEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement;
      if (target.closest('[role=menu]')) return;
      if (e.key === 'Escape') {
        onLeave();
        return;
      }
      // A text field needs these keys itself, which is why those fields come last.
      if (target instanceof HTMLInputElement && target.type !== 'checkbox') return;
      const controls = controlsIn(root.current);
      const at = controls.indexOf(target);
      const steps: Record<string, HTMLElement | undefined> = {
        ArrowRight: controls[(at + 1) % controls.length],
        ArrowLeft: controls[(at - 1 + controls.length) % controls.length],
        Home: controls[0],
        End: controls[controls.length - 1],
      };
      if (!(e.key in steps)) return;
      e.preventDefault();
      moveTo(steps[e.key]);
    },
  };
}

function TextStyle({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const current = useEditorState({
    editor,
    selector: (s) => HEADING_LEVELS.find((level) => s.editor.isActive('heading', { level })) ?? 0,
  });
  return (
    <select
      className="tool-style"
      aria-label={t('editor.textStyle')}
      value={current}
      onChange={(e) => setTextStyle(editor, Number(e.target.value) || null)}
    >
      <option value={0}>{t('editor.paragraph')}</option>
      {HEADING_LEVELS.map((level) => (
        <option key={level} value={level}>
          {t(`editor.h${level}`)}
        </option>
      ))}
    </select>
  );
}

function InlineTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  return (
    <>
      <Tool editor={editor} label={t('editor.bold')} active="bold" run={() => chain().toggleBold().run()}>
        <IconBold size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.italic')}
        active="italic"
        run={() => chain().toggleItalic().run()}
      >
        <IconItalic size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.underline')}
        active="underline"
        run={() => chain().toggleUnderline().run()}
      >
        <IconUnderline size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.strike')}
        active="strike"
        run={() => chain().toggleStrike().run()}
      >
        <IconStrike size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.code')} active="code" run={() => chain().toggleCode().run()}>
        <IconCode size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function MoreFormatting({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  return (
    <Menu
      label={t('editor.moreFormatting')}
      trigger={<IconMore size={TOOL_ICON_SIZE} />}
      className="tool"
      align="start"
    >
      <MenuItem onClick={() => chain().toggleMark('subscript').run()}>
        <IconSubscript size={TOOL_ICON_SIZE} /> {t('editor.subscript')}
      </MenuItem>
      <MenuItem onClick={() => chain().toggleMark('superscript').run()}>
        <IconSuperscript size={TOOL_ICON_SIZE} /> {t('editor.superscript')}
      </MenuItem>
      <MenuItem onClick={() => chain().unsetAllMarks().run()}>
        <IconClear size={TOOL_ICON_SIZE} /> {t('editor.clearFormatting')}
      </MenuItem>
    </Menu>
  );
}

function TextColorMenu({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const current = useEditorState({
    editor,
    selector: (s) => s.editor.getAttributes('textColor').color as string | undefined,
  });
  const chain = () => editor.chain().focus();
  return (
    <Menu
      label={t('editor.textColor')}
      trigger={<IconTextColor size={TOOL_ICON_SIZE} />}
      className="tool"
      align="start"
    >
      <MenuItem
        role="menuitemradio"
        aria-checked={!current}
        onClick={() => chain().unsetMark('textColor').run()}
      >
        <IconTextColor size={TOOL_ICON_SIZE} /> {t('editor.defaultColor')}
      </MenuItem>
      {TEXT_COLORS.map((color) => (
        <MenuItem
          key={color}
          role="menuitemradio"
          aria-checked={current === color}
          onClick={() => chain().setMark('textColor', { color }).run()}
        >
          <span className={`text-${color}`}>
            <IconTextColor size={TOOL_ICON_SIZE} />
          </span>{' '}
          {t(`editor.colors.${color}`)}
        </MenuItem>
      ))}
    </Menu>
  );
}

function HighlightMenu({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const current = useEditorState({
    editor,
    selector: (s) => s.editor.getAttributes('highlight').color as string | undefined,
  });
  const chain = () => editor.chain().focus();
  return (
    <Menu
      label={t('editor.highlight')}
      trigger={<IconHighlight size={TOOL_ICON_SIZE} />}
      className="tool"
      align="start"
    >
      <MenuItem
        role="menuitemradio"
        aria-checked={!current}
        onClick={() => chain().unsetMark('highlight').run()}
      >
        <span className="swatch" /> {t('editor.noColor')}
      </MenuItem>
      {MARK_COLORS.map((color) => (
        <MenuItem
          key={color}
          role="menuitemradio"
          aria-checked={current === color}
          onClick={() => chain().setMark('highlight', { color }).run()}
        >
          <span className={`swatch mark-${color}`} /> {t(`editor.colors.${color}`)}
        </MenuItem>
      ))}
    </Menu>
  );
}

function AlignMenu({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const current = useEditorState({
    editor,
    selector: (s) => {
      const block = s.editor.isActive('heading') ? 'heading' : 'paragraph';
      return (s.editor.getAttributes(block).align as string | null | undefined) ?? null;
    },
  });
  const options = [
    { value: null, label: t('editor.alignStart'), icon: <IconAlignStart size={TOOL_ICON_SIZE} /> },
    { value: 'center', label: t('editor.alignCenter'), icon: <IconAlignCenter size={TOOL_ICON_SIZE} /> },
    { value: 'end', label: t('editor.alignEnd'), icon: <IconAlignEnd size={TOOL_ICON_SIZE} /> },
  ] as const;
  return (
    <Menu
      label={t('editor.align')}
      trigger={<IconAlignStart size={TOOL_ICON_SIZE} />}
      className="tool"
      align="start"
    >
      {options.map((option) => (
        <MenuItem
          key={option.label}
          role="menuitemradio"
          aria-checked={current === option.value}
          onClick={() => setAlignment(editor, option.value)}
        >
          {option.icon} {option.label}
        </MenuItem>
      ))}
    </Menu>
  );
}

function BlockTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  return (
    <>
      <Tool
        editor={editor}
        label={t('editor.bullet')}
        active="bulletList"
        run={() => toggleList(editor, 'bulletList')}
      >
        <IconBullet size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.ordered')}
        active="orderedList"
        run={() => toggleList(editor, 'orderedList')}
      >
        <IconNumbered size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.task')}
        active="taskList"
        run={() => toggleList(editor, 'taskList')}
      >
        <IconTask size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.quote')}
        active="blockquote"
        run={() => chain().toggleBlockquote().run()}
      >
        <IconQuote size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.codeBlock')}
        active="codeBlock"
        run={() => chain().toggleCodeBlock().run()}
      >
        <IconCodeBlock size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

export function Toolbar({
  editor,
  canAttach,
  actions,
  fileInput,
  fullWidth,
  onFullWidth,
  onInsert,
  onFiles,
}: {
  editor: Editor;
  canAttach: boolean;
  actions: EditorActions;
  fileInput: RefObject<HTMLInputElement | null>;
  fullWidth?: boolean;
  /** Left out where there is no page to widen, such as in a template. */
  onFullWidth?: () => void;
  onInsert: () => void;
  onFiles: (files: File[]) => void;
}) {
  const { t } = useTranslation();
  const root = useRef<HTMLDivElement>(null);
  const roving = useRovingFocus(root, () => editor.commands.focus());
  const chain = () => editor.chain().focus();
  return (
    <div
      ref={root}
      role="toolbar"
      aria-label={t('editor.toolbar')}
      className="toolbar"
      onFocus={roving.onFocus}
      onKeyDown={roving.onKeyDown}
    >
      <TextStyle editor={editor} />
      <InlineTools editor={editor} />
      <MoreFormatting editor={editor} />
      <TextColorMenu editor={editor} />
      <HighlightMenu editor={editor} />
      <ToolSeparator />
      <AlignMenu editor={editor} />
      <Tool editor={editor} label={t('editor.outdent')} run={() => indentBlock(editor, 'out')}>
        <IconOutdent size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.indent')} run={() => indentBlock(editor, 'in')}>
        <IconIndent size={TOOL_ICON_SIZE} />
      </Tool>
      <ToolSeparator />
      <BlockTools editor={editor} />
      <ToolSeparator />
      <Tool editor={editor} label={t('editor.link')} active="link" run={actions.openLink}>
        <IconLink size={TOOL_ICON_SIZE} />
      </Tool>
      {canAttach && (
        <>
          <Tool editor={editor} label={t('editor.attach')} run={actions.pickFiles}>
            <IconAttach size={TOOL_ICON_SIZE} />
          </Tool>
          <input
            ref={fileInput}
            type="file"
            multiple
            hidden
            aria-label={t('editor.attach')}
            onChange={(e) => {
              onFiles(Array.from(e.target.files ?? []));
              e.target.value = '';
            }}
          />
        </>
      )}
      <Tool editor={editor} label={t('editor.table')} run={actions.openTableSize}>
        <IconTable size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.insert')} run={onInsert}>
        <IconPlus size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.find')} run={actions.openFind}>
        <IconSearch size={TOOL_ICON_SIZE} />
      </Tool>
      <ToolSeparator />
      <Tool editor={editor} label={t('editor.undo')} run={() => chain().undo().run()}>
        <IconUndo size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.redo')} run={() => chain().redo().run()}>
        <IconRedo size={TOOL_ICON_SIZE} />
      </Tool>
      {onFullWidth && (
        <>
          <ToolSeparator />
          <Tool editor={editor} label={t('editor.fullWidth')} pressed={fullWidth ?? false} run={onFullWidth}>
            <IconFullWidth size={TOOL_ICON_SIZE} />
          </Tool>
        </>
      )}
      <SelectionTools editor={editor} actions={actions} />
    </div>
  );
}
