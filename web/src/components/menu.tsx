// SPDX-License-Identifier: Apache-2.0
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type AnchorHTMLAttributes,
  type ButtonHTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import { Link } from '../router';

const ENABLED_ITEMS = '[role^=menuitem]:not(:disabled)';
const FLOATING_GAP_PX = 4;

/** Places a list on the screen beside its button, flipped when it would not fit. */
function placeBeside(list: HTMLElement, button: HTMLElement) {
  const anchor = button.getBoundingClientRect();
  const fitsBelow = anchor.bottom + FLOATING_GAP_PX + list.offsetHeight <= window.innerHeight;
  const top = fitsBelow ? anchor.bottom + FLOATING_GAP_PX : anchor.top - FLOATING_GAP_PX - list.offsetHeight;
  const left = Math.min(anchor.left, window.innerWidth - list.offsetWidth - FLOATING_GAP_PX);
  list.style.top = `${Math.max(top, FLOATING_GAP_PX)}px`;
  list.style.left = `${Math.max(left, FLOATING_GAP_PX)}px`;
}

/** The item a key leads to: arrows, Home, End, or the next item that starts with a letter. */
function itemForKey(key: string, items: HTMLElement[], at: number): HTMLElement | undefined {
  if (key === 'ArrowDown') return items[(at + 1) % items.length];
  if (key === 'ArrowUp') return items[(at - 1 + items.length) % items.length];
  if (key === 'Home') return items[0];
  if (key === 'End') return items[items.length - 1];
  if (key.length !== 1) return undefined;
  const after = [...items.slice(at + 1), ...items.slice(0, at + 1)];
  return after.find((item) => item.textContent?.trim().toLowerCase().startsWith(key.toLowerCase()));
}

/** A dropdown menu with keyboard support: arrows, Home, End and letters move, Escape and Tab close. */
export function Menu({
  label,
  trigger,
  className,
  children,
  align = 'end',
  floating,
}: {
  label: string;
  trigger: ReactNode;
  className?: string;
  children: ReactNode;
  align?: 'start' | 'end';
  /** For a menu inside a scrolling area, which would otherwise be cut off. */
  floating?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);

  useLayoutEffect(() => {
    if (open && floating && list.current && button.current) placeBeside(list.current, button.current);
  }, [open, floating]);

  useEffect(() => {
    if (!open || !floating) return;
    // A fixed list would stay behind while its button scrolls away.
    const close = () => setOpen(false);
    window.addEventListener('scroll', close, { capture: true, passive: true });
    return () => window.removeEventListener('scroll', close, { capture: true });
  }, [open, floating]);

  useEffect(() => {
    if (!open) return;
    root.current?.querySelector<HTMLElement>(ENABLED_ITEMS)?.focus();
    const outside = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', outside);
    return () => document.removeEventListener('mousedown', outside);
  }, [open]);

  const keys = (e: KeyboardEvent) => {
    if (e.key === 'Tab') {
      setOpen(false);
      return;
    }
    if (e.key === 'Escape') {
      setOpen(false);
      button.current?.focus();
      return;
    }
    const items = Array.from(root.current?.querySelectorAll<HTMLElement>(ENABLED_ITEMS) ?? []);
    const target = itemForKey(e.key, items, items.indexOf(document.activeElement as HTMLElement));
    if (!target) return;
    e.preventDefault();
    target.focus();
  };

  return (
    <div className="menu" ref={root}>
      <button
        ref={button}
        type="button"
        className={className ?? 'icon'}
        aria-label={label}
        title={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {trigger}
      </button>
      {open && (
        <div
          ref={list}
          role="menu"
          aria-label={label}
          className={floating ? 'menu-list floating' : `menu-list ${align}`}
          onKeyDown={keys}
          onClick={() => setOpen(false)}
        >
          {children}
        </div>
      )}
    </div>
  );
}

export function MenuItem({ children, ...rest }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button type="button" role="menuitem" {...rest}>
      {children}
    </button>
  );
}

export function MenuLink({
  href,
  children,
  ...rest
}: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) {
  return (
    <Link href={href} role="menuitem" {...rest}>
      {children}
    </Link>
  );
}

/** A file the browser downloads, which the client-side router must leave alone. */
export function MenuDownload({
  href,
  children,
  ...rest
}: AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) {
  return (
    <a href={href} role="menuitem" {...rest}>
      {children}
    </a>
  );
}

export function MenuSeparator() {
  return <div role="separator" className="menu-separator" />;
}
