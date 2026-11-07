// SPDX-License-Identifier: Apache-2.0
// The part of the row that follows the selection. Rail keeps tools in one row, so
// these appear there instead of in floating bars (spec 008, D3).
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { useEditorState, type Editor } from '@tiptap/react';
import {
  IconAlignCenter,
  IconAlignEnd,
  IconAlignStart,
  IconChevronDown,
  IconFrame,
  IconLineNumbers,
  IconPicture,
  IconRename,
  IconText,
  IconUnwrap,
  IconWrap,
} from '../icons';
import { Menu, MenuItem, MenuSeparator } from '../menu';
import { Dialog, Field } from '../ui';
import {
  columnCount,
  imageIntoLine,
  MAX_COLUMNS,
  MIN_COLUMNS,
  setColumnCount,
  unwrapBlock,
  updateSelected,
} from './block-commands';
import { useFileFacts } from './attachments';
import { detectLanguage } from './code';
import { today } from './inline';
import { isDrawing } from './drawing';
import type { EditorActions } from './keys';
import { LanguageDialog } from './language-dialog';
import { LinkTools } from './link-tools';
import {
  FIRST_PANEL_EMOJI,
  IMAGE_BORDERS,
  IMAGE_SIZES,
  MARK_COLORS,
  PANEL_EMOJI,
  PANEL_KINDS,
  PANEL_TINT,
} from './palette';
import { Tool, ToolSeparator, TOOL_ICON_SIZE } from './tool';

const MIN_IMAGE_WIDTH = 5;
const MAX_IMAGE_WIDTH = 100;

function Swatch({ color }: { color: string }) {
  return <span className={`swatch mark-${color}`} />;
}

/** An image still called after its file has no words of its own yet. */
const FILE_NAME = /\.[a-z0-9]{2,5}$/i;

export function imagesWithoutWords(doc: ProseNode): number {
  let count = 0;
  doc.descendants((node) => {
    const alt = node.attrs.alt as string | null | undefined;
    if (node.type.name === 'image' && (!alt || FILE_NAME.test(alt))) count += 1;
    return true;
  });
  return count;
}

export function DescribeDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const attrs = editor.getAttributes('image') as {
    alt: string | null;
    caption: string | null;
    width: number | null;
    href: string | null;
  };
  const [alt, setAlt] = useState(attrs.alt ?? '');
  const [caption, setCaption] = useState(attrs.caption ?? '');
  const [width, setWidth] = useState(attrs.width?.toString() ?? '');
  const [href, setHref] = useState(attrs.href ?? '');
  return (
    <Dialog title={t('editor.describe')} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          onClose();
          const share = Number(width);
          updateSelected(editor, {
            alt,
            caption: caption || null,
            width: share >= MIN_IMAGE_WIDTH && share <= MAX_IMAGE_WIDTH ? share : null,
            href: href.trim() || null,
          });
        }}
      >
        <Field label={t('editor.imageAlt')} help={t('editor.imageAltHelp')}>
          <input value={alt} autoFocus onChange={(e) => setAlt(e.target.value)} />
        </Field>
        <Field label={t('editor.imageCaption')}>
          <input value={caption} onChange={(e) => setCaption(e.target.value)} />
        </Field>
        <Field label={t('editor.imageWidth')} help={t('editor.imageWidthHelp')}>
          <input
            type="number"
            min={MIN_IMAGE_WIDTH}
            max={MAX_IMAGE_WIDTH}
            value={width}
            onChange={(e) => setWidth(e.target.value)}
          />
        </Field>
        <Field label={t('editor.imageLink')} help={t('editor.imageLinkHelp')}>
          <input value={href} onChange={(e) => setHref(e.target.value)} />
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
  const attrs = useEditorState({
    editor,
    selector: (s) =>
      s.editor.getAttributes('image') as {
        size: string | null;
        align: string | null;
        wrap: boolean;
        border: string | null;
        border_color: string | null;
      },
  });
  const set = (values: Record<string, string | boolean | null>) => updateSelected(editor, values);
  const canWrap = attrs.align === 'start' || attrs.align === 'end';
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
      <Tool
        editor={editor}
        label={t('editor.imageWrap')}
        pressed={attrs.wrap}
        run={() => set({ wrap: !attrs.wrap, align: canWrap ? attrs.align : 'start' })}
      >
        <IconWrap size={TOOL_ICON_SIZE} />
      </Tool>
      <Menu
        label={t('editor.imageBorder')}
        trigger={<IconFrame size={TOOL_ICON_SIZE} />}
        className="tool"
        align="start"
      >
        <MenuItem
          role="menuitemradio"
          aria-checked={!attrs.border}
          onClick={() => set({ border: null, border_color: null })}
        >
          {t('editor.imageBorderNone')}
        </MenuItem>
        {IMAGE_BORDERS.map((border) => (
          <MenuItem
            key={border}
            role="menuitemradio"
            aria-checked={attrs.border === border}
            onClick={() => set({ border })}
          >
            {t(`editor.imageBorder_${border}`)}
          </MenuItem>
        ))}
        <MenuSeparator />
        {MARK_COLORS.map((color) => (
          <MenuItem
            key={color}
            role="menuitemradio"
            aria-checked={attrs.border_color === color}
            onClick={() => set({ border: attrs.border ?? 'thin', border_color: color })}
          >
            <Swatch color={color} /> {t(`editor.colors.${color}`)}
          </MenuItem>
        ))}
      </Menu>
      <Tool editor={editor} label={t('editor.imageInline')} run={() => imageIntoLine(editor)}>
        <IconText size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.describe')} run={actions.openDescribe}>
        <IconRename size={TOOL_ICON_SIZE} />
      </Tool>
      <EditDrawingTool editor={editor} actions={actions} />
    </>
  );
}

function FileTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const display = useEditorState({
    editor,
    selector: (s) => (s.editor.getAttributes('file').display as string | null) ?? 'small',
  });
  return (
    <select
      className="tool-style"
      aria-label={t('editor.fileDisplay')}
      value={display}
      onChange={(e) => updateSelected(editor, { display: e.target.value === 'large' ? 'large' : null })}
    >
      <option value="small">{t('editor.fileSmall')}</option>
      <option value="large">{t('editor.fileLarge')}</option>
    </select>
  );
}

function PanelTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const attrs = useEditorState({
    editor,
    selector: (s) =>
      s.editor.getAttributes('panel') as { kind?: string; emoji?: string | null; color?: string | null },
  });
  const { kind } = attrs;
  const change = (values: Record<string, string | null>) =>
    editor.chain().focus().updateAttributes('panel', values).run();
  return (
    <>
      <Menu
        label={t('editor.panels.neutral')}
        trigger={
          <span className={kind === 'neutral' ? 'tool-emoji on' : 'tool-emoji'}>
            {attrs.emoji ?? FIRST_PANEL_EMOJI}
          </span>
        }
        className="tool"
        align="start"
      >
        {PANEL_EMOJI.map((emoji) => (
          <MenuItem
            key={emoji}
            role="menuitemradio"
            aria-checked={attrs.emoji === emoji}
            onClick={() => change({ kind: 'neutral', emoji })}
          >
            {emoji}
          </MenuItem>
        ))}
        <MenuSeparator />
        {MARK_COLORS.map((color) => (
          <MenuItem
            key={color}
            role="menuitemradio"
            aria-checked={attrs.color === color}
            onClick={() => change({ kind: 'neutral', color, emoji: attrs.emoji ?? FIRST_PANEL_EMOJI })}
          >
            <Swatch color={color} /> {t(`editor.colors.${color}`)}
          </MenuItem>
        ))}
      </Menu>
      {PANEL_KINDS.map((option) => (
        <button
          key={option}
          type="button"
          className={kind === option ? 'tool on' : 'tool'}
          aria-label={t(`editor.panels.${option}`)}
          aria-pressed={kind === option}
          title={t(`editor.panels.${option}`)}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => change({ kind: option, emoji: null, color: null })}
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
      width: s.editor.getAttributes('columns').width as string | null | undefined,
    }),
  });
  const counts = Array.from({ length: MAX_COLUMNS - MIN_COLUMNS + 1 }, (_, i) => i + MIN_COLUMNS);
  const layouts = [
    { value: null, label: t('editor.columnsEqual') },
    { value: 'wide-start', label: t('editor.columnsWideFirst') },
    { value: 'wide-end', label: t('editor.columnsWideLast') },
    { value: 'sidebar-start', label: t('editor.columnsSidebarFirst') },
    { value: 'sidebar-end', label: t('editor.columnsSidebarLast') },
  ];
  const widths = [
    { value: '', label: t('editor.columnsNormal') },
    { value: 'wide', label: t('editor.columnsWide') },
    { value: 'full', label: t('editor.columnsFull') },
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
      <select
        className="tool-style"
        aria-label={t('editor.columnsWidth')}
        value={state.width ?? ''}
        onChange={(e) => editor.commands.updateAttributes('columns', { width: e.target.value || null })}
      >
        {widths.map((width) => (
          <option key={width.value} value={width.value}>
            {width.label}
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
        line_start: number | null;
        folded: boolean;
      },
  });
  const [choosing, setChoosing] = useState(false);
  const set = (values: Record<string, string | boolean | number | null>) =>
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
      <Tool
        editor={editor}
        label={t('editor.codeFold')}
        pressed={attrs.folded}
        run={() => set({ folded: !attrs.folded })}
      >
        <IconChevronDown size={TOOL_ICON_SIZE} />
      </Tool>
      <button
        type="button"
        className="ghost"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          const found = detectLanguage(editor.state.selection.$from.parent.textContent);
          if (found) set({ language: found });
        }}
      >
        {t('editor.codeDetect')}
      </button>
      {attrs.line_numbers && (
        <input
          className="tool-input tool-number"
          type="number"
          min={1}
          aria-label={t('editor.codeFirstLine')}
          title={t('editor.codeFirstLine')}
          value={attrs.line_start ?? 1}
          onChange={(e) => {
            const first = Number(e.target.value);
            set({ line_start: first > 1 ? first : null });
          }}
        />
      )}
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

function SpotlightTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const attrs = useEditorState({
    editor,
    selector: (s) =>
      s.editor.getAttributes('spotlight') as {
        color: string | null;
        button_text: string | null;
        button_href: string | null;
      },
  });
  const set = (values: Record<string, string | null>) =>
    editor.commands.updateAttributes('spotlight', values);
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
          onClick={() => set({ color })}
        >
          <Swatch color={color} />
        </button>
      ))}
      <input
        className="tool-input"
        aria-label={t('editor.spotlightButton')}
        placeholder={t('editor.spotlightButton')}
        value={attrs.button_text ?? ''}
        onChange={(e) => set({ button_text: e.target.value || null })}
      />
      <input
        className="tool-input"
        aria-label={t('editor.spotlightAddress')}
        placeholder={t('editor.spotlightAddress')}
        value={attrs.button_href ?? ''}
        onChange={(e) => set({ button_href: e.target.value || null })}
      />
      <Tool editor={editor} label={t('editor.spotlightRemove')} run={() => unwrapBlock(editor, 'spotlight')}>
        <IconUnwrap size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

/** A decision holds inline items, so a person and a date need no fields of their own (spec 027, D11). */
function DecisionTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  return (
    <>
      <button
        type="button"
        className="ghost"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => editor.chain().focus().insertContent({ type: 'text', text: ' @' }).run()}
      >
        {t('editor.decisionPerson')}
      </button>
      <button
        type="button"
        className="ghost"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() =>
          editor
            .chain()
            .focus()
            .insertContent({ type: 'date', attrs: { value: today() } })
            .run()
        }
      >
        {t('editor.decisionDate')}
      </button>
    </>
  );
}

const LINK_KINDS = ['link', 'pageLink', 'pageCard', 'placeholderLink'];

/** The tools for what is selected. Only the innermost thing gets them, so the row stays short. */
export function SelectionTools({ editor, actions }: { editor: Editor; actions: EditorActions }) {
  const active = useEditorState({
    editor,
    selector: (s) =>
      [
        'status',
        'date',
        'image',
        'file',
        ...LINK_KINDS,
        'codeBlock',
        'decision',
        'spotlight',
        'panel',
        'expand',
        'columns',
      ].find((name) => s.editor.isActive(name)),
  });
  const tools: Record<string, ReactNode> = {
    // Keyed on the place, so a second mark does not show the words of the first.
    status: <StatusTools key={editor.state.selection.from} editor={editor} />,
    date: <DateTools key="date" editor={editor} />,
    image: <ImageTools key="image" editor={editor} actions={actions} />,
    file: <FileTools key="file" editor={editor} />,
    codeBlock: <CodeTools key="code" editor={editor} />,

    decision: <DecisionTools key="decision" editor={editor} />,
    spotlight: <SpotlightTools key="spotlight" editor={editor} />,
    panel: <PanelTools key="panel" editor={editor} />,
    expand: <ExpandTools key="expand" editor={editor} />,
    columns: <ColumnTools key="columns" editor={editor} />,
    ...Object.fromEntries(
      LINK_KINDS.map((kind) => [kind, <LinkTools key={kind} editor={editor} actions={actions} />]),
    ),
  };
  if (!active) return null;
  return (
    <>
      <ToolSeparator />
      {tools[active]}
    </>
  );
}
