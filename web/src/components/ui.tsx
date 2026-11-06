// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError, type Space } from '../api';
import { useDates } from '../dates';
import { animalOf, animalPicture } from './animals';
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
  className,
  onPick,
}: {
  label: string;
  accept?: string;
  className?: string;
  onPick: (file: File) => void;
}) {
  return (
    <label className={`button file-picker ${className ?? ''}`}>
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

export function Avatar({ name, size }: { name: string; size?: 'small' | 'large' }) {
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

export function SpaceBadge({
  space,
  size,
}: {
  space: Pick<Space, 'key' | 'name' | 'icon'> & Partial<Pick<Space, 'id' | 'has_icon'>>;
  size?: 'small' | 'large';
}) {
  // A picture someone uploaded comes first.
  const animal = space.has_icon ? null : animalOf(space);
  // The name of the space is always beside it, so the picture needs no words of its own.
  if (animal)
    return <img className={`space-badge animal ${size ?? ''}`} src={animalPicture(animal)} alt="" />;
  const mark = firstCharacter(space.icon.trim()) || firstCharacter(space.name).toUpperCase();
  return (
    <span className={`space-badge tint-${tintIndex(space.key)} ${size ?? ''}`} aria-hidden="true">
      {space.has_icon ? (
        <img className="space-picture" src={`/api/v1/spaces/${space.id}/icon`} alt="" />
      ) : (
        mark
      )}
    </span>
  );
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
  const dates = useDates();
  return (
    <time dateTime={value} title={dates.moment(value)}>
      {dates.relative(value)}
    </time>
  );
}

/** A password a person chooses. They can see what they type, and paste always works (WCAG 3.3.8). */
export function NewPasswordField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  const [shown, setShown] = useState(false);
  return (
    <>
      <Field label={label} help={t('account.passwordHelp')}>
        <input
          type={shown ? 'text' : 'password'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          autoComplete="new-password"
          required
        />
      </Field>
      <label className="check">
        <input type="checkbox" checked={shown} onChange={(e) => setShown(e.target.checked)} />
        {t('account.showPassword')}
      </label>
    </>
  );
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}
