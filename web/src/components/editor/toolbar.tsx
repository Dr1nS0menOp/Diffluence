// SPDX-License-Identifier: Apache-2.0
// The row of tools: one Tab stop, with arrow keys inside it (ARIA toolbar pattern).
import { useRef, useState, type ReactNode, type RefObject } from 'react';
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
  IconChevronDown,
  IconCode,
  IconCodeBlock,
  IconEmoji,
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
import { Menu, MenuItem, MenuSeparator } from '../menu';
import { toggleList } from './block-commands';
import type { EditorActions } from './keys';
import { indentBlock, setAlignment, setDirection, setTextStyle } from './marks';
import { MARK_COLORS, TEXT_COLORS } from './palette';
import { useRovingFocus } from './roving-focus';
import { SelectionTools } from './selection-tools';
import { Tool, ToolSeparator, TOOL_ICON_SIZE } from './tool';
import { useToolFolding } from './tool-fold';

const HEADING_LEVELS = [1, 2, 3, 4, 5, 6] as const;

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

function MoreFormatting({ editor, actions }: { editor: Editor; actions: EditorActions }) {
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
      <MenuItem onClick={actions.openEmojiPicker}>
        <IconEmoji size={TOOL_ICON_SIZE} /> {t('editor.emojiPicker')}
      </MenuItem>
    </Menu>
  );
}

export function TextColorMenu({ editor }: { editor: Editor }) {
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

export function HighlightMenu({ editor }: { editor: Editor }) {
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
      const attrs = s.editor.getAttributes(block) as { align?: string | null; dir?: string | null };
      return { align: attrs.align ?? null, dir: attrs.dir ?? null };
    },
  });
  const directions = [
    { value: null, label: t('editor.directionPage') },
    { value: 'ltr', label: t('editor.directionLtr') },
    { value: 'rtl', label: t('editor.directionRtl') },
  ] as const;
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
          aria-checked={current.align === option.value}
          onClick={() => setAlignment(editor, option.value)}
        >
          {option.icon} {option.label}
        </MenuItem>
      ))}
      <MenuSeparator />
      {directions.map((option) => (
        <MenuItem
          key={option.label}
          role="menuitemradio"
          aria-checked={current.dir === option.value}
          onClick={() => setDirection(editor, option.value)}
        >
          {option.label}
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

  const [showsMore, setShowsMore] = useState(false);
  const roving = useRovingFocus(root, () => editor.commands.focus());
  const chain = () => editor.chain().focus();
  // In the order of the row, which is also the order in which they fold back from the end.
  const groups: { id: string; tools: ReactNode }[] = [
    { id: 'style', tools: <TextStyle editor={editor} /> },
    { id: 'inline', tools: <InlineTools editor={editor} /> },
    {
      id: 'colour',
      tools: (
        <>
          <MoreFormatting editor={editor} actions={actions} />
          <TextColorMenu editor={editor} />
          <HighlightMenu editor={editor} />
          <ToolSeparator />
        </>
      ),
    },
    {
      id: 'layout',
      tools: (
        <>
          <AlignMenu editor={editor} />
          <Tool editor={editor} label={t('editor.outdent')} run={() => indentBlock(editor, 'out')}>
            <IconOutdent size={TOOL_ICON_SIZE} />
          </Tool>
          <Tool editor={editor} label={t('editor.indent')} run={() => indentBlock(editor, 'in')}>
            <IconIndent size={TOOL_ICON_SIZE} />
          </Tool>
          <ToolSeparator />
        </>
      ),
    },
    {
      id: 'blocks',
      tools: (
        <>
          <BlockTools editor={editor} />
          <ToolSeparator />
        </>
      ),
    },
    {
      id: 'insert',
      tools: (
        <>
          <Tool editor={editor} label={t('editor.link')} active="link" run={actions.openLink}>
            <IconLink size={TOOL_ICON_SIZE} />
          </Tool>
          {canAttach && (
            <Tool editor={editor} label={t('editor.attach')} run={actions.pickFiles}>
              <IconAttach size={TOOL_ICON_SIZE} />
            </Tool>
          )}
          <Tool editor={editor} label={t('editor.table')} run={actions.openTableSize}>
            <IconTable size={TOOL_ICON_SIZE} />
          </Tool>

          <Tool editor={editor} label={t('editor.insert')} run={onInsert}>
            <IconPlus size={TOOL_ICON_SIZE} />
          </Tool>
        </>
      ),
    },
    {
      id: 'find',
      tools: (
        <>
          <Tool editor={editor} label={t('editor.find')} run={actions.openFind}>
            <IconSearch size={TOOL_ICON_SIZE} />
          </Tool>
          <ToolSeparator />
        </>
      ),
    },
    {
      id: 'history',
      tools: (
        <>
          <Tool editor={editor} label={t('editor.undo')} run={() => chain().undo().run()}>
            <IconUndo size={TOOL_ICON_SIZE} />
          </Tool>
          <Tool editor={editor} label={t('editor.redo')} run={() => chain().redo().run()}>
            <IconRedo size={TOOL_ICON_SIZE} />
          </Tool>
        </>
      ),
    },
    ...(onFullWidth
      ? [
          {
            id: 'width',
            tools: (
              <>
                <ToolSeparator />
                <Tool
                  editor={editor}
                  label={t('editor.fullWidth')}
                  pressed={fullWidth ?? false}
                  run={onFullWidth}
                >
                  <IconFullWidth size={TOOL_ICON_SIZE} />
                </Tool>
              </>
            ),
          },
        ]
      : []),
  ];
  const folded = useToolFolding(root, groups.length);
  const shown = groups.slice(0, groups.length - folded);
  const hidden = groups.slice(groups.length - folded);
  return (
    <div
      ref={root}
      role="toolbar"
      aria-label={t('editor.toolbar')}
      className="toolbar"
      onFocus={roving.onFocus}
      onKeyDown={roving.onKeyDown}
    >
      {shown.map((group) => (
        <span key={group.id} className="tool-group">
          {group.tools}
        </span>
      ))}
      {hidden.length > 0 && (
        <span className="tool-more">
          <button
            type="button"
            className={showsMore ? 'tool on' : 'tool'}
            aria-label={t('editor.moreTools')}
            title={t('editor.moreTools')}
            aria-expanded={showsMore}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setShowsMore(!showsMore)}
          >
            <IconChevronDown size={TOOL_ICON_SIZE} />
          </button>
          {showsMore && (
            <span className="tool-more-panel" role="group" aria-label={t('editor.moreTools')}>
              {hidden.map((group) => (
                <span key={group.id} className="tool-fold">
                  {group.tools}
                </span>
              ))}
            </span>
          )}
        </span>
      )}
      {canAttach && (
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
      )}
      <span className="tool-side">
        <SelectionTools editor={editor} actions={actions} />
      </span>
    </div>
  );
}
