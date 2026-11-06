// SPDX-License-Identifier: Apache-2.0
// PdfCraft and GridCraft, in a sandboxed frame that has no origin of its own. They
// paint on a canvas, which a screen reader cannot read, so the file always stays a
// download as well. The frame cannot fetch the file: this page hands it over.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Dialog, ErrorNote, Loading } from './ui';

export type Craft = 'pdfcraft' | 'gridcraft';

export interface CraftFile {
  name: string;
  type: string;
  load: () => Promise<ArrayBuffer>;
}

const SPREADSHEET_FILE = /\.(xlsx|csv)$/i;

/** The viewer of a file, if it has one. */
export function craftOf(filename: string, contentType: string): Craft | null {
  if (contentType === 'application/pdf' || filename.toLowerCase().endsWith('.pdf')) return 'pdfcraft';
  return SPREADSHEET_FILE.test(filename) ? 'gridcraft' : null;
}

async function loadFile(address: string): Promise<ArrayBuffer> {
  const response = await fetch(`${address}?download=1`);
  if (!response.ok) throw new Error(response.statusText);
  return response.arrayBuffer();
}

function signalOf(data: unknown): string | undefined {
  if (typeof data !== 'object' || data === null || !('craft' in data)) return undefined;
  return typeof data.craft === 'string' ? data.craft : undefined;
}

export function CraftDialog({
  craft,
  file,
  download,
  onClose,
}: {
  craft: Craft;
  file: CraftFile;
  download?: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const frame = useRef<HTMLIFrameElement>(null);
  const [state, setState] = useState<'starting' | 'open'>('starting');
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    const send = async (target: Window) => {
      try {
        const bytes = await file.load();
        target.postMessage({ bytes, name: file.name, type: file.type }, '*', [bytes]);
      } catch (e) {
        setError(e);
      }
    };
    const listen = (event: MessageEvent) => {
      const target = frame.current?.contentWindow;
      if (!target || event.source !== target) return;
      const signal = signalOf(event.data);
      if (signal === 'ready') void send(target);
      if (signal === 'opened') setState('open');
      // A canvas keeps the keyboard, so Escape has to come back from the frame.
      if (signal === 'escape') onClose();
    };
    window.addEventListener('message', listen);
    return () => window.removeEventListener('message', listen);
  }, [file, onClose]);

  return (
    <Dialog title={file.name} onClose={onClose} wide>
      <ErrorNote error={error} />
      {state === 'starting' && !error && <Loading />}
      <p className="muted">
        {t('craft.note')} {download && <a href={download}>{t('craft.download')}</a>}
      </p>
      <iframe
        ref={frame}
        className="craft-frame"
        title={t('craft.frame', { name: file.name })}
        src={`/vendor/${craft}/host.html`}
        sandbox="allow-scripts allow-downloads"
      />
    </Dialog>
  );
}

/** Opens a stored file in its viewer, for a file that has one. */
export function OpenInCraft({
  file,
  address,
}: {
  file: { filename: string; content_type?: string };
  address: string;
}) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const close = useCallback(() => setIsOpen(false), []);
  const type = file.content_type ?? '';
  const target = useMemo(
    () => ({ name: file.filename, type, load: () => loadFile(address) }),
    [file.filename, type, address],
  );
  const craft = craftOf(file.filename, type);
  if (!craft) return null;
  return (
    <>
      <button type="button" className="ghost" onClick={() => setIsOpen(true)}>
        {t(`craft.open.${craft}`)}
      </button>
      {isOpen && (
        <CraftDialog craft={craft} file={target} download={`${address}?download=1`} onClose={close} />
      )}
    </>
  );
}
