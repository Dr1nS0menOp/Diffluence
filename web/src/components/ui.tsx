// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api';

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
      <header className="row">
        <h2>{title}</h2>
        <button type="button" className="ghost" onClick={onClose}>
          {t('common.close')}
        </button>
      </header>
      {children}
    </dialog>
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

export function formatDate(value: string | null | undefined): string {
  if (!value) return '';
  return new Date(value).toLocaleString();
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}
