// SPDX-License-Identifier: Apache-2.0
// Drawing in the optional draw.io service, on an address the administrator set
// (ADR-026). It runs on another origin, so the frame can never reach our session
// (SEC-521), and every message is checked on window and origin (SEC-523). The result
// is a PNG with the diagram inside it, stored as a normal attachment.
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Attachment, type Me } from '../../api';
import { fromFrame, listenToFrame, postToFrame, type IncomingMessage } from '../frame-messages';
import { Dialog, ErrorNote } from '../ui';

const EMPTY_DRAWING = '<mxfile/>';
const DRAWING_PREFIX = 'drawing-';
const PNG_PREFIX = 'data:image/png;base64,';

/** A saved drawing. The PNG holds the diagram, which is what makes it editable. */
export interface DrawingFile {
  id: string;
  filename: string;
}

export const isDrawing = (filename: string) =>
  filename.startsWith(DRAWING_PREFIX) && filename.endsWith('.png');

/** What the service gets when it is ready: an empty drawing, or the stored one. */
export function loadMessage(storedPng: string | null) {
  return storedPng
    ? { action: 'load', xmlpng: storedPng, autosave: 0 }
    : { action: 'load', xml: EMPTY_DRAWING, autosave: 0 };
}

async function storedPng(file: DrawingFile | undefined): Promise<string | null> {
  if (!file) return null;
  const answer = await fetch(`/api/v1/attachments/${file.id}/content`);
  if (!answer.ok) throw new Error(answer.statusText);
  const bytes = await answer.blob();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, PNG_PREFIX));
    reader.onerror = () => reject(new Error('unreadable'));
    reader.readAsDataURL(bytes);
  });
}

/** The address stays the same for a new version, so the images load it again. */
export function showNewVersion(root: HTMLElement, id: string) {
  root.querySelectorAll<HTMLImageElement>(`img[src*="${id}"]`).forEach((image) => {
    const address = image.src;
    image.src = address;
  });
}

interface DrawingMessage {
  event: string;
  data?: unknown;
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
  return fromFrame(event, frame, origin) ? drawingMessageOf(event.data) : null;
}

/** What the service said, once the frame module has checked where it came from. */
function drawingMessageOf(data: unknown): DrawingMessage | null {
  const message = parsed(data);
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
  return `${DRAWING_PREFIX}${now
    .toISOString()
    .replace(/[^0-9]/g, '')
    .slice(0, 14)}.png`;
}

function DrawingFrame({
  origin,
  pageId,
  existing,
  onDone,
  onClose,
}: {
  origin: string;
  pageId: string;
  existing?: DrawingFile;
  onDone: (attachment: Attachment) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const frame = useRef<HTMLIFrameElement>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    const send = (message: unknown) =>
      postToFrame(frame.current?.contentWindow, origin, JSON.stringify(message));
    const save = async (address: unknown) => {
      const png = pngFromDataAddress(address);
      if (!png) {
        setError(new Error(t('drawing.failed')));
        return;
      }
      // The same name makes a new version of the same file, so the page keeps showing it.
      const name = encodeURIComponent(existing?.filename ?? drawingFilename(new Date()));
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
    return listenToFrame(
      () => frame.current?.contentWindow,
      origin,
      (data) => {
        const message = drawingMessageOf(data);
        if (message?.event === 'init') {
          storedPng(existing).then((png) => send(loadMessage(png)), setError);
        }
        if (message?.event === 'save') send({ action: 'export', format: 'xmlpng' });
        if (message?.event === 'export') void save(message.data);
        if (message?.event === 'exit') onClose();
      },
    );
  }, [origin, pageId, existing, onDone, onClose, t]);
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
  existing,
  onDone,
  onClose,
}: {
  pageId: string;
  existing?: DrawingFile;
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
      {origin && (
        <DrawingFrame origin={origin} pageId={pageId} existing={existing} onDone={onDone} onClose={onClose} />
      )}
    </Dialog>
  );
}
