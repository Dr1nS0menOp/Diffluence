// SPDX-License-Identifier: Apache-2.0
// Characters a keyboard does not have, made from what people type anyway (EDT-029, 030).
// The rules of the library stay out of code by themselves, and Backspace right after one
// brings back what was typed.
import { Extension, InputRule } from '@tiptap/core';
import replacements from './typing.json';

// In typing.json, like the emoji list: these go into the text of a document, they are not
// part of our screens. The first rule that fits wins, so an opening quote comes before a
// closing one. The first group of a rule is what stays in front of the replaced
// characters. A face with a letter waits for a space: `:D` can also start an emoji search.

export const TypingHelp = Extension.create<{ isOn: () => boolean }>({
  name: 'typingHelp',
  addOptions() {
    return { isOn: () => true };
  },
  addInputRules() {
    const { isOn } = this.options;
    return replacements.map(
      ({ find, text }) =>
        new InputRule({
          find: new RegExp(find),
          handler: ({ state, range, match }) => {
            if (!isOn()) return null;
            state.tr.insertText(text, range.from + (match[1]?.length ?? 0), range.to);
            return undefined;
          },
        }),
    );
  },
});
