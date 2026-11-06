// SPDX-License-Identifier: Apache-2.0
// Backspace right after a shortcut brings back what was typed (EDT-028). The library can
// do that too, but forgets it as soon as anything follows, and the empty line it adds
// after a last block always does. So we keep the change here, with what followed by
// itself, and take back both.
import { Plugin, PluginKey, type EditorState, type Transaction } from '@tiptap/pm/state';
import type { EditorView } from '@tiptap/pm/view';

interface RuleResult {
  transform: Transaction;
  from: number;
  to: number;
  text: string;
}

interface UndoableRule extends RuleResult {
  /** What the editor added by itself straight after the rule. */
  followedBy: Transaction[];
}

const ruleUndoKey = new PluginKey<UndoableRule | null>('ruleUndo');

function ruleResultOf(tr: Transaction, state: EditorState): RuleResult | undefined {
  // The library may hold more than one plugin of rules, and only the one that fired says so.
  return state.plugins
    .filter((plugin) => (plugin.spec as { isInputRules?: boolean }).isInputRules)
    .map((plugin) => tr.getMeta(plugin) as RuleResult | undefined)
    .find((result) => result !== undefined);
}

export const ruleUndoPlugin = () =>
  new Plugin<UndoableRule | null>({
    key: ruleUndoKey,
    state: {
      init: () => null,
      apply(tr, previous, _old, next) {
        const result = ruleResultOf(tr, next);
        if (result?.transform) return { ...result, followedBy: [] };
        if (previous && tr.getMeta('appendedTransaction')) {
          return { ...previous, followedBy: [...previous.followedBy, tr] };
        }
        return tr.selectionSet || tr.docChanged ? null : previous;
      },
    },
  });

function revert(tr: Transaction, change: Transaction) {
  for (let at = change.steps.length - 1; at >= 0; at -= 1) {
    const step = change.steps[at];
    const before = change.docs[at];
    if (step && before) tr.step(step.invert(before));
  }
}

/** Takes back the shortcut that just fired. Says whether there was one. */
export function undoLastRule(view: EditorView): boolean {
  const undoable = ruleUndoKey.getState(view.state);
  if (!undoable) return false;
  const { tr, schema } = view.state;
  [...undoable.followedBy].reverse().forEach((change) => revert(tr, change));
  revert(tr, undoable.transform);
  const marks = tr.doc.resolve(undoable.from).marks();
  tr.replaceWith(undoable.from, undoable.to, schema.text(undoable.text, marks));
  view.dispatch(tr);
  return true;
}
