// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef } from 'react';
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

const lowlight = createLowlight(common);

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
      editorProps: { attributes: { class: 'prose', 'aria-label': t('page.title') } },
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
  text,
}: {
  editor: Editor;
  label: string;
  active?: string | [string, Record<string, unknown>];
  run: () => void;
  text: string;
}) {
  const on = useEditorState({
    editor,
    selector: (s) => {
      if (!active) return false;
      return Array.isArray(active) ? s.editor.isActive(active[0], active[1]) : s.editor.isActive(active);
    },
  });
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
      {text}
    </button>
  );
}

function Toolbar({ editor, onImage }: { editor: Editor; onImage: (file: File) => void }) {
  const { t } = useTranslation();
  const file = useRef<HTMLInputElement>(null);
  const chain = () => editor.chain().focus();
  const inTable = useEditorState({ editor, selector: (s) => s.editor.isActive('table') });
  const link = () => {
    const previous = (editor.getAttributes('link').href as string | undefined) ?? '';
    const href = window.prompt(t('editor.linkPrompt'), previous);
    if (href === null) return;
    if (href === '') chain().unsetLink().run();
    else chain().setLink({ href }).run();
  };
  return (
    <div role="toolbar" aria-label={t('editor.toolbar')} className="toolbar">
      <Tool
        editor={editor}
        label={t('editor.bold')}
        active="bold"
        text="B"
        run={() => chain().toggleBold().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.italic')}
        active="italic"
        text="I"
        run={() => chain().toggleItalic().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.underline')}
        active="underline"
        text="U"
        run={() => chain().toggleUnderline().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.strike')}
        active="strike"
        text="S"
        run={() => chain().toggleStrike().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.code')}
        active="code"
        text="`"
        run={() => chain().toggleCode().run()}
      />
      <span className="sep" />
      {([1, 2, 3] as const).map((level) => (
        <Tool
          key={level}
          editor={editor}
          label={t(`editor.h${level}`)}
          active={['heading', { level }]}
          text={`H${level}`}
          run={() => chain().toggleHeading({ level }).run()}
        />
      ))}
      <span className="sep" />
      <Tool
        editor={editor}
        label={t('editor.bullet')}
        active="bulletList"
        text="•"
        run={() => chain().toggleBulletList().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.ordered')}
        active="orderedList"
        text="1."
        run={() => chain().toggleOrderedList().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.task')}
        active="taskList"
        text="☑"
        run={() => chain().toggleTaskList().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.quote')}
        active="blockquote"
        text="❝"
        run={() => chain().toggleBlockquote().run()}
      />
      <Tool
        editor={editor}
        label={t('editor.codeBlock')}
        active="codeBlock"
        text="{ }"
        run={() => chain().toggleCodeBlock().run()}
      />
      <Tool editor={editor} label={t('editor.rule')} text="―" run={() => chain().setHorizontalRule().run()} />
      <span className="sep" />
      <Tool editor={editor} label={t('editor.link')} active="link" text="🔗" run={link} />
      <Tool editor={editor} label={t('editor.image')} text="🖼" run={() => file.current?.click()} />
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
        text="▦"
        run={() => chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()}
      />
      {inTable && (
        <>
          <Tool
            editor={editor}
            label={t('editor.tableAddRow')}
            text="+R"
            run={() => chain().addRowAfter().run()}
          />
          <Tool
            editor={editor}
            label={t('editor.tableAddCol')}
            text="+C"
            run={() => chain().addColumnAfter().run()}
          />
          <Tool
            editor={editor}
            label={t('editor.tableDeleteRow')}
            text="−R"
            run={() => chain().deleteRow().run()}
          />
          <Tool
            editor={editor}
            label={t('editor.tableDeleteCol')}
            text="−C"
            run={() => chain().deleteColumn().run()}
          />
          <Tool
            editor={editor}
            label={t('editor.tableDelete')}
            text="✕"
            run={() => chain().deleteTable().run()}
          />
        </>
      )}
      <span className="sep" />
      <Tool editor={editor} label={t('editor.undo')} text="↶" run={() => chain().undo().run()} />
      <Tool editor={editor} label={t('editor.redo')} text="↷" run={() => chain().redo().run()} />
    </div>
  );
}

/** The editing surface. Markdown shortcuts (#, **, -, ```) come from the input rules. */
export function DocumentEditor({
  pageId,
  initial,
  onChange,
  onUploadError,
}: {
  pageId: string;
  initial: Json;
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
        'aria-label': t('page.title'),
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
    <div className="editor">
      <Toolbar editor={editor} onImage={(f) => void insertImage(f)} />
      <EditorContent editor={editor} />
    </div>
  );
}
