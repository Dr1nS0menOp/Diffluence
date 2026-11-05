// SPDX-License-Identifier: Apache-2.0
import { useRef, useState, type DragEvent, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  EditorContent,
  NodeViewWrapper,
  ReactNodeViewRenderer,
  useEditor,
  useEditorState,
  type Editor,
  type ReactNodeViewProps,
} from '@tiptap/react';
import { Node as DocumentNode, type AnyExtension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import { TaskList, TaskItem } from '@tiptap/extension-list';
import { TableKit } from '@tiptap/extension-table';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { common, createLowlight } from 'lowlight';
import { api, type Attachment, type Json } from '../api';
import {
  IconAttach,
  IconBold,
  IconBullet,
  IconCode,
  IconCodeBlock,
  IconColumnAdd,
  IconColumnDelete,
  IconDownload,
  IconItalic,
  IconLink,
  IconNumbered,
  IconPage,
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
import { Dialog, ErrorNote, Field } from './ui';

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

/** A file in the document: a tile to download. While editing it is a block to move or remove. */
function FileTile({ node, editor }: ReactNodeViewProps) {
  const { attachment_id: id, filename } = node.attrs as { attachment_id: string; filename: string };
  const label = (
    <>
      <IconPage />
      <span className="file-name">{filename}</span>
    </>
  );
  return (
    <NodeViewWrapper className="file-block">
      {editor.isEditable ? (
        <span className="file-tile">{label}</span>
      ) : (
        <a className="file-tile" href={`/api/v1/attachments/${id}/content?download=1`} download={filename}>
          {label}
          <IconDownload size={TOOL_ICON_SIZE} />
        </a>
      )}
    </NodeViewWrapper>
  );
}

const AttachmentFile = DocumentNode.create({
  name: 'file',
  group: 'block',
  atom: true,
  draggable: true,
  addAttributes() {
    return {
      attachment_id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-attachment-id'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-attachment-id': attrs.attachment_id }),
      },
      filename: {
        default: '',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-filename'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-filename': attrs.filename }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-attachment-file]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', { ...HTMLAttributes, 'data-attachment-file': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(FileTile);
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
    AttachmentFile,
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

function Toolbar({
  editor,
  canAttach,
  onFiles,
}: {
  editor: Editor;
  canAttach: boolean;
  onFiles: (files: File[]) => void;
}) {
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
        {canAttach && (
          <>
            <Tool editor={editor} label={t('editor.attach')} run={() => file.current?.click()}>
              <IconAttach size={TOOL_ICON_SIZE} />
            </Tool>
            <input
              ref={file}
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

const carriesFiles = (transfer: DataTransfer | null) => transfer?.types.includes('Files') ?? false;

/** What the server made of a file decides how it shows: only safe images show as images. */
function attachmentNode(attachment: Attachment) {
  return attachment.inline
    ? { type: 'image', attrs: { attachment_id: attachment.id, alt: attachment.filename } }
    : { type: 'file', attrs: { attachment_id: attachment.id, filename: attachment.filename } };
}

/** The editing surface. Markdown shortcuts (#, **, -, ```) come from the input rules. */
export function DocumentEditor({
  pageId,
  initial,
  canAttach,
  header,
  onChange,
}: {
  pageId: string;
  initial: Json;
  canAttach: boolean;
  header: ReactNode;
  onChange: (doc: Json) => void;
}) {
  const { t } = useTranslation();
  const [uploading, setUploading] = useState('');
  const [uploadError, setUploadError] = useState<unknown>(null);
  const [dropping, setDropping] = useState(false);
  const change = useRef(onChange);
  change.current = onChange;

  const addFiles = async (files: File[]) => {
    setUploadError(null);
    if (!canAttach) {
      setUploadError(new Error(t('editor.cannotAttach')));
      return;
    }
    // One at a time, so files land in the order they were dropped.
    for (const file of files) {
      setUploading(file.name);
      try {
        const attachment = await api.upload<Attachment>(
          `/pages/${pageId}/attachments?filename=${encodeURIComponent(file.name)}`,
          file,
        );
        // After the selection, never over it: the previous file is still selected.
        editor?.chain().focus().insertContentAt(editor.state.selection.to, attachmentNode(attachment)).run();
      } catch (error) {
        setUploadError(error);
      }
    }
    setUploading('');
  };
  const add = useRef(addFiles);
  add.current = addFiles;

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
      // Files are taken by the area around the editor, so a drop beside the page counts too.
      handleDrop: (_view, event) => carriesFiles(event.dataTransfer),
      handlePaste: (_view, event) => {
        const images = Array.from(event.clipboardData?.files ?? []).filter((f) =>
          f.type.startsWith('image/'),
        );
        if (images.length === 0) return false;
        void add.current(images);
        return true;
      },
    },
    onUpdate: ({ editor: e }) => change.current(e.getJSON() as Json),
  });

  if (!editor) return null;

  const dropFiles = (e: DragEvent<HTMLDivElement>) => {
    if (!carriesFiles(e.dataTransfer)) return;
    e.preventDefault();
    setDropping(false);
    const target = editor.view.posAtCoords({ left: e.clientX, top: e.clientY });
    if (target) editor.commands.setTextSelection(target.pos);
    else if (!editor.isFocused) editor.commands.focus('end');
    void addFiles(Array.from(e.dataTransfer.files));
  };

  return (
    <div
      className={dropping ? 'editor-area dropping' : 'editor-area'}
      onDragOver={(e) => {
        if (!carriesFiles(e.dataTransfer)) return;
        e.preventDefault();
        setDropping(true);
      }}
      onDragLeave={(e) => {
        if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) {
          setDropping(false);
        }
      }}
      onDrop={dropFiles}
    >
      <Toolbar editor={editor} canAttach={canAttach} onFiles={(files) => void addFiles(files)} />
      <div className="editor-sheet">
        {uploading && (
          <p role="status" className="note">
            {t('editor.uploading', { name: uploading })}
          </p>
        )}
        <ErrorNote error={uploadError} />
        {header}
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}
