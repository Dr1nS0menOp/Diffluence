// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { EditorContent, useEditor, useEditorState, type Editor } from '@tiptap/react';
import type { AnyExtension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import { TaskList, TaskItem } from '@tiptap/extension-list';
import { TableKit } from '@tiptap/extension-table';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { common, createLowlight } from 'lowlight';
import { api, type Attachment, type Json } from '../api';
import {
  IconBold,
  IconBullet,
  IconCode,
  IconCodeBlock,
  IconColumnAdd,
  IconColumnDelete,
  IconImage,
  IconItalic,
  IconLink,
  IconNumbered,
  IconQuote,
  IconRedo,
  IconRowAdd,
  IconRowDelete,
  IconRule,
  IconStrike,
  IconTable,
  IconTask,
  IconTrash,
  IconUnderline,
  IconUndo,
} from './icons';
import { Dialog, Field } from './ui';

const lowlight = createLowlight(common);
const HEADING_LEVELS = [1, 2, 3] as const;
const TOOL_ICON_SIZE = 18;

/**
 * Images are attachments, never external addresses (spec 001). Pasted images
 * from elsewhere are dropped here, and the server refuses them as well.
 */
const AttachmentImage = Image.extend({
  addAttributes() {
    return {
      attachment_id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-attachment-id'),
        renderHTML: (attrs: Record<string, string>) => ({
          'data-attachment-id': attrs.attachment_id,
          src: `/api/v1/attachments/${attrs.attachment_id}/content`,
        }),
      },
      alt: { default: null },
      title: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'img[data-attachment-id]' }];
  },
});

export function extensions(): AnyExtension[] {
  return [
    StarterKit.configure({
      codeBlock: false,
      link: {
        openOnClick: false,
        autolink: true,
        protocols: ['http', 'https', 'mailto'],
        HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
      },
    }),
    TaskList,
    TaskItem.configure({ nested: true }),
    TableKit.configure({ table: { resizable: false } }),
    CodeBlockLowlight.configure({ lowlight }),
    AttachmentImage,
  ];
}

/** Shows a stored document. The editor renders it, so no HTML is ever injected. */
export function DocumentView({ document }: { document: Json }) {
  const { t } = useTranslation();
  const editor = useEditor(
    {
      extensions: extensions(),
      content: document,
      editable: false,
      injectCSS: false,
      editorProps: { attributes: { class: 'prose', 'aria-label': t('editor.content') } },
    },
    [document],
  );
  return <EditorContent editor={editor} />;
}

function Tool({
  editor,
  label,
  active,
  run,
  children,
}: {
  editor: Editor;
  label: string;
  active?: string;
  run: () => void;
  children: ReactNode;
}) {
  const on = useEditorState({ editor, selector: (s) => (active ? s.editor.isActive(active) : false) });
  return (
    <button
      type="button"
      className={on ? 'tool on' : 'tool'}
      aria-label={label}
      aria-pressed={active ? on : undefined}
      title={label}
      onMouseDown={(e) => e.preventDefault()}
      onClick={run}
    >
      {children}
    </button>
  );
}

function ToolSeparator() {
  return <span className="tool-separator" />;
}

function TextStyle({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const current = useEditorState({
    editor,
    selector: (s) => HEADING_LEVELS.find((level) => s.editor.isActive('heading', { level })) ?? 0,
  });
  const apply = (value: number) => {
    const level = HEADING_LEVELS.find((l) => l === value);
    const chain = editor.chain().focus();
    if (level) chain.setHeading({ level }).run();
    else chain.setParagraph().run();
  };
  return (
    <select
      className="tool-style"
      aria-label={t('editor.textStyle')}
      value={current}
      onChange={(e) => apply(Number(e.target.value))}
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

function BlockTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  return (
    <>
      <Tool
        editor={editor}
        label={t('editor.bullet')}
        active="bulletList"
        run={() => chain().toggleBulletList().run()}
      >
        <IconBullet size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.ordered')}
        active="orderedList"
        run={() => chain().toggleOrderedList().run()}
      >
        <IconNumbered size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.task')}
        active="taskList"
        run={() => chain().toggleTaskList().run()}
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
      <Tool editor={editor} label={t('editor.rule')} run={() => chain().setHorizontalRule().run()}>
        <IconRule size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function TableTools({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  return (
    <>
      <ToolSeparator />
      <Tool editor={editor} label={t('editor.tableAddRow')} run={() => chain().addRowAfter().run()}>
        <IconRowAdd size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.tableAddCol')} run={() => chain().addColumnAfter().run()}>
        <IconColumnAdd size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.tableDeleteRow')} run={() => chain().deleteRow().run()}>
        <IconRowDelete size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.tableDeleteCol')} run={() => chain().deleteColumn().run()}>
        <IconColumnDelete size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.tableDelete')} run={() => chain().deleteTable().run()}>
        <IconTrash size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function LinkDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const isLink = editor.isActive('link');
  const [href, setHref] = useState((editor.getAttributes('link').href as string | undefined) ?? '');
  const apply = (e: FormEvent) => {
    e.preventDefault();
    onClose();
    if (editor.state.selection.empty && !isLink) {
      const link = { type: 'link', attrs: { href } };
      editor
        .chain()
        .focus()
        .insertContent({ type: 'text', text: href, marks: [link] })
        .run();
      return;
    }
    editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
  };
  const remove = () => {
    onClose();
    editor.chain().focus().extendMarkRange('link').unsetLink().run();
  };
  return (
    <Dialog title={t('editor.link')} onClose={onClose}>
      <form onSubmit={apply}>
        <Field label={t('editor.linkPrompt')}>
          <input type="url" value={href} onChange={(e) => setHref(e.target.value)} required autoFocus />
        </Field>
        <div className="dialog-actions">
          {isLink && (
            <button type="button" className="quiet" onClick={remove}>
              {t('editor.linkRemove')}
            </button>
          )}
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function Toolbar({ editor, onImage }: { editor: Editor; onImage: (file: File) => void }) {
  const { t } = useTranslation();
  const file = useRef<HTMLInputElement>(null);
  const [linking, setLinking] = useState(false);
  const chain = () => editor.chain().focus();
  const inTable = useEditorState({ editor, selector: (s) => s.editor.isActive('table') });
  return (
    <>
      <div role="toolbar" aria-label={t('editor.toolbar')} className="toolbar">
        <TextStyle editor={editor} />
        <InlineTools editor={editor} />
        <ToolSeparator />
        <BlockTools editor={editor} />
        <ToolSeparator />
        <Tool editor={editor} label={t('editor.link')} active="link" run={() => setLinking(true)}>
          <IconLink size={TOOL_ICON_SIZE} />
        </Tool>
        <Tool editor={editor} label={t('editor.image')} run={() => file.current?.click()}>
          <IconImage size={TOOL_ICON_SIZE} />
        </Tool>
        <input
          ref={file}
          type="file"
          accept="image/png,image/jpeg,image/gif,image/webp"
          hidden
          aria-label={t('editor.image')}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) onImage(f);
            e.target.value = '';
          }}
        />
        <Tool
          editor={editor}
          label={t('editor.table')}
          run={() => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()}
        >
          <IconTable size={TOOL_ICON_SIZE} />
        </Tool>
        {inTable && <TableTools editor={editor} />}
        <ToolSeparator />
        <Tool editor={editor} label={t('editor.undo')} run={() => chain().undo().run()}>
          <IconUndo size={TOOL_ICON_SIZE} />
        </Tool>
        <Tool editor={editor} label={t('editor.redo')} run={() => chain().redo().run()}>
          <IconRedo size={TOOL_ICON_SIZE} />
        </Tool>
      </div>
      {linking && <LinkDialog editor={editor} onClose={() => setLinking(false)} />}
    </>
  );
}

/** The editing surface. Markdown shortcuts (#, **, -, ```) come from the input rules. */
export function DocumentEditor({
  pageId,
  initial,
  header,
  onChange,
  onUploadError,
}: {
  pageId: string;
  initial: Json;
  header: ReactNode;
  onChange: (doc: Json) => void;
  onUploadError: () => void;
}) {
  const { t } = useTranslation();
  const change = useRef(onChange);
  change.current = onChange;

  const editor = useEditor({
    extensions: extensions(),
    content: initial,
    injectCSS: false,
    editorProps: {
      attributes: {
        class: 'prose editing',
        'aria-label': t('editor.content'),
        role: 'textbox',
        'aria-multiline': 'true',
      },
    },
    onUpdate: ({ editor: e }) => change.current(e.getJSON() as Json),
  });

  const insertImage = async (file: File) => {
    if (!editor) return;
    try {
      const a = await api.upload<Attachment>(
        `/pages/${pageId}/attachments?filename=${encodeURIComponent(file.name)}`,
        file,
      );
      editor
        .chain()
        .focus()
        .insertContent({ type: 'image', attrs: { attachment_id: a.id, alt: file.name } })
        .run();
    } catch {
      onUploadError();
    }
  };
  const insert = useRef(insertImage);
  insert.current = insertImage;

  useEffect(() => {
    if (!editor) return;
    const dom = editor.view.dom;
    const take = (files: FileList | undefined | null, e: Event) => {
      const images = Array.from(files ?? []).filter((f) => f.type.startsWith('image/'));
      if (images.length === 0) return;
      e.preventDefault();
      images.forEach((f) => void insert.current(f));
    };
    const paste = (e: ClipboardEvent) => take(e.clipboardData?.files, e);
    const drop = (e: DragEvent) => take(e.dataTransfer?.files, e);
    dom.addEventListener('paste', paste);
    dom.addEventListener('drop', drop);
    return () => {
      dom.removeEventListener('paste', paste);
      dom.removeEventListener('drop', drop);
    };
  }, [editor]);

  if (!editor) return null;
  return (
    <div>
      <Toolbar editor={editor} onImage={(f) => void insertImage(f)} />
      <div className="editor-sheet">
        {header}
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}
