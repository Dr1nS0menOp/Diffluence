// SPDX-License-Identifier: Apache-2.0
// Two ways to see a page full size: one image, and the whole page as slides.
import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { Json } from '../api';
import { DocumentView } from './editor/editor';
import { slidesOf } from './editor/outline';
import { IconChevronLeft, IconChevronRight, IconClose } from './icons';

export interface ShownImage {
  src: string;
  alt: string;
  caption: string;
}

/** On top of the page, with arrows when there is more than one image. */
export function Lightbox({
  images,
  start,
  onClose,
}: {
  images: ShownImage[];
  start: number;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [at, setAt] = useState(start);
  const dialog = useRef<HTMLDialogElement>(null);
  const image = images[at];
  const many = images.length > 1;
  const move = (step: number) => setAt((at + step + images.length) % images.length);

  useEffect(() => {
    if (dialog.current && !dialog.current.open) dialog.current.showModal();
  }, []);

  const keys = (e: KeyboardEvent) => {
    if (!many) return;
    if (e.key === 'ArrowRight') move(1);
    if (e.key === 'ArrowLeft') move(-1);
  };
  if (!image) return null;
  return (
    <dialog
      ref={dialog}
      className="lightbox"
      aria-label={image.alt || t('viewer.image')}
      onClose={onClose}
      onKeyDown={keys}
    >
      <button type="button" className="icon lightbox-close" aria-label={t('common.close')} onClick={onClose}>
        <IconClose />
      </button>
      <figure>
        <img src={image.src} alt={image.alt} />
        {image.caption && <figcaption>{image.caption}</figcaption>}
      </figure>
      {many && (
        <div className="lightbox-steps">
          <button type="button" className="icon" aria-label={t('viewer.previous')} onClick={() => move(-1)}>
            <IconChevronLeft />
          </button>
          <span role="status">{t('viewer.position', { at: at + 1, total: images.length })}</span>
          <button type="button" className="icon" aria-label={t('viewer.next')} onClick={() => move(1)}>
            <IconChevronRight />
          </button>
        </div>
      )}
    </dialog>
  );
}

/** The page as slides. Full screen is a bonus: it works without it (spec 009, B10). */
export function Presentation({
  title,
  document: content,
  onClose,
}: {
  title: string;
  document: Json;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const slides = useMemo(() => slidesOf(content), [content]);
  const [at, setAt] = useState(0);
  const dialog = useRef<HTMLDialogElement>(null);
  const last = slides.length - 1;
  const slide = slides[at];

  useEffect(() => {
    const box = dialog.current;
    if (!box) return;
    if (!box.open) box.showModal();
    // A browser may refuse full screen, and then the dialog is simply big.
    box.requestFullscreen?.().catch(() => undefined);
    return () => {
      if (window.document.fullscreenElement) void window.document.exitFullscreen();
    };
  }, []);

  const keys = (e: KeyboardEvent) => {
    const steps: Record<string, number> = {
      ArrowRight: Math.min(at + 1, last),
      ' ': Math.min(at + 1, last),
      PageDown: Math.min(at + 1, last),
      ArrowLeft: Math.max(at - 1, 0),
      PageUp: Math.max(at - 1, 0),
      Home: 0,
      End: last,
    };
    const next = steps[e.key];
    if (next === undefined) return;
    e.preventDefault();
    setAt(next);
  };
  return (
    <dialog ref={dialog} className="presentation" aria-label={title} onClose={onClose} onKeyDown={keys}>
      <button type="button" className="icon lightbox-close" aria-label={t('common.close')} onClick={onClose}>
        <IconClose />
      </button>
      <div className="presentation-slide">{slide && <DocumentView key={at} document={slide} plain />}</div>
      <div className="lightbox-steps">
        <button
          type="button"
          className="icon"
          aria-label={t('viewer.previous')}
          disabled={at === 0}
          onClick={() => setAt(at - 1)}
        >
          <IconChevronLeft />
        </button>
        <span role="status">{t('viewer.position', { at: at + 1, total: slides.length })}</span>
        <button
          type="button"
          className="icon"
          aria-label={t('viewer.next')}
          disabled={at === last}
          onClick={() => setAt(at + 1)}
        >
          <IconChevronRight />
        </button>
      </div>
    </dialog>
  );
}
