// SPDX-License-Identifier: Apache-2.0
import { useRef, useState, type DragEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { EditorContent, useEditor } from '@tiptap/react';
import { api, type Attachment, type Json } from '../../api';
import { ErrorNote } from '../ui';
import { attachmentNode } from './attachments';
import { BlockHandle, useBlockTracking } from './block-handle';
import { documentExtensions } from './extensions';
import { FindBar, FindReplace } from './find';
import { EmojiSuggestions, InsertDialog, SlashCommands } from './insert';
import { EditorKeys, noActions, type EditorActions } from './keys';
import { LinkDialog } from './link-dialog';
import { Toolbar } from './toolbar';

/** Shows a stored document. The editor renders it, so no HTML is ever injected. */
export function DocumentView({ document }: { document: Json }) {
  const { t } = useTranslation();
  const editor = useEditor(
    {
      extensions: documentExtensions(),
      content: document,
      editable: false,
      injectCSS: false,
      editorProps: { attributes: { class: 'prose', 'aria-label': t('editor.content') } },
    },
    [document],
  );
  return <EditorContent editor={editor} />;
}

const carriesFiles = (transfer: DataTransfer | null) => transfer?.types.includes('Files') ?? false;

/** The editing surface. Markdown shortcuts (#, **, -, ```) come from the input rules. */
export function DocumentEditor({
  pageId,
  initial,
  canAttach,
  header,
  onChange,
  onPublish,
  onSave,
}: {
  pageId: string;
  initial: Json;
  canAttach: boolean;
  header: ReactNode;
  onChange: (doc: Json) => void;
  onPublish: () => void;
  onSave: () => void;
}) {
  const { t } = useTranslation();
  const [uploading, setUploading] = useState('');
  const [uploadError, setUploadError] = useState<unknown>(null);
  const [dropping, setDropping] = useState(false);
  const [linking, setLinking] = useState(false);
  const [inserting, setInserting] = useState(false);
  const [finding, setFinding] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
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

  // Extensions are made once, so they reach the latest functions through this ref.
  const actions = useRef<EditorActions>(noActions);

  const editor = useEditor({
    extensions: [
      ...documentExtensions(),
      SlashCommands.configure({ actions: () => actions.current }),
      EmojiSuggestions,
      FindReplace.configure({ onOpen: () => actions.current.openFind() }),
      EditorKeys.configure({ actions: () => actions.current }),
    ],
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
  const tracker = useBlockTracking(editor);

  actions.current = {
    pickFiles: () => fileInput.current?.click(),
    openLink: () => setLinking(true),
    openFind: () => setFinding(true),
    openBlockMenu: tracker.openAtCursor,
    publish: onPublish,
    save: onSave,
  };

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
      <Toolbar
        editor={editor}
        canAttach={canAttach}
        actions={actions.current}
        fileInput={fileInput}
        onInsert={() => setInserting(true)}
        onFiles={(files) => void addFiles(files)}
      />
      {finding && <FindBar editor={editor} onClose={() => setFinding(false)} />}
      <div className="editor-sheet" onMouseMove={tracker.track} onMouseLeave={tracker.leave}>
        <BlockHandle editor={editor} tracker={tracker} actions={actions.current} />
        {uploading && (
          <p role="status" className="note">
            {t('editor.uploading', { name: uploading })}
          </p>
        )}
        <ErrorNote error={uploadError} />
        {header}
        <EditorContent editor={editor} />
      </div>
      {linking && <LinkDialog editor={editor} onClose={() => setLinking(false)} />}
      {inserting && (
        <InsertDialog editor={editor} actions={actions.current} onClose={() => setInserting(false)} />
      )}
    </div>
  );
}
