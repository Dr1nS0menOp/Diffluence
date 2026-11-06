// SPDX-License-Identifier: Apache-2.0
// The tools of a selected link. They sit in the row, as all selection tools do (EDT-063).
import { useTranslation } from 'react-i18next';
import { useEditorState, type Editor } from '@tiptap/react';
import { IconCopy, IconExternal, IconRename, IconUnlink } from '../icons';
import { useCopy } from '../toast';
import type { EditorActions } from './keys';
import {
  addressOf,
  currentLink,
  insertPageLink,
  linkedPageOf,
  unlink,
  type LinkDisplay,
} from './link-commands';
import { useLinkFact } from './page-links';
import { Tool, TOOL_ICON_SIZE } from './tool';

const fullAddress = (address: string) => new URL(address, window.location.href).href;

function DisplayChoice({
  editor,
  page,
  fragment,
  display,
}: {
  editor: Editor;
  page: string;
  fragment: string | null;
  display: LinkDisplay;
}) {
  const { t } = useTranslation();
  const options: LinkDisplay[] =
    display === 'words' ? ['words', 'title', 'tag', 'card'] : ['title', 'tag', 'card'];
  return (
    <select
      className="tool-style"
      aria-label={t('editor.linkDisplay')}
      value={display}
      onChange={(e) => {
        const next = e.target.value as LinkDisplay;
        if (next !== 'words') insertPageLink(editor, page, fragment, next);
      }}
    >
      {options.map((option) => (
        <option key={option} value={option}>
          {t(`editor.linkDisplay_${option}`)}
        </option>
      ))}
    </select>
  );
}

export function LinkTools({ editor, actions }: { editor: Editor; actions: EditorActions }) {
  const { t } = useTranslation();
  const copy = useCopy();
  const link = useEditorState({ editor, selector: (s) => currentLink(s.editor) });
  const linked = linkedPageOf(link);
  const { fact } = useLinkFact(link?.kind === 'page' ? link.page : null);
  if (!link) return null;
  const address = addressOf(link);
  const leadsSomewhere = address !== null && !address.startsWith('#');
  const words = link.kind === 'placeholder' ? link.title : (fact?.title ?? t('page.linkHidden'));
  return (
    <>
      {leadsSomewhere && (
        <Tool
          editor={editor}
          label={t('editor.linkOpen')}
          run={() => window.open(fullAddress(address), '_blank', 'noopener')}
        >
          <IconExternal size={TOOL_ICON_SIZE} />
        </Tool>
      )}
      {leadsSomewhere && (
        <Tool
          editor={editor}
          label={t('editor.linkCopy')}
          run={() => void copy(fullAddress(address), t('editor.linkCopied'))}
        >
          <IconCopy size={TOOL_ICON_SIZE} />
        </Tool>
      )}
      <Tool editor={editor} label={t('editor.linkEdit')} run={actions.openLink}>
        <IconRename size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.linkRemove')} run={() => unlink(editor, words)}>
        <IconUnlink size={TOOL_ICON_SIZE} />
      </Tool>
      {linked && (
        <DisplayChoice
          editor={editor}
          page={linked.page}
          fragment={linked.fragment}
          display={link.kind === 'page' ? link.display : 'words'}
        />
      )}
    </>
  );
}
