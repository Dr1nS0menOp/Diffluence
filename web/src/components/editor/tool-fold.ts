// SPDX-License-Identifier: Apache-2.0
// Tools that do not fit fold into More, from the end of the row (EDT-007, spec 027, D1).
import { useLayoutEffect, useRef, useState, type RefObject } from 'react';

/** How many groups fold from the end, so that the rest and the More button fit. */
export function foldedGroups(widths: number[], room: number, moreWidth: number): number {
  let used = widths.reduce((sum, width) => sum + width, 0);
  if (used <= room) return 0;
  const left = room - moreWidth;
  let folded = 0;
  for (let i = widths.length - 1; i >= 0 && used > left; i -= 1) {
    used -= widths[i] ?? 0;
    folded += 1;
  }
  return folded;
}

/** Room for a More button, which is not on the screen while it is not needed. */
const MORE_WIDTH_PX = 40;
const GROUP_SELECTOR = ':scope > .tool-group';

/**
 * Watches the row. A group that has folded is not on the screen, so its width is the one
 * it had when it last was: that tells when it fits again.
 */
export function useToolFolding(row: RefObject<HTMLElement | null>, groups: number) {
  const [folded, setFolded] = useState(0);
  const widths = useRef<number[]>([]);

  useLayoutEffect(() => {
    const element = row.current;
    if (!element) return;
    const measure = () => {
      const style = getComputedStyle(element);
      const gap = parseFloat(style.columnGap) || 0;
      const padding = (parseFloat(style.paddingInlineStart) || 0) + (parseFloat(style.paddingInlineEnd) || 0);
      Array.from(element.querySelectorAll<HTMLElement>(GROUP_SELECTOR)).forEach((group, i) => {
        widths.current[i] = group.getBoundingClientRect().width + gap;
      });
      // The tools of what is selected wrap below when they must, so the row itself stays put.
      const room = element.clientWidth - padding;
      setFolded(foldedGroups(widths.current.slice(0, groups), room, MORE_WIDTH_PX));
    };
    measure();
    const watcher = new ResizeObserver(measure);
    watcher.observe(element);
    return () => watcher.disconnect();
  }, [row, groups]);

  return folded;
}
