// SPDX-License-Identifier: Apache-2.0
// Cuts a square from a picture. Dragging has a way by keyboard and by buttons
// (WCAG 2.5.7). A canvas needs no change to the Content Security Policy (spec 013, D7).
import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type WheelEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { moveCrop, startCrop, zoomCrop, type Crop } from '../crop';
import { Dialog, ErrorNote } from './ui';

const CANVAS_SIZE = 256;

const ARROW_STEPS: Record<string, { x: number; y: number }> = {
  ArrowLeft: { x: -1, y: 0 },
  ArrowRight: { x: 1, y: 0 },
  ArrowUp: { x: 0, y: -1 },
  ArrowDown: { x: 0, y: 1 },
};

function useBitmap(file: File) {
  const [bitmap, setBitmap] = useState<ImageBitmap | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    let current = true;
    createImageBitmap(file).then(
      (image) => {
        if (current) setBitmap(image);
      },
      (e: unknown) => {
        if (current) setError(e);
      },
    );
    return () => {
      current = false;
    };
  }, [file]);
  return { bitmap, error };
}

function CropCanvas({
  bitmap,
  crop,
  onChange,
}: {
  bitmap: ImageBitmap;
  crop: Crop;
  onChange: (next: Crop) => void;
}) {
  const { t } = useTranslation();
  const canvas = useRef<HTMLCanvasElement>(null);
  const dragFrom = useRef<{ x: number; y: number } | null>(null);
  const { width, height } = bitmap;
  useEffect(() => {
    canvas.current
      ?.getContext('2d')
      ?.drawImage(bitmap, crop.x, crop.y, crop.size, crop.size, 0, 0, CANVAS_SIZE, CANVAS_SIZE);
  }, [bitmap, crop]);
  const keys = (e: KeyboardEvent) => {
    const steps = ARROW_STEPS[e.key];
    if (steps) onChange(moveCrop(crop, steps, width, height));
    else if (e.key === '+' || e.key === '=') onChange(zoomCrop(crop, 1, width, height));
    else if (e.key === '-') onChange(zoomCrop(crop, -1, width, height));
    else return;
    e.preventDefault();
  };
  const drag = (e: PointerEvent) => {
    if (!dragFrom.current) return;
    const scale = crop.size / CANVAS_SIZE;
    const x = crop.x + (dragFrom.current.x - e.clientX) * scale;
    const y = crop.y + (dragFrom.current.y - e.clientY) * scale;
    dragFrom.current = { x: e.clientX, y: e.clientY };
    onChange(moveCrop({ ...crop, x: Math.round(x), y: Math.round(y) }, { x: 0, y: 0 }, width, height));
  };
  const wheel = (e: WheelEvent) => onChange(zoomCrop(crop, e.deltaY < 0 ? 1 : -1, width, height));
  return (
    <canvas
      ref={canvas}
      className="crop-canvas"
      width={CANVAS_SIZE}
      height={CANVAS_SIZE}
      tabIndex={0}
      role="application"
      aria-label={t('account.cropHelp')}
      onKeyDown={keys}
      onWheel={wheel}
      onPointerDown={(e) => {
        dragFrom.current = { x: e.clientX, y: e.clientY };
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={drag}
      onPointerUp={() => {
        dragFrom.current = null;
      }}
    />
  );
}

export function CropDialog({
  file,
  busy,
  onSave,
  onClose,
}: {
  file: File;
  busy: boolean;
  onSave: (crop: Crop) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { bitmap, error } = useBitmap(file);
  const [moved, setMoved] = useState<Crop | null>(null);
  const crop = moved ?? (bitmap ? startCrop(bitmap.width, bitmap.height) : null);
  const zoom = (step: number) => {
    if (bitmap && crop) setMoved(zoomCrop(crop, step, bitmap.width, bitmap.height));
  };
  const move = (x: number, y: number) => {
    if (bitmap && crop) setMoved(moveCrop(crop, { x, y }, bitmap.width, bitmap.height));
  };
  return (
    <Dialog title={t('account.cropTitle')} onClose={onClose}>
      <ErrorNote error={error ? new Error(t('account.pictureUnreadable')) : null} />
      {bitmap && crop && (
        <>
          <p className="muted">{t('account.cropHelp')}</p>
          <CropCanvas bitmap={bitmap} crop={crop} onChange={setMoved} />
          <div className="row">
            <button type="button" onClick={() => zoom(1)}>
              {t('account.zoomIn')}
            </button>
            <button type="button" onClick={() => zoom(-1)}>
              {t('account.zoomOut')}
            </button>
            <button type="button" onClick={() => move(-1, 0)}>
              {t('account.moveLeft')}
            </button>
            <button type="button" onClick={() => move(1, 0)}>
              {t('account.moveRight')}
            </button>
            <button type="button" onClick={() => move(0, -1)}>
              {t('account.moveUp')}
            </button>
            <button type="button" onClick={() => move(0, 1)}>
              {t('account.moveDown')}
            </button>
          </div>
          <div className="dialog-actions">
            <button type="button" className="primary" disabled={busy} onClick={() => onSave(crop)}>
              {t('common.save')}
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}
