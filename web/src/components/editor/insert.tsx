// SPDX-License-Identifier: Apache-2.0
// Everything that can be added to a document, reached with `/`, `:` or the Insert tool.
import {
  forwardRef,
  useEffect,
  useId,
  useImperativeHandle,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Extension, type Editor } from '@tiptap/core';
import { NodeSelection, PluginKey } from '@tiptap/pm/state';
import { ReactRenderer } from '@tiptap/react';
import Suggestion, { type SuggestionProps } from '@tiptap/suggestion';
import { api, type MentionCandidate } from '../../api';
import i18n from '../../i18n';
import {
  IconAttach,
  IconBullet,
  IconCodeBlock,
  IconColumns,
  IconDate,
  IconDecision,
  IconEmoji,
  IconExpand,
  IconHeading,
  IconLink,
  IconNumbered,
  IconPanelError,
  IconPanelInfo,
  IconPanelNote,
  IconPanelSuccess,
  IconPanelWarning,
  IconQuote,
  IconRule,
  IconStatus,
  IconRename,
  IconTable,
  IconTask,
  IconText,
} from '../icons';
import { Dialog, Field } from '../ui';
import { insertColumns } from './block-commands';
import emoji from './emoji.json';
import { today } from './inline';
import { noActions, type EditorActions } from './keys';
import { setTextStyle } from './marks';
import type { PanelKind } from './palette';

const HEADING_LEVELS = [1, 2, 3, 4, 5, 6] as const;
const ITEM_ICON_SIZE = 18;
const MAX_EMOJI_RESULTS = 8;

export interface InsertItem {
  id: string;
  icon: ComponentType<{ size?: number }>;
  run: (editor: Editor, actions: EditorActions) => void;
}

const panelItem = (id: string, kind: PanelKind, icon: InsertItem['icon']): InsertItem => ({
  id,
  icon,
  run: (editor) => editor.chain().focus().wrapIn('panel', { kind }).run(),
});

/** Text goes in as a node: a plain string would be parsed as HTML, which Trusted Types refuses. */
const insertText = (editor: Editor, text: string) =>
  editor.chain().focus().insertContent({ type: 'text', text }).run();

/** Adds an item that lives in a line, and selects it so its tools appear in the row. */
function insertSelected(editor: Editor, type: string, attrs: Record<string, string>) {
  const at = editor.state.selection.from;
  editor.chain().focus().insertContent({ type, attrs }).run();
  const { tr, doc } = editor.state;
  editor.view.dispatch(tr.setSelection(NodeSelection.create(doc, at)));
}

export const INSERT_ITEMS: InsertItem[] = [
  { id: 'text', icon: IconText, run: (editor) => editor.chain().focus().clearNodes().run() },
  ...HEADING_LEVELS.map((level) => ({
    id: `h${level}`,
    icon: IconHeading,
    run: (editor: Editor) => setTextStyle(editor, level),
  })),
  { id: 'bullet', icon: IconBullet, run: (editor) => editor.chain().focus().toggleBulletList().run() },
  { id: 'ordered', icon: IconNumbered, run: (editor) => editor.chain().focus().toggleOrderedList().run() },
  { id: 'task', icon: IconTask, run: (editor) => editor.chain().focus().toggleTaskList().run() },
  { id: 'quote', icon: IconQuote, run: (editor) => editor.chain().focus().toggleBlockquote().run() },
  { id: 'code', icon: IconCodeBlock, run: (editor) => editor.chain().focus().toggleCodeBlock().run() },
  { id: 'rule', icon: IconRule, run: (editor) => editor.chain().focus().setHorizontalRule().run() },
  panelItem('panelInfo', 'info', IconPanelInfo),
  panelItem('panelNote', 'note', IconPanelNote),
  panelItem('panelSuccess', 'success', IconPanelSuccess),
  panelItem('panelWarning', 'warning', IconPanelWarning),
  panelItem('panelError', 'error', IconPanelError),
  { id: 'expand', icon: IconExpand, run: (editor) => editor.chain().focus().wrapIn('expand').run() },
  { id: 'columns2', icon: IconColumns, run: (editor) => insertColumns(editor, 2) },
  { id: 'columns3', icon: IconColumns, run: (editor) => insertColumns(editor, 3) },
  { id: 'decision', icon: IconDecision, run: (editor) => editor.chain().focus().setNode('decision').run() },
  {
    id: 'table',
    icon: IconTable,
    run: (editor) => editor.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
  },
  { id: 'attach', icon: IconAttach, run: (_editor, actions) => actions.pickFiles() },
  { id: 'link', icon: IconLink, run: (_editor, actions) => actions.openLink() },
  {
    id: 'status',
    icon: IconStatus,
    run: (editor) =>
      insertSelected(editor, 'status', { text: i18n.t('editor.statusDefault'), color: 'blue' }),
  },
  { id: 'date', icon: IconDate, run: (editor) => insertSelected(editor, 'date', { value: today() }) },
  { id: 'emoji', icon: IconEmoji, run: (editor) => insertText(editor, ':') },
];

/** Only a template asks questions, so only there do these appear. */
export const TEMPLATE_ITEMS: InsertItem[] = [
  { id: 'field', icon: IconRename, run: (_editor, actions) => actions.openTemplateItem('field') },
  {
    id: 'instruction',
    icon: IconPanelNote,
    run: (_editor, actions) => actions.openTemplateItem('instruction'),
  },
];

const TURN_INTO_IDS = [
  'text',
  'h1',
  'h2',
  'h3',
  'bullet',
  'ordered',
  'task',
  'quote',
  'code',
  'panelInfo',
  'expand',
];

export const TURN_INTO_ITEMS = INSERT_ITEMS.filter((item) => TURN_INTO_IDS.includes(item.id));

/** Looks in names and in the extra words a person might try. */
export function matchingItems(query: string, items: InsertItem[] = INSERT_ITEMS): InsertItem[] {
  const needle = query.trim().toLowerCase();
  return items.filter((item) => {
    const words = `${i18n.t(`insert.${item.id}.name`)} ${i18n.t(`insert.${item.id}.keywords`)}`;
    return words.toLowerCase().includes(needle);
  });
}

export function matchingEmoji(query: string): typeof emoji {
  const needle = query.toLowerCase();
  return emoji.filter((entry) => entry.words.includes(needle)).slice(0, MAX_EMOJI_RESULTS);
}

interface ListEntry {
  id: string;
  name: string;
  hint?: string;
  icon?: ReactNode;
}

interface ListProps {
  entries: ListEntry[];
  label: string;
  editor: Editor;
  choose: (index: number) => void;
}

interface ListHandle {
  onKeyDown: (event: KeyboardEvent) => boolean;
}

/** The popup at the cursor. The editor keeps focus, as the ARIA combobox pattern asks. */
const SuggestionList = forwardRef<ListHandle, ListProps>(function SuggestionList(
  { entries, label, editor, choose },
  ref,
) {
  const [active, setActive] = useState(0);
  const [shown, setShown] = useState(entries);
  const listId = useId();
  if (shown !== entries) {
    setShown(entries);
    setActive(0);
  }

  useEffect(() => {
    const dom = editor.view.dom;
    const optionId = `${listId}-${active}`;
    dom.setAttribute('aria-controls', listId);
    dom.setAttribute('aria-activedescendant', optionId);
    document.getElementById(optionId)?.scrollIntoView({ block: 'nearest' });
    return () => {
      dom.removeAttribute('aria-controls');
      dom.removeAttribute('aria-activedescendant');
    };
  }, [editor, listId, active]);

  useImperativeHandle(ref, () => ({
    onKeyDown(event) {
      if (entries.length === 0) return false;
      if (event.key === 'ArrowDown') setActive((active + 1) % entries.length);
      else if (event.key === 'ArrowUp') setActive((active - 1 + entries.length) % entries.length);
      else if (event.key === 'Enter' || event.key === 'Tab') choose(active);
      else return false;
      return true;
    },
  }));

  if (entries.length === 0) return null;
  return (
    <ul id={listId} role="listbox" aria-label={label} className="suggestions">
      {entries.map((entry, index) => (
        <li
          key={entry.id}
          id={`${listId}-${index}`}
          role="option"
          aria-selected={index === active}
          className={index === active ? 'suggestion active' : 'suggestion'}
          onMouseEnter={() => setActive(index)}
          onMouseDown={(e) => {
            e.preventDefault();
            choose(index);
          }}
        >
          {entry.icon}
          <span className="suggestion-name">{entry.name}</span>
          {entry.hint && <span className="muted suggestion-hint">{entry.hint}</span>}
        </li>
      ))}
    </ul>
  );
});

/** Connects a popup to the Suggestion utility, which places it near the cursor. */
function popupRender<Item>(label: () => string, toEntry: (item: Item) => ListEntry) {
  return () => {
    let popup: ReactRenderer<ListHandle, ListProps> | null = null;
    let unmount: () => void = () => undefined;
    const propsFor = (props: SuggestionProps<Item>): ListProps => ({
      entries: props.items.map(toEntry),
      label: label(),
      editor: props.editor,
      choose: (index) => {
        const item = props.items[index];
        if (item) props.command(item);
      },
    });
    return {
      onStart(props: SuggestionProps<Item>) {
        popup = new ReactRenderer(SuggestionList, {
          props: propsFor(props),
          editor: props.editor,
          className: 'suggestion-popup',
        });
        unmount = props.mount(popup.element as HTMLElement);
      },
      onUpdate(props: SuggestionProps<Item>) {
        popup?.updateProps(propsFor(props));
      },
      onKeyDown({ event }: { event: KeyboardEvent }) {
        return popup?.ref?.onKeyDown(event) ?? false;
      },
      onExit() {
        unmount();
        popup?.destroy();
      },
    };
  };
}

const itemEntry = (item: InsertItem): ListEntry => ({
  id: item.id,
  name: i18n.t(`insert.${item.id}.name`),
  hint: i18n.t(`insert.${item.id}.hint`),
  icon: <item.icon size={ITEM_ICON_SIZE} />,
});

export const SlashCommands = Extension.create<{ actions: () => EditorActions; items: InsertItem[] }>({
  name: 'slashCommands',
  // Before our own keys, so that Escape closes the menu first.
  priority: 1100,
  addOptions() {
    return { actions: () => noActions, items: INSERT_ITEMS };
  },
  addProseMirrorPlugins() {
    const { actions, items } = this.options;
    return [
      Suggestion<InsertItem>({
        editor: this.editor,
        char: '/',
        allowSpaces: true,
        pluginKey: new PluginKey('slashCommands'),
        allow: ({ editor }) => !editor.isActive('codeBlock'),
        items: ({ query }) => matchingItems(query, items),
        command: ({ editor, range, props }) => {
          editor.chain().focus().deleteRange(range).run();
          props.run(editor, actions());
        },
        render: popupRender<InsertItem>(() => i18n.t('editor.insert'), itemEntry),
      }),
    ];
  },
});

export const EmojiSuggestions = Extension.create({
  name: 'emojiSuggestions',
  priority: 1100,
  addProseMirrorPlugins() {
    return [
      Suggestion<(typeof emoji)[number]>({
        editor: this.editor,
        char: ':',
        pluginKey: new PluginKey('emojiSuggestions'),
        minQueryLength: 1,
        allow: ({ editor }) => !editor.isActive('codeBlock'),
        items: ({ query }) => matchingEmoji(query),
        command: ({ editor, range, props }) => {
          editor.chain().focus().insertContentAt(range, { type: 'text', text: props.glyph }).run();
        },
        render: popupRender<(typeof emoji)[number]>(
          () => i18n.t('insert.emoji.name'),
          (entry) => ({ id: entry.glyph, name: entry.glyph, hint: entry.words }),
        ),
      }),
    ];
  },
});

/** `@` opens the same list as `/`: people who can see the page come first. */
export const MentionSuggestions = Extension.create<{ pageId: string; onNoAccess: (name: string) => void }>({
  name: 'mentionSuggestions',
  priority: 1100,
  addOptions() {
    return { pageId: '', onNoAccess: () => undefined };
  },
  addProseMirrorPlugins() {
    const { pageId, onNoAccess } = this.options;
    return [
      Suggestion<MentionCandidate>({
        editor: this.editor,
        char: '@',
        pluginKey: new PluginKey('mentionSuggestions'),
        allow: ({ editor }) => !editor.isActive('codeBlock'),
        items: async ({ query }) => {
          const path = `/pages/${pageId}/mention-candidates?q=${encodeURIComponent(query)}`;
          const found = await api.get<{ candidates: MentionCandidate[] }>(path);
          return found.candidates;
        },
        command: ({ editor, range, props }) => {
          const attrs = { id: props.id, kind: props.kind, label: props.name };
          editor
            .chain()
            .focus()
            .insertContentAt(range, [
              { type: 'mention', attrs },
              { type: 'text', text: ' ' },
            ])
            .run();
          if (!props.can_view) onNoAccess(props.name);
        },
        render: popupRender<MentionCandidate>(
          () => i18n.t('mentions.title'),
          (entry) => ({
            id: entry.id,
            name: entry.name,
            hint: entry.can_view
              ? entry.kind === 'group'
                ? i18n.t('mentions.members', { count: entry.members ?? 0 })
                : ''
              : i18n.t('mentions.cannotSee'),
          }),
        ),
      }),
    ];
  },
});

export function InsertDialog({
  editor,
  actions,
  onClose,
}: {
  editor: Editor;
  actions: EditorActions;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const items = matchingItems(query);
  const choose = (item: InsertItem) => {
    onClose();
    item.run(editor, actions);
  };
  return (
    <Dialog title={t('editor.insert')} onClose={onClose}>
      <Field label={t('editor.insertSearch')}>
        <input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            const first = items[0];
            if (e.key !== 'Enter' || !first) return;
            e.preventDefault();
            choose(first);
          }}
        />
      </Field>
      <ul className="plain-list insert-list">
        {items.map((item) => (
          <li key={item.id}>
            <button type="button" className="suggestion" onClick={() => choose(item)}>
              <item.icon size={ITEM_ICON_SIZE} />
              <span className="suggestion-name">{t(`insert.${item.id}.name`)}</span>
              <span className="muted suggestion-hint">{t(`insert.${item.id}.hint`)}</span>
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
}
