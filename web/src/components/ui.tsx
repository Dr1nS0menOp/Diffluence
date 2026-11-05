// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError, type Space } from '../api';
import { IconClose, IconUpload } from './icons';

const TINT_COUNT = 4;

export function Loading() {
  const { t } = useTranslation();
  return (
    <p role="status" className="muted">
      {t('app.loading')}
    </p>
  );
}

export function ErrorNote({ error }: { error: unknown }) {
  const { t } = useTranslation();
  if (!error) return null;
  const notFound = error instanceof ApiError && error.status === 404;
  const text = notFound ? t('app.notFound') : error instanceof Error ? error.message : t('app.error');
  return (
    <p role="alert" className="note note-error">
      {text}
    </p>
  );
}

/** A modal built on the native dialog element: focus trap and Escape come for free. */
export function Dialog({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (d && !d.open) d.showModal();
  }, []);
  return (
    <dialog ref={ref} onClose={onClose} aria-label={title} className={wide ? 'wide' : undefined}>
      <header className="dialog-head">
        <h2>{title}</h2>
        <button type="button" className="icon" aria-label={t('common.close')} onClick={onClose}>
          <IconClose />
        </button>
      </header>
      {children}
    </dialog>
  );
}

/** Asks before something that is hard to undo. The browser's own dialog does not fit the design. */
export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  tone = 'danger',
  onConfirm,
  onClose,
}: {
  title: string;
  message: string;
  confirmLabel: string;
  tone?: 'danger' | 'primary';
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog title={title} onClose={onClose}>
      <p>{message}</p>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button
          type="button"
          className={tone}
          onClick={() => {
            onClose();
            onConfirm();
          }}
        >
          {confirmLabel}
        </button>
      </div>
    </Dialog>
  );
}

/** A button that asks first. */
export function ConfirmButton({
  label,
  message,
  className,
  tone,
  onConfirm,
}: {
  label: string;
  message: string;
  className?: string;
  tone?: 'danger' | 'primary';
  onConfirm: () => void;
}) {
  const [asking, setAsking] = useState(false);
  return (
    <>
      <button type="button" className={className} onClick={() => setAsking(true)}>
        {label}
      </button>
      {asking && (
        <ConfirmDialog
          title={label}
          message={message}
          confirmLabel={label}
          tone={tone}
          onConfirm={onConfirm}
          onClose={() => setAsking(false)}
        />
      )}
    </>
  );
}

export function Field({ label, help, children }: { label: string; help?: string; children: ReactNode }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {help && <small className="muted">{help}</small>}
    </label>
  );
}

/** A file input that looks like a button, and still works from the keyboard. */
export function FilePicker({
  label,
  accept,
  onPick,
}: {
  label: string;
  accept?: string;
  onPick: (file: File) => void;
}) {
  return (
    <label className="button file-picker">
      <IconUpload size={16} /> {label}
      <input
        type="file"
        className="visually-hidden"
        accept={accept}
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) onPick(file);
          e.target.value = '';
        }}
      />
    </label>
  );
}

/** The same text always gets the same colour. */
export function tintIndex(text: string): number {
  return Array.from(text).reduce((sum, c) => sum + (c.codePointAt(0) ?? 0), 0) % TINT_COUNT;
}

export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  const firstLetter = (word: string | undefined) => Array.from(word ?? '')[0] ?? '';
  const last = words.length > 1 ? firstLetter(words[words.length - 1]) : '';
  return (firstLetter(words[0]) + last).toUpperCase() || '?';
}

function firstCharacter(text: string): string {
  const [first] = new Intl.Segmenter().segment(text);
  return first?.segment ?? '';
}

export function Avatar({ name, size }: { name: string; size?: 'small' }) {
  return (
    // From the style sheet: these letters are decoration, and the name of a button holding
    // this picture would otherwise have to contain them (WCAG 2.5.3).
    <span
      className={`avatar tint-${tintIndex(name)} ${size ?? ''}`}
      aria-hidden="true"
      data-initials={initials(name)}
    />
  );
}

export function SpaceBadge({ space, size }: { space: Space; size?: 'small' | 'large' }) {
  const mark = firstCharacter(space.icon.trim()) || firstCharacter(space.name).toUpperCase();
  return (
    <span className={`space-badge tint-${tintIndex(space.key)} ${size ?? ''}`} aria-hidden="true">
      {mark}
    </span>
  );
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return '';
  return new Date(value).toLocaleString();
}

export function formatDay(value: string): string {
  return new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const RELATIVE_DAYS = 30;

/** "3 hours ago" in the person's language. Older than a month is a date. */
export function relativeTime(value: string, now = Date.now(), locale?: string): string {
  const past = now - new Date(value).getTime();
  if (past >= RELATIVE_DAYS * DAY) return formatDay(value);
  const format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  if (past >= DAY) return format.format(-Math.floor(past / DAY), 'day');
  if (past >= HOUR) return format.format(-Math.floor(past / HOUR), 'hour');
  if (past >= MINUTE) return format.format(-Math.floor(past / MINUTE), 'minute');
  return format.format(0, 'second');
}

/** A time that reads easily, with the exact moment on hover and for screen readers. */
/** Tabs with the keys of the ARIA tabs pattern: the arrows move, and the focus moves along. */
export function Tabs<Name extends string>({
  names,
  current,
  label,
  nameOf,
  panel,
  onChange,
}: {
  names: readonly Name[];
  current: Name;
  label: string;
  nameOf: (name: Name) => string;
  panel?: string;
  onChange: (next: Name) => void;
}) {
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    const forward = getComputedStyle(e.currentTarget).direction === 'rtl' ? 'ArrowLeft' : 'ArrowRight';
    const backward = forward === 'ArrowLeft' ? 'ArrowRight' : 'ArrowLeft';
    const step = e.key === forward ? 1 : e.key === backward ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const at = (names.indexOf(current) + step + names.length) % names.length;
    const next = names[at];
    if (!next) return;
    onChange(next);
    e.currentTarget.querySelectorAll<HTMLElement>('[role="tab"]')[at]?.focus();
  };
  return (
    <div role="tablist" aria-label={label} className="tabs" onKeyDown={keys}>
      {names.map((name) => (
        <button
          key={name}
          type="button"
          role="tab"
          aria-selected={current === name}
          aria-controls={panel}
          tabIndex={current === name ? 0 : -1}
          onClick={() => onChange(name)}
        >
          {nameOf(name)}
        </button>
      ))}
    </div>
  );
}

export function RelativeTime({ value }: { value: string }) {
  return (
    <time dateTime={value} title={formatDate(value)}>
      {relativeTime(value)}
    </time>
  );
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}
