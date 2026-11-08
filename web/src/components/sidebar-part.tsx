// SPDX-License-Identifier: Apache-2.0
// A part of the sidebar that folds shut and, for the links of a space, can be made
// taller or shorter. Both follow the person to other devices (spec 033).
import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePreferences } from '../preferences';
import { IconChevronDown } from './icons';

export type SidebarPartName = 'space' | 'you' | 'spaces';

const MIN_HEIGHT_REM = 4;
const MAX_HEIGHT_REM = 40;
const CHEVRON_SIZE = 14;

const remInPixels = () => parseFloat(getComputedStyle(document.documentElement).fontSize);
const clampHeight = (rem: number) => Math.min(MAX_HEIGHT_REM, Math.max(MIN_HEIGHT_REM, Math.round(rem)));

/** Whether a part is folded shut, and the way to change it. */
export function useSidebarFold(name: SidebarPartName): [boolean, () => void] {
  const { preferences, update } = usePreferences();
  const shut = preferences.sidebar_shut ?? [];
  const isShut = shut.includes(name);
  const toggle = () =>
    update({ sidebar_shut: isShut ? shut.filter((part) => part !== name) : [...shut, name] });
  return [isShut, toggle];
}

/** The heading of a part: it folds the part, and may hold tools at its far end. */
export function SidebarPart({
  name,
  title,
  tools,
  resizable,
  children,
}: {
  name: SidebarPartName;
  title: string;
  tools?: ReactNode;
  /** The part keeps a height the person dragged it to, and scrolls inside. */
  resizable?: boolean;
  children: ReactNode;
}) {
  const [shut, toggle] = useSidebarFold(name);
  const bodyId = `sidebar-part-${name}`;
  return (
    <section className="sidebar-part" aria-label={title}>
      <div className="sidebar-part-head">
        <button
          type="button"
          className="sidebar-part-toggle"
          aria-expanded={!shut}
          aria-controls={bodyId}
          onClick={toggle}
        >
          <span className={shut ? 'part-chevron shut' : 'part-chevron'}>
            <IconChevronDown size={CHEVRON_SIZE} />
          </span>
          {title}
        </button>
        {!shut && tools && <span className="sidebar-tools">{tools}</span>}
      </div>
      {!shut &&
        (resizable ? <SizedBody id={bodyId}>{children}</SizedBody> : <div id={bodyId}>{children}</div>)}
    </section>
  );
}

/** A body with a handle below it. Dragging has a keyboard way as well (WCAG 2.5.7). */
function SizedBody({ id, children }: { id: string; children: ReactNode }) {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const stored = preferences.sidebar_links_height;
  const [height, setHeight] = useState(stored);
  const [seen, setSeen] = useState(stored);
  if (seen !== stored) {
    setSeen(stored);
    setHeight(stored);
  }
  const body = useRef<HTMLDivElement>(null);
  useEffect(() => {
    body.current?.style.setProperty('--part-height', `${height}rem`);
  }, [height]);
  const start = useRef<{ y: number; rem: number } | null>(null);
  const measured = () =>
    body.current ? body.current.getBoundingClientRect().height / remInPixels() : MIN_HEIGHT_REM;
  const heightAt = (e: PointerEvent<HTMLDivElement>) =>
    start.current ? clampHeight(start.current.rem + (e.clientY - start.current.y) / remInPixels()) : height;
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
    e.preventDefault();
    const next = clampHeight((height || measured()) + (e.key === 'ArrowDown' ? 1 : -1));
    setHeight(next);
    update({ sidebar_links_height: next });
  };
  return (
    <>
      <div id={id} ref={body} className="sidebar-part-body" data-sized={height ? '' : undefined}>
        {children}
      </div>
      <div
        role="separator"
        tabIndex={0}
        className="sidebar-part-handle"
        aria-orientation="horizontal"
        aria-label={t('space.partHeight')}
        aria-valuemin={MIN_HEIGHT_REM}
        aria-valuemax={MAX_HEIGHT_REM}
        aria-valuenow={Math.round(height || measured())}
        onKeyDown={keys}
        onDoubleClick={() => {
          setHeight(0);
          update({ sidebar_links_height: 0 });
        }}
        onPointerDown={(e) => {
          start.current = { y: e.clientY, rem: height || measured() };
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          if (start.current) setHeight(heightAt(e));
        }}
        onPointerUp={(e) => {
          if (!start.current) return;
          const next = heightAt(e);
          start.current = null;
          update({ sidebar_links_height: next });
        }}
      />
    </>
  );
}
