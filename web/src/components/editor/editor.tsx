// SPDX-License-Identifier: Apache-2.0
import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type DragEvent,
  type MouseEvent,
  type ReactNode,
  type RefObject,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { EditorContent, useEditor, useEditorState } from '@tiptap/react';
import type { Editor } from '@tiptap/core';
import Collaboration from '@tiptap/extension-collaboration';
import CollaborationCaret from '@tiptap/extension-collaboration-caret';
import { NodeSelection } from '@tiptap/pm/state';
import type { EditorView } from '@tiptap/pm/view';
import { api, isCancelled, type Json, type UploadWatch } from '../../api';
import { offered, useCapabilities } from '../../capabilities';
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
  markDropTile,
  tileUnder,
  uploadAsking,
  uploadNewVersion,
  type FileAddress,
  type NameChoice,
} from './attachments';
import { columnsFromDrop, continueAtEnd, type DropSide } from './block-commands';
import { keepUnknownBlocks } from './blocks';
import { BrowseDialog } from './browse';
import { EmojiPicker } from './emoji-picker';
import { SavedBlocksDialog, savedBlockItem, useSavedBlocks } from './saved-blocks';
import { DescribeDialog, imagesWithoutWords } from './selection-tools';
import { DrawingDialog, showNewVersion, type DrawingFile } from './drawing';
import { BlockHandle, useBlockTracking } from './block-handle';
import { useCodeColours } from './code';
import { documentExtensions, knownBlockType, type TaskToggle } from './extensions';
import { FindBar, FindReplace } from './find';
import { GoneLinks, linkedPages, refreshGoneLinks } from './gone-links';
import {
  EmojiSuggestions,
  INSERT_ITEMS,
  InsertDialog,
  MentionSuggestions,
  SlashCommands,
  TEMPLATE_ITEMS,
  type InsertItem,
} from './insert';
import { EditorKeys, noActions, type EditorActions } from './keys';
import { cleanWordHtml, convertPastedMarkdown, looksLikeMarkdown, pastedImages } from './paste';
import { renderCaret, type Together } from './together';
import { LinkDialog } from './link-dialog';
import { TemplateItemDialog } from './template-item';
import { HeadingAnchors } from './outline';
import { TaskDates } from './people';
import { ReaderTables } from './reader-tables';
import { tableAround } from './table';
import { TableFrame, useTableTracking } from './table-frame';
import { TableRules } from './table-rules';
import { TableSizeDialog } from './table-size';
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

/** A link into a collapsible section opens it, or it would lead to nothing to see (EDT-116). */
function useLinksOpenSections(root: RefObject<HTMLDivElement | null>) {
  useEffect(() => {
    const open = () => {
      const target = decodeURIComponent(window.location.hash.slice(1));
      if (!target) return;
      const element = root.current?.querySelector(`[id="${CSS.escape(target)}"]`);
      for (
        let section = element?.closest('details');
        section;
        section = section.parentElement?.closest('details')
      ) {
        section.open = true;
      }
      element?.scrollIntoView();
    };
    open();
    window.addEventListener('hashchange', open);
    return () => window.removeEventListener('hashchange', open);
  }, [root]);
}

/** A document as this release can show it. What it does not know stays, as a note. */
function withUnknownBlocks(document: Json, known: (type: string) => boolean): Json {
  return keepUnknownBlocks(document as Parameters<typeof keepUnknownBlocks>[0], known) as Json;
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
  pageId,
  language,
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
  /** The published page shown, so that a link to a page that does not exist yet can start it. */
  pageId?: string;
  /** What the text is written in, for screen readers and hyphenation. */
  language?: string;
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
          sourcePage: pageId,
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
      content: withUnknownBlocks(document, knownBlockType),
      editable: false,
      injectCSS: false,
      editorProps: {
        attributes: {
          class: 'prose',
          'aria-label': t('editor.content'),
          ...(language ? { lang: language } : {}),
        },
      },
    },
    [document],
  );
  const root = useRef<HTMLDivElement>(null);
  usePrintOpensSections();
  useLinksOpenSections(root);
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
    const plainClick = e.button === 0 && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey;
    const here = target.closest<HTMLAnchorElement>('a[href^="#"]');
    if (here && plainClick) {
      // Links open in a new tab, which makes no sense for a place on this very page.
      e.preventDefault();
      window.location.hash = here.getAttribute('href') ?? '';
      return;
    }
    const link = target.closest<HTMLAnchorElement>('a[href^="/"]');
    if (!link || !plainClick || link.hasAttribute('download')) return;
    e.preventDefault();
    navigate(link.getAttribute('href') ?? '/');
  };

  return (
    // The keyboard reaches links by themselves, and images through their own buttons.
    <div
      ref={root}
      onClick={follow}
      onKeyDown={(e) => {
        if ((e.key === 'Enter' || e.key === ' ') && openMark(e.target as HTMLElement)) e.preventDefault();
      }}
    >
      <EditorContent editor={editor} />
    </div>
  );
}

/** How much of a block counts as its edge, where a drop makes columns instead of a new line. */
const BESIDE_EDGE_SHARE = 0.1;

interface BesidePlace {
  pos: number;
  side: DropSide;
  box: DOMRect;
}

interface BesideLine {
  insetBlockStart: number;
  insetInlineStart: number;
  blockSize: number;
}

/** The block a drag stands at the edge of, if any. */
function besidePlace(view: EditorView, x: number, y: number): BesidePlace | null {
  const bounds = view.dom.getBoundingClientRect();
  const reach = bounds.width * BESIDE_EDGE_SHARE;
  // A drop in the text means a new place in the page, so one side counts from the margin on.
  const isLeft = x < bounds.left;
  const isRight = x > bounds.right - reach;
  const runsRight = getComputedStyle(view.dom).direction !== 'rtl';
  const side = isLeft ? (runsRight ? 'start' : 'end') : isRight ? (runsRight ? 'end' : 'start') : null;
  const found = side && view.posAtCoords({ left: bounds.left + bounds.width / 2, top: y });
  if (!side || !found) return null;
  const inside = found.inside >= 0 ? found.inside : found.pos;
  const $pos = view.state.doc.resolve(inside);
  const pos = $pos.depth > 0 ? $pos.before(1) : inside;
  const block = view.nodeDOM(pos);
  return block instanceof HTMLElement ? { pos, side, box: block.getBoundingClientRect() } : null;
}

function besideLineOf(place: BesidePlace, sheet: HTMLElement): BesideLine {
  const outer = sheet.getBoundingClientRect();
  const x = place.side === 'start' ? place.box.left : place.box.right;
  return {
    insetBlockStart: place.box.top - outer.top,
    insetInlineStart: x - outer.left,
    blockSize: place.box.height,
  };
}

/** The dashed line where a dropped block would land. Only our code knows these numbers. */
function BesideMark({ line }: { line: BesideLine }) {
  const mark = useRef<HTMLSpanElement>(null);
  useLayoutEffect(() => {
    if (!mark.current) return;
    mark.current.style.insetBlockStart = `${line.insetBlockStart}px`;
    mark.current.style.insetInlineStart = `${line.insetInlineStart}px`;
    mark.current.style.blockSize = `${line.blockSize}px`;
  }, [line]);
  return <span ref={mark} className="beside-line" />;
}

/** A quiet word when images have nothing for screen readers (EDT-075). */
function ImagesHint({ editor }: { editor: Editor }) {
  const { t } = useTranslation();
  const count = useEditorState({ editor, selector: (s) => imagesWithoutWords(s.editor.state.doc) });
  if (count === 0) return null;
  return <p className="muted images-hint">{t('editor.imagesWithoutWords', { count })}</p>;
}

/** Below this, an upload is gone before anyone could read a number (spec 024, D21). */
const SHOWN_PROGRESS_BYTES = 1 << 20;

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
  language,
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
  /** What the text is written in, which the spell checker follows. */
  language?: string;
}) {
  const { t } = useTranslation();
  const toast = useToast();
  const copy = useCopy();
  const person = usePerson();
  const typingHelp = useRef(true);
  typingHelp.current = usePreferences().preferences.typing_help;
  const client = useQueryClient();
  const [uploading, setUploading] = useState<{ name: string; percent: number | null } | null>(null);
  const stopUpload = useRef<AbortController | null>(null);
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
  const [sizingTable, setSizingTable] = useState(false);
  const [finding, setFinding] = useState(false);
  const [browsing, setBrowsing] = useState(false);
  const [pickingEmoji, setPickingEmoji] = useState(false);
  const [savingBlock, setSavingBlock] = useState(false);
  const [describing, setDescribing] = useState(false);
  const savedBlocks = useSavedBlocks();
  const savedItems = useRef<InsertItem[]>([]);
  savedItems.current = (savedBlocks.data ?? []).map(savedBlockItem);
  // The slash menu is built once, so it reads what is on through a ref (ARC-071).
  const capabilities = useCapabilities();
  const offeredNow = useRef(capabilities);
  useEffect(() => {
    offeredNow.current = capabilities;
  }, [capabilities]);
  const [templateItem, setTemplateItem] = useState<'field' | 'instruction' | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const plainPaste = useRef(false);
  const change = useRef(onChange);
  change.current = onChange;

  const watchUpload = (file: File): UploadWatch => {
    const stop = new AbortController();
    stopUpload.current = stop;
    const isLarge = file.size >= SHOWN_PROGRESS_BYTES;
    setUploading({ name: file.name, percent: isLarge ? 0 : null });
    return {
      signal: stop.signal,
      onProgress: (share) => {
        if (isLarge) setUploading({ name: file.name, percent: Math.round(share * 100) });
      },
    };
  };

  const addVersion = async (id: string, file: File) => {
    setUploadError(null);
    if (!canAttach) {
      setUploadError(new Error(t('editor.cannotAttach')));
      return;
    }
    try {
      const attachment = await uploadNewVersion(id, file, watchUpload(file));
      await client.invalidateQueries({ queryKey: ['file-facts', id] });
      toast({ message: t('files.versionAdded', { name: attachment.filename, number: attachment.version }) });
    } catch (error) {
      if (!isCancelled(error)) setUploadError(error);
    }
    setUploading(null);
  };

  /** Selects the image just added, and offers to describe it: screen readers need that (EDT-075). */
  const offerDescription = () => {
    if (!editor) return;
    const { doc, selection } = editor.state;
    const at = selection.from - 1;
    if (at >= 0 && doc.nodeAt(at)?.type.name === 'image') editor.commands.setNodeSelection(at);
    toast({
      message: t('editor.imageNeedsWords'),
      action: { label: t('editor.describeNow'), run: () => setDescribing(true) },
    });
  };

  const addFiles = async (files: File[]) => {
    setUploadError(null);
    if (!canAttach) {
      setUploadError(new Error(t('editor.cannotAttach')));
      return;
    }
    // One at a time, so files land in the order they were dropped.
    for (const file of files) {
      try {
        const attachment = await uploadAsking(pageId, file, askAboutName, watchUpload(file));
        // After the selection, never over it: the previous file is still selected.
        if (attachment) {
          editor
            ?.chain()
            .focus()
            .insertContentAt(editor.state.selection.to, attachmentNode(attachment))
            .run();
          if (attachment.inline) offerDescription();
        }
      } catch (error) {
        if (!isCancelled(error)) setUploadError(error);
      }
    }
    setUploading(null);
  };
  const add = useRef(addFiles);
  add.current = addFiles;

  const [besideLine, setBesideLine] = useState<BesideLine | null>(null);
  const dropBeside = useRef<(view: EditorView, event: globalThis.DragEvent) => boolean>(() => false);
  const pasteMarkdown = useRef<(text: string) => void>(() => undefined);
  const takeImagesAlong = useRef((html: string) => {
    const { files, left } = pastedImages(html);
    if (files.length > 0) void add.current(files);
    if (left > 0) toast({ message: t('editor.imagesLeftBehind', { count: left }) });
  });

  // Extensions are made once, so they reach the latest functions through this ref.
  const actions = useRef<EditorActions>(noActions);
  const nestedTable = useRef(() => toast({ message: t('table.noNested') }));

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
        items: () => [
          ...offered(INSERT_ITEMS, offeredNow.current),
          ...(inTemplate ? TEMPLATE_ITEMS : []),
          ...savedItems.current,
        ],
        recent: () => readRecentItems(person),
        onUsed: (id) => rememberItem(person, id),
      }),
      TypingHelp.configure({ isOn: () => typingHelp.current }),
      EmojiSuggestions,
      MentionSuggestions.configure({ pageId, onNoAccess: (name) => actions.current.mentionNoAccess(name) }),
      FindReplace.configure({ onOpen: () => actions.current.openFind() }),
      EditorKeys.configure({ actions: () => actions.current }),
      TableRules.configure({ onNested: () => nestedTable.current() }),
    ],
    // A document even when empty: a string would be parsed as HTML, which our policy blocks.
    content: together
      ? { type: 'doc', content: [{ type: 'paragraph' }] }
      : withUnknownBlocks(initial, knownBlockType),
    injectCSS: false,
    editorProps: {
      attributes: {
        class: 'prose editing',
        'aria-label': t('editor.content'),
        role: 'textbox',
        'aria-multiline': 'true',
        spellcheck: 'true',
        ...(language ? { lang: language } : {}),
      },
      transformPastedHTML: cleanWordHtml,
      // Files are taken by the area around the editor, so a drop beside the page counts too.
      handleDrop: (view, event) => {
        if (carriesFiles(event.dataTransfer)) return true;
        return dropBeside.current(view, event);
      },
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
          takeImagesAlong.current(event.clipboardData?.getData('text/html') ?? '');
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
  dropBeside.current = (view, event) => {
    setBesideLine(null);
    const source = (view.dragging as { node?: unknown } | null)?.node;
    const place = besidePlace(view, event.clientX, event.clientY);
    if (!(source instanceof NodeSelection) || !place || source.$from.depth !== 0) return false;
    return columnsFromDrop(editor, source.from, place.pos, place.side);
  };
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
    openTableSize: () => {
      if (tableAround(editor)) nestedTable.current();
      else setSizingTable(true);
    },
    openTemplateItem: setTemplateItem,
    copyBlockLink: (anchor) => {
      const address = `${window.location.origin}/pages/${pageId}#${anchor}`;
      void copy(address, t('editor.blockLinkCopied'));
    },
    openSaveBlock: () => setSavingBlock(true),
    openEmojiPicker: () => setPickingEmoji(true),
    openDescribe: () => setDescribing(true),
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
    markDropTile(e.currentTarget, null);
    const tile = tileUnder(e.target);
    const [only, ...others] = Array.from(e.dataTransfer.files);
    if (tile && only && others.length === 0) {
      void addVersion(tile, only);
      return;
    }
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
        markDropTile(e.currentTarget, e.target);
      }}
      onDragLeave={(e) => {
        if (!(e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget))) {
          setDropping(false);
          markDropTile(e.currentTarget, null);
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
        onDragOver={(e) => {
          const place = editor.view.dragging ? besidePlace(editor.view, e.clientX, e.clientY) : null;
          setBesideLine(place && sheet.current ? besideLineOf(place, sheet.current) : null);
        }}
        onDragEnd={() => setBesideLine(null)}
      >
        {besideLine && <BesideMark line={besideLine} />}
        <BlockHandle editor={editor} tracker={tracker} actions={actions.current} />
        <TableFrame editor={editor} tracker={tables} />
        {uploading && (
          <p role="status" className="note row">
            <span>
              {uploading.percent === null
                ? t('editor.uploading', { name: uploading.name })
                : t('editor.uploadingShare', { name: uploading.name, percent: uploading.percent })}
            </span>
            {uploading.percent !== null && (
              <button type="button" className="ghost" onClick={() => stopUpload.current?.abort()}>
                {t('common.cancel')}
              </button>
            )}
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
        <ImagesHint editor={editor} />
        <WordCount editor={editor} />
        {footer}
      </div>
      {linking && <LinkDialog editor={editor} onClose={() => setLinking(false)} />}
      {sizingTable && <TableSizeDialog editor={editor} onClose={() => setSizingTable(false)} />}
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
        <InsertDialog
          editor={editor}
          actions={actions.current}
          onClose={() => setInserting(false)}
          onBrowse={() => {
            setInserting(false);
            setBrowsing(true);
          }}
        />
      )}
      {browsing && (
        <BrowseDialog editor={editor} actions={actions.current} onClose={() => setBrowsing(false)} />
      )}
      {pickingEmoji && <EmojiPicker editor={editor} onClose={() => setPickingEmoji(false)} />}
      {savingBlock && <SavedBlocksDialog editor={editor} onClose={() => setSavingBlock(false)} />}
      {describing && <DescribeDialog editor={editor} onClose={() => setDescribing(false)} />}
    </div>
  );
}
