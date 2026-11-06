// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useRef, useState, type DragEvent, type MouseEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { EditorContent, useEditor } from '@tiptap/react';
import type { Editor } from '@tiptap/core';
import Collaboration from '@tiptap/extension-collaboration';
import CollaborationCaret from '@tiptap/extension-collaboration-caret';
import type { EditorView } from '@tiptap/pm/view';
import { api, type Json } from '../../api';
import { readRecentItems, rememberItem, usePerson } from '../../personal';
import { usePreferences } from '../../preferences';
import { navigate } from '../../router';
import { CommentHighlights, refreshHighlights, type Highlight } from '../comments/anchor';
import { useCopy, useToast } from '../toast';
import { ErrorNote } from '../ui';
import type { ShownImage } from '../viewers';
import {
  attachmentNode,
  NameTakenDialog,
  PageFilesDialog,
  uploadAsking,
  type FileAddress,
  type NameChoice,
} from './attachments';
import { continueAtEnd } from './block-commands';
import { DrawingDialog, showNewVersion, type DrawingFile } from './drawing';
import { BlockHandle, useBlockTracking } from './block-handle';
import { useCodeColours } from './code';
import { documentExtensions, type TaskToggle } from './extensions';
import { FindBar, FindReplace } from './find';
import { GoneLinks, linkedPages, refreshGoneLinks } from './gone-links';
import {
  EmojiSuggestions,
  INSERT_ITEMS,
  InsertDialog,
  MentionSuggestions,
  SlashCommands,
  TEMPLATE_ITEMS,
} from './insert';
import { EditorKeys, noActions, type EditorActions } from './keys';
import { convertPastedMarkdown, looksLikeMarkdown } from './paste';
import { renderCaret, type Together } from './together';
import { LinkDialog } from './link-dialog';
import { TemplateItemDialog } from './template-item';
import { HeadingAnchors } from './outline';
import { TaskDates } from './people';
import { ReaderTables } from './reader-tables';
import { TableFrame, useTableTracking } from './table-frame';
import { Toolbar } from './toolbar';
import { TypingHelp } from './typing';
import { pickWordFile } from './word';
import { WordCount } from './word-count';

function shownImages(root: HTMLElement): { element: HTMLImageElement; shown: ShownImage }[] {
  return Array.from(root.querySelectorAll<HTMLImageElement>('figure.doc-image img')).map((element) => ({
    element,
    shown: {
      src: element.src,
      alt: element.alt,
      caption: element.closest('figure')?.querySelector('figcaption')?.textContent ?? '',
    },
  }));
}

/** Every collapsible section opens for printing, and goes back afterwards. */
function usePrintOpensSections() {
  useEffect(() => {
    const opened: HTMLDetailsElement[] = [];
    const before = () => {
      globalThis.document.querySelectorAll<HTMLDetailsElement>('.prose details:not([open])').forEach((d) => {
        d.open = true;
        opened.push(d);
      });
    };
    const after = () => {
      opened.splice(0).forEach((d) => {
        d.open = false;
      });
    };
    window.addEventListener('beforeprint', before);
    window.addEventListener('afterprint', after);
    return () => {
      window.removeEventListener('beforeprint', before);
      window.removeEventListener('afterprint', after);
    };
  }, []);
}

/** The pages this document links to that the reader cannot reach. */
function useGoneLinks(document: Json, enabled: boolean): Set<string> {
  const linked = useMemo(() => (enabled ? linkedPages(document) : []), [document, enabled]);
  const reachable = useQuery({
    queryKey: ['reachable', linked],
    queryFn: () => api.post<{ pages: string[] }>('/pages/reachable', { ids: linked }),
    enabled: linked.length > 0,
  });
  return useMemo(() => {
    const found = reachable.data?.pages;
    return new Set(found ? linked.filter((page) => !found.includes(page)) : []);
  }, [linked, reachable.data]);
}

/**
 * Shows a stored document. The editor renders it, so no HTML is ever injected.
 * `plain` leaves out what only makes sense on the page itself, such as heading links.
 */
export function DocumentView({
  document,
  plain,
  fileAddress,
  onImage,
  highlights,
  onFound,
  onHighlight,
  onTask,
  onView,
  checkLinks,
}: {
  document: Json;
  plain?: boolean;
  fileAddress?: FileAddress;
  onImage?: (images: ShownImage[], start: number) => void;
  highlights?: Highlight[];
  onFound?: (ids: string[]) => void;
  onHighlight?: (id: string) => void;
  onTask?: TaskToggle;
  onView?: (view: EditorView) => void;
  /** Marks links to what this reader cannot reach. Needs someone who is logged in. */
  checkLinks?: boolean;
}) {
  const { t } = useTranslation();
  const copy = useCopy();
  const goneLinks = useGoneLinks(document, checkLinks ?? false);
  const gone = useRef(goneLinks);
  gone.current = goneLinks;
  const marks = useRef<Highlight[]>([]);
  marks.current = highlights ?? [];
  const found = useRef(onFound);
  found.current = onFound;
  const tick = useRef(onTask);
  tick.current = onTask;
  const copyHeading = useRef((slug: string) => {
    const address = `${window.location.origin}${window.location.pathname}#${slug}`;
    void copy(address, t('page.headingLinkCopied'));
  });
  const editor = useEditor(
    {
      extensions: [
        ...documentExtensions({
          fileAddress,
          onTask: onTask ? (id, done) => tick.current?.(id, done) : undefined,
        }),
        TaskDates.configure({ overdueWord: t('tasks.overdue') }),
        GoneLinks.configure({ gone: () => gone.current, explanation: t('page.linkGone') }),
        ...(plain
          ? []
          : [
              HeadingAnchors.configure({
                label: t('page.copyHeadingLink'),
                onCopy: (slug) => copyHeading.current(slug),
              }),
              ReaderTables,
              CommentHighlights.configure({
                highlights: () => marks.current,
                // Later: this runs while the editor is made, in the middle of a render.
                onFound: (ids) => queueMicrotask(() => found.current?.(ids)),
              }),
            ]),
      ],
      content: document,
      editable: false,
      injectCSS: false,
      editorProps: { attributes: { class: 'prose', 'aria-label': t('editor.content') } },
    },
    [document],
  );
  usePrintOpensSections();
  useCodeColours(editor);

  useEffect(() => {
    if (!plain) editor.view.dispatch(editor.state.tr.setMeta(refreshHighlights, true));
  }, [editor, highlights, plain]);
  useEffect(() => {
    editor.view.dispatch(editor.state.tr.setMeta(refreshGoneLinks, true));
  }, [editor, goneLinks]);
  useEffect(() => onView?.(editor.view), [editor, onView]);

  const openMark = (target: HTMLElement): boolean => {
    const id = target.closest<HTMLElement>('[data-comment]')?.dataset.comment;
    if (id) onHighlight?.(id);
    return id !== undefined;
  };

  const follow = (e: MouseEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement;
    if (openMark(target)) return;
    const image = target.closest<HTMLImageElement>('figure.doc-image img');
    if (image && onImage) {
      const images = shownImages(e.currentTarget);
      onImage(
        images.map((i) => i.shown),
        images.findIndex((i) => i.element === image),
      );
      return;
    }
    const link = target.closest<HTMLAnchorElement>('a[href^="/"]');
    const plainClick = e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey;
    if (!link || !plainClick || link.hasAttribute('download')) return;
    e.preventDefault();
    navigate(link.getAttribute('href') ?? '/');
  };

  return (
    // The keyboard reaches links by themselves, and images through their own buttons.
    <div
      onClick={follow}
      onKeyDown={(e) => {
        if ((e.key === 'Enter' || e.key === ' ') && openMark(e.target as HTMLElement)) e.preventDefault();
      }}
    >
      <EditorContent editor={editor} />
    </div>
  );
}

const carriesFiles = (transfer: DataTransfer | null) => transfer?.types.includes('Files') ?? false;

/** The editing surface. Markdown shortcuts (#, **, -, ```) come from the input rules. */
export function DocumentEditor({
  pageId,
  initial,
  canAttach,
  inTemplate,
  fileAddress,
  header,
  footer,
  fullWidth,
  onFullWidth,
  onChange,
  onPublish,
  onSave,
  onView,
  onShare,
  together,
  onReady,
  onFocusTitle,
}: {
  pageId: string;
  initial: Json;
  canAttach: boolean;
  inTemplate?: boolean;
  fileAddress?: FileAddress;
  header: ReactNode;
  footer?: ReactNode;
  fullWidth?: boolean;
  onFullWidth?: () => void;
  onChange: (doc: Json) => void;
  onPublish: () => void;
  onSave: () => void;
  onView?: (view: EditorView) => void;
  onShare?: () => void;
  /** Editing together: the shared document replaces the initial text. */
  together?: Together | null;
  onReady?: (editor: Editor) => void;
  /** Backspace at the very start of the text goes here. */
  onFocusTitle?: () => void;
}) {
  const { t } = useTranslation();
  const toast = useToast();
  const person = usePerson();
  const typingHelp = useRef(true);
  typingHelp.current = usePreferences().preferences.typing_help;
  const [uploading, setUploading] = useState('');
  const [uploadError, setUploadError] = useState<unknown>(null);
  const [drawing, setDrawing] = useState<DrawingFile | 'new' | null>(null);
  const [pickingFile, setPickingFile] = useState(false);
  const [nameQuestion, setNameQuestion] = useState<{
    filename: string;
    answer: (choice: NameChoice) => void;
  } | null>(null);
  const askAboutName = (filename: string) =>
    new Promise<NameChoice>((resolve) => {
      setNameQuestion({ filename, answer: resolve });
    });
  const [dropping, setDropping] = useState(false);
  const [linking, setLinking] = useState(false);
  const [inserting, setInserting] = useState(false);
  const [finding, setFinding] = useState(false);
  const [templateItem, setTemplateItem] = useState<'field' | 'instruction' | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const plainPaste = useRef(false);
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
        const attachment = await uploadAsking(pageId, file, askAboutName);
        // After the selection, never over it: the previous file is still selected.
        if (attachment) {
          editor
            ?.chain()
            .focus()
            .insertContentAt(editor.state.selection.to, attachmentNode(attachment))
            .run();
        }
      } catch (error) {
        setUploadError(error);
      }
    }
    setUploading('');
  };
  const add = useRef(addFiles);
  add.current = addFiles;

  const pasteMarkdown = useRef<(text: string) => void>(() => undefined);

  // Extensions are made once, so they reach the latest functions through this ref.
  const actions = useRef<EditorActions>(noActions);

  const editor = useEditor({
    extensions: [
      ...documentExtensions({ fileAddress, shared: !!together }),
      ...(together
        ? [
            Collaboration.configure({ document: together.doc }),
            CollaborationCaret.configure({
              provider: together.provider,
              user: { id: together.userId, name: together.name },
              render: renderCaret,
              selectionRender: () => ({ class: 'collab-selection' }),
            }),
          ]
        : []),
      SlashCommands.configure({
        actions: () => actions.current,
        items: inTemplate ? [...INSERT_ITEMS, ...TEMPLATE_ITEMS] : INSERT_ITEMS,
        recent: () => readRecentItems(person),
        onUsed: (id) => rememberItem(person, id),
      }),
      TypingHelp.configure({ isOn: () => typingHelp.current }),
      EmojiSuggestions,
      MentionSuggestions.configure({ pageId, onNoAccess: (name) => actions.current.mentionNoAccess(name) }),
      FindReplace.configure({ onOpen: () => actions.current.openFind() }),
      EditorKeys.configure({ actions: () => actions.current }),
    ],
    // A document even when empty: a string would be parsed as HTML, which our policy blocks.
    content: together ? { type: 'doc', content: [{ type: 'paragraph' }] } : initial,
    injectCSS: false,
    editorProps: {
      attributes: {
        class: 'prose editing',
        'aria-label': t('editor.content'),
        role: 'textbox',
        'aria-multiline': 'true',
        spellcheck: 'true',
      },
      // Files are taken by the area around the editor, so a drop beside the page counts too.
      handleDrop: (_view, event) => carriesFiles(event.dataTransfer),
      handleKeyDown: (_view, event) => {
        plainPaste.current =
          (event.ctrlKey || event.metaKey) && event.shiftKey && event.key.toLowerCase() === 'v';
        return false;
      },
      handlePaste: (view, event) => {
        const images = Array.from(event.clipboardData?.files ?? []).filter((f) =>
          f.type.startsWith('image/'),
        );
        if (images.length === 0) {
          const inCode = view.state.selection.$from.parent.type.spec.code === true;
          const text = event.clipboardData?.getData('text/plain') ?? '';
          const formatted = event.clipboardData?.types.includes('text/html') ?? false;
          if (!plainPaste.current && !inCode && !formatted && looksLikeMarkdown(text)) {
            pasteMarkdown.current(text);
          }
          return false;
        }
        void add.current(images);
        return true;
      },
    },
    // The empty line after a last block is added by itself on the first click. Nobody typed it,
    // so it must not count, or merely opening a page would leave a draft behind.
    onUpdate: ({ editor: e, transaction }) => {
      if (transaction.docChanged) change.current(e.getJSON() as Json);
    },
  });
  pasteMarkdown.current = (text) => convertPastedMarkdown(editor, text);
  const tracker = useBlockTracking(editor);
  const sheet = useRef<HTMLDivElement>(null);
  const tables = useTableTracking(editor, sheet);
  useCodeColours(editor);
  useEffect(() => onView?.(editor.view), [editor, onView]);
  useEffect(() => onReady?.(editor), [editor, onReady]);

  actions.current = {
    pickFiles: () => fileInput.current?.click(),
    pickWord: () => pickWordFile(editor, pageId, (message) => toast({ message })),
    openLink: () => setLinking(true),
    openFind: () => setFinding(true),
    openDrawing: (existing) => setDrawing(existing ?? 'new'),
    openPageFiles: () => setPickingFile(true),
    openBlockMenu: tracker.openAtCursor,
    openTableMenu: tables.open,
    openTemplateItem: setTemplateItem,
    publish: onPublish,
    save: onSave,
    focusTitle: () => {
      onFocusTitle?.();
      return onFocusTitle !== undefined;
    },
    mentionNoAccess: (name) =>
      toast({
        message: t('mentions.noAccess', { name }),
        action: onShare ? { label: t('share.button'), run: onShare } : undefined,
      }),
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

  /** The sheet goes on below the text, and a click there means: go on writing. */
  const clickBelowText = (e: MouseEvent<HTMLDivElement>) => {
    const onEmptySheet = e.target === e.currentTarget || e.target === sheet.current;
    if (onEmptySheet && e.clientY > editor.view.dom.getBoundingClientRect().bottom) continueAtEnd(editor);
  };

  return (
    <div
      className={dropping ? 'editor-area dropping' : 'editor-area'}
      onClick={clickBelowText}
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
        fullWidth={fullWidth}
        onFullWidth={onFullWidth}
        onInsert={() => setInserting(true)}
        onFiles={(files) => void addFiles(files)}
      />
      {finding && <FindBar editor={editor} onClose={() => setFinding(false)} />}
      <div
        ref={sheet}
        className={fullWidth ? 'editor-sheet full' : 'editor-sheet'}
        onMouseMove={(e) => {
          tracker.track(e);
          tables.track(e);
        }}
        onMouseLeave={() => {
          tracker.leave();
          tables.leave();
        }}
      >
        <BlockHandle editor={editor} tracker={tracker} actions={actions.current} />
        <TableFrame editor={editor} tracker={tables} />
        {uploading && (
          <p role="status" className="note">
            {t('editor.uploading', { name: uploading })}
          </p>
        )}
        <ErrorNote error={uploadError} />
        {drawing && (
          <DrawingDialog
            pageId={pageId}
            existing={drawing === 'new' ? undefined : drawing}
            onDone={(attachment) => {
              setDrawing(null);
              if (drawing !== 'new') {
                showNewVersion(editor.view.dom, attachment.id);
                return;
              }
              editor
                ?.chain()
                .focus()
                .insertContentAt(editor.state.selection.to, attachmentNode(attachment))
                .run();
            }}
            onClose={() => setDrawing(null)}
          />
        )}
        {nameQuestion && (
          <NameTakenDialog
            filename={nameQuestion.filename}
            onAnswer={(choice) => {
              setNameQuestion(null);
              nameQuestion.answer(choice);
            }}
          />
        )}
        {header}
        <EditorContent editor={editor} />
        <WordCount editor={editor} />
        {footer}
      </div>
      {linking && <LinkDialog editor={editor} onClose={() => setLinking(false)} />}
      {pickingFile && (
        <PageFilesDialog
          pageId={pageId}
          onPick={(attachment) => {
            setPickingFile(false);
            editor
              .chain()
              .focus()
              .insertContentAt(editor.state.selection.to, attachmentNode(attachment))
              .run();
          }}
          onClose={() => setPickingFile(false)}
        />
      )}
      {templateItem && (
        <TemplateItemDialog editor={editor} what={templateItem} onClose={() => setTemplateItem(null)} />
      )}
      {inserting && (
        <InsertDialog editor={editor} actions={actions.current} onClose={() => setInserting(false)} />
      )}
    </div>
  );
}
