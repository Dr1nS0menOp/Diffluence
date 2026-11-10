// SPDX-License-Identifier: Apache-2.0
// One Tab stop for a row of tools, with arrow keys inside it (ARIA toolbar pattern). The
// fixed row and the floating bars both use it, so they behave the same (spec 047, D6).
import { useLayoutEffect, useRef, type FocusEvent, type KeyboardEvent, type RefObject } from 'react';

// Everything that can take focus in the row, except what sits in an open menu.
const CONTROLS = 'button:not(:disabled), select, input:not([hidden])';

function controlsIn(root: HTMLElement | null): HTMLElement[] {
  const all = Array.from(root?.querySelectorAll<HTMLElement>(CONTROLS) ?? []);
  return all.filter((control) => !control.closest('[role=menu]'));
}

/**
 * Where a key moves to, as an index into the controls. On a right-to-left page the arrows
 * swap, so each moves the way it points (spec 047, D7). Undefined for any other key.
 */
export function stepFor(key: string, at: number, count: number, rightToLeft: boolean): number | undefined {
  const next = (at + 1) % count;
  const previous = (at - 1 + count) % count;
  switch (key) {
    case 'ArrowRight':
      return rightToLeft ? previous : next;
    case 'ArrowLeft':
      return rightToLeft ? next : previous;
    case 'Home':
      return 0;
    case 'End':
      return count - 1;
    default:
      return undefined;
  }
}

/** One Tab stop for the whole row. Tools come and go with the selection, so it runs every time. */
export function useRovingFocus(root: RefObject<HTMLElement | null>, onLeave: () => void) {
  const current = useRef<HTMLElement | null>(null);

  useLayoutEffect(() => {
    const controls = controlsIn(root.current);
    const active = current.current && controls.includes(current.current) ? current.current : controls[0];
    controls.forEach((control) => (control.tabIndex = control === active ? 0 : -1));
    current.current = active ?? null;
  });

  const moveTo = (control: HTMLElement | undefined) => {
    if (!control) return;
    controlsIn(root.current).forEach((other) => (other.tabIndex = other === control ? 0 : -1));
    current.current = control;
    control.focus();
  };

  return {
    onFocus: (e: FocusEvent<HTMLElement>) => {
      if (e.target.closest('[role=menu]')) return;
      current.current = e.target;
    },
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => {
      const target = e.target as HTMLElement;
      if (target.closest('[role=menu]')) return;
      if (e.key === 'Escape') {
        onLeave();
        return;
      }
      // A text field needs these keys itself, which is why those fields come last.
      if (target instanceof HTMLInputElement && target.type !== 'checkbox') return;
      const controls = controlsIn(root.current);
      const rightToLeft = root.current ? getComputedStyle(root.current).direction === 'rtl' : false;
      const step = stepFor(e.key, controls.indexOf(target), controls.length, rightToLeft);
      if (step === undefined) return;
      e.preventDefault();
      moveTo(controls[step]);
    },
  };
}

/** The first control of a row that takes focus, for a key that moves into the row. */
export function firstControlOf(root: HTMLElement | null): HTMLElement | undefined {
  const controls = controlsIn(root);
  return controls.find((control) => control.tabIndex === 0) ?? controls[0];
}
