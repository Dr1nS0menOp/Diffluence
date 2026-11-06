// SPDX-License-Identifier: Apache-2.0
// Import Word in the Insert list: the content of a file lands at the cursor (EXC-029).
import type { Editor } from '@tiptap/core';
import { api, type Json } from '../../api';

interface DraftContent {
  content: Json[];
  warnings: string[];
}

/** Lets a person pick a .docx, and adds what it holds to the draft. */
export function pickWordFile(editor: Editor, pageId: string, onMessage: (text: string) => void): void {
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = '.docx';
  input.onchange = () => {
    const file = input.files?.[0];
    if (!file) return;
    api
      .upload<DraftContent>(`/pages/${pageId}/import-word`, file)
      .then((got) => {
        editor.chain().focus().insertContent(got.content).run();
        got.warnings.forEach(onMessage);
      })
      .catch((e: unknown) => onMessage(e instanceof Error ? e.message : String(e)));
  };
  input.click();
}
