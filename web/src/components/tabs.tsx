// SPDX-License-Identifier: Apache-2.0
import { useRef, type KeyboardEvent } from 'react';

/** Tabs with arrow keys and one tab stop (WAI-ARIA tabs pattern). */
export function Tabs<T extends string>({
  label,
  panel,
  tabs,
  current,
  name,
  onChange,
}: {
  label: string;
  /** The id of the element that shows the content of the chosen tab. */
  panel: string;
  tabs: readonly T[];
  current: T;
  name: (tab: T) => string;
  onChange: (tab: T) => void;
}) {
  const list = useRef<HTMLDivElement>(null);
  const keys = (e: KeyboardEvent) => {
    const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const next = tabs[(tabs.indexOf(current) + step + tabs.length) % tabs.length];
    if (next === undefined) return;
    onChange(next);
    list.current?.querySelector<HTMLElement>(`#${panel}-tab-${next}`)?.focus();
  };
  return (
    <div ref={list} role="tablist" aria-label={label} className="tabs" onKeyDown={keys}>
      {tabs.map((tab) => (
        <button
          key={tab}
          id={`${panel}-tab-${tab}`}
          type="button"
          role="tab"
          aria-selected={current === tab}
          aria-controls={panel}
          tabIndex={current === tab ? 0 : -1}
          onClick={() => onChange(tab)}
        >
          {name(tab)}
        </button>
      ))}
    </div>
  );
}
