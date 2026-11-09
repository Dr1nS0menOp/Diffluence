// SPDX-License-Identifier: Apache-2.0
// PdfCraft and GridCraft, in a sandboxed frame that has no origin of its own. They
// paint on a canvas, which a screen reader cannot read, so the file always stays a
// download as well. The frame cannot fetch the file: this page hands it over. A PDF
// changed in the viewer comes back here, and is stored through the normal upload as a
// new version, with every check of an upload (ARC-099). The frame itself writes nothing.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api, type Attachment } from '../api';
import { listenToFrame, OPAQUE_ORIGIN, postToFrame } from './frame-messages';
import { Dialog, ErrorNote, Loading } from './ui';

export type Craft = 'pdfcraft' | 'gridcraft';

export interface CraftFile {
  name: string;
  type: string;
  load: () => Promise<ArrayBuffer>;
}

const SPREADSHEET_FILE = /\.(xlsx|csv)$/i;
const PDF_START = '%PDF-';

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

/** What the frame says, or nothing when the message is not one of ours. */
export function signalOf(data: unknown): string | undefined {
  if (typeof data !== 'object' || data === null || !('craft' in data)) return undefined;
  return typeof data.craft === 'string' ? data.craft : undefined;
}

/** The bytes of a PDF the viewer saved, or null when they are not a PDF at all. */
export function savedPDF(data: unknown): ArrayBuffer | null {
  if (typeof data !== 'object' || data === null || !('bytes' in data)) return null;
  const bytes = data.bytes;
  if (!(bytes instanceof ArrayBuffer) || bytes.byteLength < PDF_START.length) return null;
  const start = new TextDecoder().decode(new Uint8Array(bytes, 0, PDF_START.length));
  return start === PDF_START ? bytes : null;
}

type SaveState =
  { kind: 'none' } | { kind: 'changed'; bytes: ArrayBuffer } | { kind: 'saving' } | { kind: 'saved' };

export function CraftDialog({
  craft,
  file,
  download,
  onSaveVersion,
  onClose,
}: {
  craft: Craft;
  file: CraftFile;
  download?: string;
  /** Given only to someone who may store a new version of this file. */
  onSaveVersion?: (bytes: ArrayBuffer) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const frame = useRef<HTMLIFrameElement>(null);
  const [state, setState] = useState<'starting' | 'open'>('starting');
  const [save, setSave] = useState<SaveState>({ kind: 'none' });
  const [error, setError] = useState<unknown>(null);
  const mayKeep = craft === 'pdfcraft' && !!onSaveVersion;

  useEffect(() => {
    const target = () => frame.current?.contentWindow;
    const send = async () => {
      try {
        const bytes = await file.load();
        postToFrame(target(), OPAQUE_ORIGIN, { bytes, name: file.name, type: file.type }, [bytes]);
      } catch (e) {
        setError(e);
      }
    };
    return listenToFrame(target, OPAQUE_ORIGIN, (data) => {
      const signal = signalOf(data);
      if (signal === 'ready') void send();
      if (signal === 'opened') setState('open');
      // A canvas keeps the keyboard, so Escape has to come back from the frame.
      if (signal === 'escape') onClose();
      if (signal === 'saved' && mayKeep) {
        const bytes = savedPDF(data);
        if (bytes) setSave({ kind: 'changed', bytes });
        else setError(new Error(t('craft.notPdf')));
      }
    });
  }, [file, onClose, mayKeep, t]);

  const keep = async () => {
    if (save.kind !== 'changed' || !onSaveVersion) return;
    const { bytes } = save;
    setSave({ kind: 'saving' });
    try {
      await onSaveVersion(bytes);
      setSave({ kind: 'saved' });
    } catch (e) {
      setError(e);
      setSave({ kind: 'changed', bytes });
    }
  };

  return (
    <Dialog title={file.name} onClose={onClose} full>
      <ErrorNote error={error} />
      {state === 'starting' && !error && <Loading />}
      <p className="muted craft-note">
        {t('craft.note')} {download && <a href={download}>{t('craft.download')}</a>}
      </p>
      {save.kind !== 'none' && (
        <p className="craft-note" role="status">
          {save.kind === 'saved' ? (
            t('craft.saved')
          ) : (
            <>
              {t('craft.changed')}{' '}
              <button type="button" onClick={() => void keep()} disabled={save.kind === 'saving'}>
                {t('craft.saveVersion')}
              </button>
            </>
          )}
        </p>
      )}
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
  versionOf,
}: {
  file: { filename: string; content_type?: string };
  address: string;
  /** The id of the stored file, when the person may add a version of it. */
  versionOf?: string;
}) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const close = useCallback(() => setIsOpen(false), []);
  const type = file.content_type ?? '';
  const target = useMemo(
    () => ({ name: file.filename, type, load: () => loadFile(address) }),
    [file.filename, type, address],
  );
  const saveVersion = useCallback(
    async (bytes: ArrayBuffer) => {
      if (!versionOf) return;
      const name = encodeURIComponent(file.filename);
      await api.upload<Attachment>(
        `/attachments/${versionOf}/versions?filename=${name}`,
        new File([bytes], file.filename, { type: 'application/pdf' }),
      );
    },
    [versionOf, file.filename],
  );
  const craft = craftOf(file.filename, type);
  if (!craft) return null;
  return (
    <>
      <button type="button" className="ghost" onClick={() => setIsOpen(true)}>
        {t(`craft.open.${craft}`)}
      </button>
      {isOpen && (
        <CraftDialog
          craft={craft}
          file={target}
          download={`${address}?download=1`}
          onSaveVersion={versionOf ? saveVersion : undefined}
          onClose={close}
        />
      )}
    </>
  );
}
