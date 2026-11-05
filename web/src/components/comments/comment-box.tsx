// SPDX-License-Identifier: Apache-2.0
// The small editor of a comment. It writes the same document format as a page, with a
// shorter list, and the server checks it through the same gate (spec 010, D4).
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Extension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { EditorContent, useEditor, type Editor } from '@tiptap/react';
import { api, type Attachment, type Json } from '../../api';
import { dropDraft, keepDraft, readDraft, usePerson } from '../../personal';
import { match, navigate } from '../../router';
import { AttachmentImage, attachmentNode } from '../editor/attachments';
import { MentionSuggestions } from '../editor/insert';
import { Mention } from '../editor/people';
import { IconBold, IconBullet, IconCode, IconItalic, IconPicture } from '../icons';
import { useToast } from '../toast';
import { ErrorNote } from '../ui';

const BUTTON_ICON_SIZE = 16;
// As an object, because an empty string would go through an HTML parser, which the
// Content Security Policy refuses.
const EMPTY_COMMENT: Json = { type: 'doc', content: [{ type: 'paragraph' }] };

const SubmitKey = Extension.create<{ submit: () => void }>({
  name: 'submitComment',
  addOptions() {
    return { submit: () => undefined };
  },
  addKeyboardShortcuts() {
    const { submit } = this.options;
    return {
      'Mod-Enter': () => {
        submit();
        return true;
      },
    };
  },
});

function FormatButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      className="icon"
      aria-label={label}
      title={label}
      aria-pressed={active}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function Formats({ editor, onImage }: { editor: Editor; onImage?: (file: File) => void }) {
  const { t } = useTranslation();
  const picker = useRef<HTMLInputElement>(null);
  const chain = () => editor.chain().focus();
  return (
    <div className="comment-tools">
      <FormatButton
        label={t('editor.bold')}
        active={editor.isActive('bold')}
        onClick={() => chain().toggleBold().run()}
      >
        <IconBold size={BUTTON_ICON_SIZE} />
      </FormatButton>
      <FormatButton
        label={t('editor.italic')}
        active={editor.isActive('italic')}
        onClick={() => chain().toggleItalic().run()}
      >
        <IconItalic size={BUTTON_ICON_SIZE} />
      </FormatButton>
      <FormatButton
        label={t('editor.code')}
        active={editor.isActive('code')}
        onClick={() => chain().toggleCode().run()}
      >
        <IconCode size={BUTTON_ICON_SIZE} />
      </FormatButton>
      <FormatButton
        label={t('editor.bullet')}
        active={editor.isActive('bulletList')}
        onClick={() => chain().toggleBulletList().run()}
      >
        <IconBullet size={BUTTON_ICON_SIZE} />
      </FormatButton>
      {onImage && (
        <>
          <FormatButton label={t('comments.addImage')} active={false} onClick={() => picker.current?.click()}>
            <IconPicture size={BUTTON_ICON_SIZE} />
          </FormatButton>
          <input
            ref={picker}
            type="file"
            accept="image/*"
            hidden
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) onImage(file);
              e.target.value = '';
            }}
          />
        </>
      )}
    </div>
  );
}

/** A comment being written. Its draft stays in the browser until it is posted. */
export function CommentBox({
  pageId,
  draftKey,
  label,
  submitLabel,
  initial,
  busy,
  autoFocus,
  canAttach,
  onSubmit,
  onCancel,
}: {
  pageId: string;
  draftKey: string;
  label: string;
  submitLabel: string;
  initial?: Json;
  busy: boolean;
  autoFocus?: boolean;
  canAttach?: boolean;
  onSubmit: (body: Json, clear: () => void) => void;
  onCancel?: () => void;
}) {
  const { t } = useTranslation();
  const toast = useToast();
  const person = usePerson();
  const submit = useRef<() => void>(() => undefined);
  const [empty, setEmpty] = useState(true);
  const [failed, setFailed] = useState<unknown>(null);
  // Sharing happens on the page itself, so from an editor the warning stands alone.
  const warnNoAccess = (name: string) => {
    const onPage = match('/pages/:id', window.location.pathname);
    toast({
      message: t('mentions.noAccess', { name }),
      action: onPage
        ? { label: t('share.button'), run: () => navigate(`/pages/${pageId}?share=1`) }
        : undefined,
    });
  };
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: false,
        blockquote: false,
        horizontalRule: false,
        strike: false,
        underline: false,
        link: { openOnClick: false, protocols: ['http', 'https', 'mailto'] },
      }),
      Mention,
      AttachmentImage,
      MentionSuggestions.configure({ pageId, onNoAccess: (name) => warnNoAccess(name) }),
      SubmitKey.configure({ submit: () => submit.current() }),
    ],
    content: initial ?? readDraft(person, draftKey) ?? EMPTY_COMMENT,
    injectCSS: false,
    autofocus: autoFocus ? 'end' : false,
    editorProps: {
      attributes: {
        class: 'comment-input',
        'aria-label': label,
        role: 'textbox',
        'aria-multiline': 'true',
      },
    },
    onCreate: ({ editor: e }) => setEmpty(e.isEmpty),
    onUpdate: ({ editor: e }) => {
      setEmpty(e.isEmpty);
      if (e.isEmpty) dropDraft(person, draftKey);
      else keepDraft(person, draftKey, e.getJSON() as Json);
    },
  });

  const clear = () => {
    dropDraft(person, draftKey);
    editor.commands.setContent(EMPTY_COMMENT, { emitUpdate: true });
  };
  const addImage = async (file: File) => {
    setFailed(null);
    try {
      const path = `/pages/${pageId}/attachments?filename=${encodeURIComponent(file.name)}`;
      const attachment = await api.upload<Attachment>(path, file);
      if (attachment.inline) editor.chain().focus().insertContent(attachmentNode(attachment)).run();
      else setFailed(new Error(t('comments.notAnImage')));
    } catch (error) {
      setFailed(error);
    }
  };
  submit.current = () => {
    if (!editor.isEmpty && !busy) onSubmit(editor.getJSON() as Json, clear);
  };

  // `m` moves here, by the id of the box under the page.
  useEffect(() => {
    if (draftKey.endsWith(':page')) editor.view.dom.id = 'comment-box';
  }, [editor, draftKey]);

  return (
    <div className="comment-box">
      <Formats editor={editor} onImage={canAttach ? (file) => void addImage(file) : undefined} />
      <EditorContent editor={editor} />
      <ErrorNote error={failed} />
      <div className="comment-actions">
        <button type="button" className="primary" disabled={empty || busy} onClick={() => submit.current()}>
          {submitLabel}
        </button>
        {onCancel && (
          <button type="button" className="quiet" onClick={onCancel}>
            {t('common.cancel')}
          </button>
        )}
      </div>
    </div>
  );
}
