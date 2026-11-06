// SPDX-License-Identifier: Apache-2.0
// The part of the row that follows the selection. Rail keeps tools in one row, so
// these appear there instead of in floating bars (spec 008, D3).
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useEditorState, type Editor } from '@tiptap/react';
import {
  IconAlignCenter,
  IconAlignEnd,
  IconAlignStart,
  IconLineNumbers,
  IconPicture,
  IconRename,
  IconUnwrap,
  IconWrap,
} from '../icons';
import { Menu, MenuItem } from '../menu';
import { Dialog, Field } from '../ui';
import {
  columnCount,
  MAX_COLUMNS,
  MIN_COLUMNS,
  setColumnCount,
  unwrapBlock,
  updateSelected,
} from './block-commands';
import { useFileFacts } from './attachments';
import { isDrawing } from './drawing';
import type { EditorActions } from './keys';
import { LanguageDialog } from './language-dialog';
import { IMAGE_SIZES, MARK_COLORS, PANEL_KINDS, PANEL_TINT } from './palette';
import { Tool, ToolSeparator, TOOL_ICON_SIZE } from './tool';

function Swatch({ color }: { color: string }) {
  return <span className={`swatch mark-${color}`} />;
}

function DescribeDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const attrs = editor.getAttributes('image') as { alt: string | null; caption: string | null };
  const [alt, setAlt] = useState(attrs.alt ?? '');
  const [caption, setCaption] = useState(attrs.caption ?? '');
  return (
    <Dialog title={t('editor.describe')} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onClose();
          updateSelected(editor, { alt, caption: caption || null });
        }}
      >
        <Field label={t('editor.imageAlt')} help={t('editor.imageAltHelp')}>
          <input value={alt} autoFocus onChange={(e) => setAlt(e.target.value)} />
        </Field>
        <Field label={t('editor.imageCaption')}>
          <input value={caption} onChange={(e) => setCaption(e.target.value)} />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function EditDrawingTool({ editor, actions }: { editor: Editor; actions: EditorActions }) {
  const { t } = useTranslation();
  const id = (editor.getAttributes('image') as { attachment_id: string | null }).attachment_id;
  const file = useFileFacts(id, true).data;
  if (!file || !isDrawing(file.filename)) return null;
  return (
    <button type="button" className="ghost" onClick={() => actions.openDrawing(file)}>
      {t('drawing.edit')}
    </button>
  );
}

function ImageTools({ editor, actions }: { editor: Editor; actions: EditorActions }) {
  const { t } = useTranslation();
  const [describing, setDescribing] = useState(false);
  const attrs = useEditorState({
    editor,
    selector: (s) => s.editor.getAttributes('image') as { size: string | null; align: string | null },
  });
  const set = (values: Record<string, string | null>) => updateSelected(editor, values);
  const alignments = [
    { value: 'start', label: t('editor.alignStart'), icon: <IconAlignStart size={TOOL_ICON_SIZE} /> },
    { value: 'center', label: t('editor.alignCenter'), icon: <IconAlignCenter size={TOOL_ICON_SIZE} /> },
    { value: 'end', label: t('editor.alignEnd'), icon: <IconAlignEnd size={TOOL_ICON_SIZE} /> },
  ];
  return (
    <>
      <Menu
        label={t('editor.imageSize')}
        trigger={<IconPicture size={TOOL_ICON_SIZE} />}
        className="tool"
        align="start"
      >
        <MenuItem role="menuitemradio" aria-checked={!attrs.size} onClick={() => set({ size: null })}>
          {t('editor.imageOriginal')}
        </MenuItem>
        {IMAGE_SIZES.map((size) => (
          <MenuItem
            key={size}
            role="menuitemradio"
            aria-checked={attrs.size === size}
            onClick={() => set({ size })}
          >
            {t(`editor.image_${size}`)}
          </MenuItem>
        ))}
      </Menu>
      {alignments.map((option) => (
        <Tool
          key={option.value}
          editor={editor}
          label={option.label}
          pressed={attrs.align === option.value}
          run={() => set({ align: option.value })}
        >
          {option.icon}
        </Tool>
      ))}
      <Tool editor={editor} label={t('editor.describe')} run={() => setDescribing(true)}>
        <IconRename size={TOOL_ICON_SIZE} />
      </Tool>
      {describing && <DescribeDialog editor={editor} onClose={() => setDescribing(false)} />}
      <EditDrawingTool editor={editor} actions={actions} />
    </>
  );
}

function PanelTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const kind = useEditorState({
    editor,
    selector: (s) => s.editor.getAttributes('panel').kind as string | undefined,
  });
  return (
    <>
      {PANEL_KINDS.map((option) => (
        <button
          key={option}
          type="button"
          className={kind === option ? 'tool on' : 'tool'}
          aria-label={t(`editor.panels.${option}`)}
          aria-pressed={kind === option}
          title={t(`editor.panels.${option}`)}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => editor.chain().focus().updateAttributes('panel', { kind: option }).run()}
        >
          <Swatch color={PANEL_TINT[option]} />
        </button>
      ))}
      <Tool editor={editor} label={t('editor.panelRemove')} run={() => unwrapBlock(editor, 'panel')}>
        <IconUnwrap size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function ExpandTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const open = useEditorState({
    editor,
    selector: (s) => (s.editor.getAttributes('expand').open as boolean | undefined) ?? false,
  });
  return (
    <>
      <label className="row tool-check">
        <input
          type="checkbox"
          checked={open}
          onChange={(e) => editor.commands.updateAttributes('expand', { open: e.target.checked })}
        />
        {t('editor.expandOpen')}
      </label>
      <Tool editor={editor} label={t('editor.expandRemove')} run={() => unwrapBlock(editor, 'expand')}>
        <IconUnwrap size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function ColumnTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const state = useEditorState({
    editor,
    selector: (s) => ({
      count: columnCount(s.editor),
      layout: s.editor.getAttributes('columns').layout as string | null | undefined,
    }),
  });
  const counts = Array.from({ length: MAX_COLUMNS - MIN_COLUMNS + 1 }, (_, i) => i + MIN_COLUMNS);
  const layouts = [
    { value: null, label: t('editor.columnsEqual') },
    { value: 'wide-start', label: t('editor.columnsWideFirst') },
    { value: 'wide-end', label: t('editor.columnsWideLast') },
  ];
  return (
    <>
      <select
        className="tool-style"
        aria-label={t('editor.columnCount')}
        value={state.count}
        onChange={(e) => setColumnCount(editor, Number(e.target.value))}
      >
        {counts.map((count) => (
          <option key={count} value={count}>
            {t('editor.columnCountOption', { count })}
          </option>
        ))}
      </select>
      <select
        className="tool-style"
        aria-label={t('editor.columnLayout')}
        value={state.layout ?? ''}
        onChange={(e) => editor.commands.updateAttributes('columns', { layout: e.target.value || null })}
      >
        {layouts.map((layout) => (
          <option key={layout.label} value={layout.value ?? ''}>
            {layout.label}
          </option>
        ))}
      </select>
      <Tool editor={editor} label={t('editor.columnsRemove')} run={() => unwrapBlock(editor, 'columns')}>
        <IconUnwrap size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function StatusTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const attrs = useEditorState({
    editor,
    selector: (s) => s.editor.getAttributes('status') as { text: string; color: string },
  });
  const [draft, setDraft] = useState(attrs.text);
  return (
    <>
      {MARK_COLORS.map((color) => (
        <button
          key={color}
          type="button"
          className={attrs.color === color ? 'tool on' : 'tool'}
          aria-label={t(`editor.colors.${color}`)}
          aria-pressed={attrs.color === color}
          title={t(`editor.colors.${color}`)}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => updateSelected(editor, { color })}
        >
          <Swatch color={color} />
        </button>
      ))}
      <input
        className="tool-input"
        aria-label={t('editor.statusText')}
        value={draft}
        maxLength={40}
        onChange={(e) => {
          setDraft(e.target.value);
          // A mark without words is refused, so the last valid words stay until there are new ones.
          if (e.target.value.trim()) updateSelected(editor, { text: e.target.value });
        }}
      />
    </>
  );
}

function DateTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const value = useEditorState({
    editor,
    selector: (s) => s.editor.getAttributes('date').value as string,
  });
  return (
    <input
      type="date"
      className="tool-input"
      aria-label={t('editor.dateValue')}
      value={value}
      onChange={(e) => {
        if (e.target.value) updateSelected(editor, { value: e.target.value });
      }}
    />
  );
}

function CodeTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const attrs = useEditorState({
    editor,
    selector: (s) =>
      s.editor.getAttributes('codeBlock') as {
        language: string | null;
        title: string | null;
        line_numbers: boolean;
        wrap: boolean;
      },
  });
  const [choosing, setChoosing] = useState(false);
  const set = (values: Record<string, string | boolean | null>) =>
    editor.commands.updateAttributes('codeBlock', values);
  const language = attrs.language ?? t('editor.plainText');
  return (
    <>
      <button
        type="button"
        className="tool-language"
        aria-label={`${t('editor.codeLanguage')}: ${language}`}
        title={t('editor.codeLanguage')}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setChoosing(true)}
      >
        {language}
      </button>
      {choosing && (
        <LanguageDialog
          editor={editor}
          onPick={(picked) => {
            setChoosing(false);
            set({ language: picked });
            editor.commands.focus();
          }}
          onClose={() => setChoosing(false)}
        />
      )}
      <Tool
        editor={editor}
        label={t('editor.lineNumbers')}
        pressed={attrs.line_numbers}
        run={() => set({ line_numbers: !attrs.line_numbers })}
      >
        <IconLineNumbers size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.wrapLines')}
        pressed={attrs.wrap}
        run={() => set({ wrap: !attrs.wrap })}
      >
        <IconWrap size={TOOL_ICON_SIZE} />
      </Tool>
      <input
        className="tool-input"
        aria-label={t('editor.codeTitle')}
        placeholder={t('editor.codeTitle')}
        value={attrs.title ?? ''}
        onChange={(e) => set({ title: e.target.value || null })}
      />
    </>
  );
}

/** The tools for what is selected. Only the innermost thing gets them, so the row stays short. */
export function SelectionTools({ editor, actions }: { editor: Editor; actions: EditorActions }) {
  const active = useEditorState({
    editor,
    selector: (s) =>
      ['status', 'date', 'image', 'codeBlock', 'panel', 'expand', 'columns'].find((name) =>
        s.editor.isActive(name),
      ),
  });
  const tools: Record<string, ReactNode> = {
    // Keyed on the place, so a second mark does not show the words of the first.
    status: <StatusTools key={editor.state.selection.from} editor={editor} />,
    date: <DateTools key="date" editor={editor} />,
    image: <ImageTools key="image" editor={editor} actions={actions} />,
    codeBlock: <CodeTools key="code" editor={editor} />,

    panel: <PanelTools key="panel" editor={editor} />,
    expand: <ExpandTools key="expand" editor={editor} />,
    columns: <ColumnTools key="columns" editor={editor} />,
  };
  if (!active) return null;
  return (
    <>
      <ToolSeparator />
      {tools[active]}
    </>
  );
}
