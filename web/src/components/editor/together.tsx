// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { HocuspocusProvider } from '@hocuspocus/provider';
import { yUndoPluginKey } from '@tiptap/y-tiptap';
import type { Editor } from '@tiptap/core';
import type { EditorState } from '@tiptap/pm/state';
import { IndexeddbPersistence } from 'y-indexeddb';
import * as Y from 'yjs';
import type { Json } from '../../api';
import { markSharedSaved, markSharedUnsaved, sharedDocumentName } from '../../personal';
import { Avatar } from '../ui';

const ACCESS_ENDED = 'access-ended';
const TITLE_KEY = 'title';
const META_KEY = 'meta';

/** What the API says about editing this page together. */
export interface CollabInfo {
  enabled: boolean;
  user_id: string;
  name: string;
}

export interface Together {
  doc: Y.Doc;
  provider: HocuspocusProvider;
  userId: string;
  name: string;
}

export type LinkState = 'connecting' | 'online' | 'offline' | 'ended';

/** The socket of one page. The API checks the right to edit it before the socket opens. */
function socketAddress(pageId: string): string {
  const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${scheme}://${window.location.host}/collab?page=${encodeURIComponent(pageId)}`;
}

/** Connects to the service for as long as the editor is open, and keeps typing in the browser. */
export function useTogether(pageId: string, info: CollabInfo | null) {
  const [together, setTogether] = useState<Together | null>(null);
  const [link, setLink] = useState<LinkState>('connecting');
  const [ready, setReady] = useState(false);
  const userId = info?.user_id;
  const name = info?.name ?? '';
  useEffect(() => {
    if (!userId) return undefined;
    const doc = new Y.Doc();
    const kept = new IndexeddbPersistence(sharedDocumentName(userId, pageId), doc);
    const provider = new HocuspocusProvider({
      url: socketAddress(pageId),
      name: pageId,
      document: doc,
      // The session cookie proves to the API who this is, and the API gives the service a
      // ticket in its place. The protocol still wants a token.
      token: 'session',
      onSynced: () => setReady(true),
      onStatus: ({ status }) => {
        setLink((now) => {
          if (now === 'ended') return now;
          return status === 'connected' ? 'online' : 'offline';
        });
      },
      // What the service has not confirmed must survive logging out, as nobody has it yet.
      onUnsyncedChanges: ({ number }) => {
        if (number > 0) markSharedUnsaved(userId, pageId);
        else markSharedSaved(userId, pageId);
      },
      onAuthenticationFailed: () => setLink('ended'),
      onStateless: ({ payload }) => {
        if (payload === ACCESS_ENDED) setLink('ended');
      },
    });
    provider.setAwarenessField('user', { id: userId, name });
    void kept.whenSynced.then(() => {
      if (doc.getXmlFragment('default').length > 0) setReady(true);
    });
    setTogether({ doc, provider, userId, name });
    return () => {
      provider.destroy();
      void kept.destroy();
      doc.destroy();
      setTogether(null);
      setReady(false);
    };
  }, [pageId, userId, name]);
  return { together, link, ready };
}

/** Whether this browser fills a new shared document: the service names one person. */
export function shouldSeed(together: Together): boolean {
  const meta = together.doc.getMap(META_KEY);
  return meta.get('seeder') === together.userId && !meta.get('seeded');
}

/** Fills a new shared document with the published page, once. */
export function seed(together: Together, editor: Editor, document: Json, title: string) {
  if (!shouldSeed(together)) return;
  together.doc.transact(() => {
    together.doc.getMap(META_KEY).set('seeded', true);
    together.doc.getText(TITLE_KEY).insert(0, title);
  });
  editor.commands.setContent(document);
}

/** The title lives in the shared document, so everyone sees it change. */
export function useSharedTitle(together: Together | null, onTitle: (title: string) => void) {
  useEffect(() => {
    if (!together) return undefined;
    const text = together.doc.getText(TITLE_KEY);
    const show = () => {
      if (text.length > 0) onTitle(text.toString());
    };
    show();
    text.observe(show);
    return () => text.unobserve(show);
  }, [together, onTitle]);
}

export function writeSharedTitle(together: Together, title: string) {
  const text = together.doc.getText(TITLE_KEY);
  together.doc.transact(() => {
    text.delete(0, text.length);
    text.insert(0, title);
  });
}

/** A block command is one step for undo, also on the shared document. */
export function endUndoStep(state: EditorState) {
  const undo = yUndoPluginKey.getState(state) as { undoManager?: { stopCapturing: () => void } } | undefined;
  undo?.undoManager?.stopCapturing();
}

interface PresentEditor {
  id: string;
  name: string;
}

function editorsOf(together: Together): PresentEditor[] {
  const seen = new Map<string, string>();
  together.provider.awareness?.getStates().forEach((state) => {
    const user = state.user as PresentEditor | undefined;
    if (user && user.id !== together.userId) seen.set(user.id, user.name);
  });
  return [...seen].map(([id, name]) => ({ id, name }));
}

function jumpTo(id: string) {
  const carets = document.querySelectorAll<HTMLElement>('.collab-caret');
  const found = [...carets].find((caret) => caret.dataset.person === id);
  found?.scrollIntoView({ block: 'center' });
}

/** Everyone in this document, as buttons that lead to their cursor (COL-089, COL-093). */
export function TogetherPeople({ together }: { together: Together }) {
  const { t } = useTranslation();
  const [people, setPeople] = useState<PresentEditor[]>([]);
  useEffect(() => {
    const awareness = together.provider.awareness;
    const update = () => setPeople(editorsOf(together));
    update();
    awareness?.on('change', update);
    return () => awareness?.off('change', update);
  }, [together]);
  const names = useMemo(() => people.map((p) => p.name).join(', '), [people]);
  if (people.length === 0) return null;
  return (
    <span className="presence">
      {people.map((p) => (
        <button
          key={p.id}
          type="button"
          className="icon"
          aria-label={t('together.jump', { name: p.name })}
          title={t('together.jump', { name: p.name })}
          onClick={() => jumpTo(p.id)}
        >
          <Avatar name={p.name} size="small" />
        </button>
      ))}
      <span role="status" className="visually-hidden">
        {t('presence.here', { names })}
      </span>
    </span>
  );
}

/** A cursor with a name in words, so it never rests on colour (WCAG 1.4.1). */
export function renderCaret(user: Record<string, string>): HTMLElement {
  const caret = document.createElement('span');
  caret.className = 'collab-caret';
  caret.dataset.person = user.id ?? '';
  const label = document.createElement('span');
  label.className = 'collab-caret-name';
  label.textContent = user.name ?? '';
  caret.append(label);
  return caret;
}
