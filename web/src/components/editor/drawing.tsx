// SPDX-License-Identifier: Apache-2.0
// Drawing in the optional draw.io service, on an address the administrator set
// (ADR-026). It runs on another origin, so the frame can never reach our session
// (SEC-521), and every message is checked on window and origin (SEC-523). The result
// is a PNG with the diagram inside it, stored as a normal attachment.
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Attachment, type Me } from '../../api';
import { Dialog, ErrorNote } from '../ui';

const EMPTY_DRAWING = '<mxfile/>';
const PNG_PREFIX = 'data:image/png;base64,';

interface DrawingMessage {
  event: string;
  data?: unknown;
}

/** The parts of a MessageEvent that decide whether we listen to it. */
interface IncomingMessage {
  source: unknown;
  origin: string;
  data: unknown;
}

/** The address of embed mode. Offline and stealth keep it from calling anyone else. */
export function drawingAddress(origin: string): string {
  const address = new URL(origin);
  address.search = new URLSearchParams({ embed: '1', proto: 'json', offline: '1', stealth: '1' }).toString();
  return address.toString();
}

function parsed(data: unknown): unknown {
  if (typeof data !== 'string') return null;
  try {
    return JSON.parse(data) as unknown;
  } catch {
    // Other scripts post messages too. What is not JSON is not meant for us.
    return null;
  }
}

/** Gives the message only if it came from our frame, on the origin we set. */
export function drawingMessage(
  event: IncomingMessage,
  frame: Window | null | undefined,
  origin: string,
): DrawingMessage | null {
  if (!frame || event.source !== frame || event.origin !== origin) return null;
  const message = parsed(event.data);
  if (typeof message !== 'object' || message === null) return null;
  return typeof (message as DrawingMessage).event === 'string' ? (message as DrawingMessage) : null;
}

/** Turns the export into a file. Only a PNG gets through. */
export function pngFromDataAddress(address: unknown): Blob | null {
  if (typeof address !== 'string' || !address.startsWith(PNG_PREFIX)) return null;
  try {
    const bytes = Uint8Array.from(atob(address.slice(PNG_PREFIX.length)), (c) => c.charCodeAt(0));
    return new Blob([bytes], { type: 'image/png' });
  } catch {
    // The service sent something that is not base64: the caller says it failed.
    return null;
  }
}

function drawingFilename(now: Date): string {
  return `drawing-${now
    .toISOString()
    .replace(/[^0-9]/g, '')
    .slice(0, 14)}.png`;
}

function DrawingFrame({
  origin,
  pageId,
  onDone,
  onClose,
}: {
  origin: string;
  pageId: string;
  onDone: (attachment: Attachment) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const frame = useRef<HTMLIFrameElement>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    const send = (message: unknown) =>
      frame.current?.contentWindow?.postMessage(JSON.stringify(message), origin);
    const save = async (address: unknown) => {
      const png = pngFromDataAddress(address);
      if (!png) {
        setError(new Error(t('drawing.failed')));
        return;
      }
      const name = encodeURIComponent(drawingFilename(new Date()));
      try {
        onDone(
          await api.upload<Attachment>(
            `/pages/${pageId}/attachments?filename=${name}`,
            new File([png], name),
          ),
        );
      } catch (e) {
        setError(e);
      }
    };
    const listen = (event: MessageEvent) => {
      const message = drawingMessage(event, frame.current?.contentWindow, origin);
      if (message?.event === 'init') send({ action: 'load', xml: EMPTY_DRAWING, autosave: 0 });
      if (message?.event === 'save') send({ action: 'export', format: 'xmlpng' });
      if (message?.event === 'export') void save(message.data);
      if (message?.event === 'exit') onClose();
    };
    window.addEventListener('message', listen);
    return () => window.removeEventListener('message', listen);
  }, [origin, pageId, onDone, onClose, t]);
  return (
    <>
      <ErrorNote error={error} />
      <iframe
        ref={frame}
        className="drawing-frame"
        title={t('drawing.title')}
        src={drawingAddress(origin)}
        sandbox="allow-scripts allow-same-origin"
        referrerPolicy="no-referrer"
      />
    </>
  );
}

export function DrawingDialog({
  pageId,
  onDone,
  onClose,
}: {
  pageId: string;
  onDone: (attachment: Attachment) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const origin = me.data?.drawing_origin;
  return (
    <Dialog title={t('drawing.title')} onClose={onClose}>
      <ErrorNote error={me.error} />
      {me.data && !origin && <p className="muted">{t('drawing.off')}</p>}
      {origin && <DrawingFrame origin={origin} pageId={pageId} onDone={onDone} onClose={onClose} />}
    </Dialog>
  );
}
