// SPDX-License-Identifier: Apache-2.0
// A table of the app in a box of its own, so a wide table scrolls there and never pushes
// the screen past its edge.
import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Holds one table of the app. While the table is wider than the room, the box scrolls
 * sideways and is a named Tab stop, so the keyboard can scroll it too (WCAG 2.1.1).
 */
export function TableBox({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const box = useRef<HTMLDivElement>(null);
  const [scrolls, setScrolls] = useState(false);
  useLayoutEffect(() => {
    const element = box.current;
    if (!element) return;
    const measure = () => setScrolls(element.scrollWidth > element.clientWidth);
    measure();
    // The box changes with the window, the table with its rows: either can make it scroll.
    const watcher = new ResizeObserver(measure);
    watcher.observe(element);
    if (element.firstElementChild) watcher.observe(element.firstElementChild);
    return () => watcher.disconnect();
  }, []);
  return (
    <div
      ref={box}
      className="table-box"
      role={scrolls ? 'region' : undefined}
      aria-label={scrolls ? t('common.tableScrolls') : undefined}
      tabIndex={scrolls ? 0 : undefined}
    >
      {children}
    </div>
  );
}
