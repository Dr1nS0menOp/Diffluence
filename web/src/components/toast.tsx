// SPDX-License-Identifier: Apache-2.0
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { IconClose } from './icons';

const SHOWN_MS = 10_000;
const CLOSE_ICON_SIZE = 16;

export interface Toast {
  message: string;
  action?: { label: string; run: () => void };
}

const ToastContext = createContext<(toast: Toast) => void>(() => undefined);

/** Shows a short message at the bottom of the screen. */
export const useToast = () => useContext(ToastContext);

/** Copies text, and says so. A browser can refuse, and then the person must know. */
export function useCopy() {
  const { t } = useTranslation();
  const show = useToast();
  return async (text: string, done: string) => {
    try {
      await navigator.clipboard.writeText(text);
      show({ message: done });
    } catch {
      show({ message: t('app.copyFailed') });
    }
  };
}

function ToastMessage({ toast, onClose }: { toast: Toast; onClose: () => void }) {
  const { t } = useTranslation();
  const root = useRef<HTMLDivElement>(null);
  const [paused, setPaused] = useState(false);

  // WCAG 2.2.1: a message with an action stays while it is being used.
  useEffect(() => {
    if (paused) return;
    const timer = setTimeout(onClose, SHOWN_MS);
    return () => clearTimeout(timer);
  }, [paused, onClose]);

  return (
    <div
      ref={root}
      className="toast"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(root.current?.contains(document.activeElement) ?? false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <span>{toast.message}</span>
      {toast.action && (
        <button
          type="button"
          className="ghost"
          onClick={() => {
            toast.action?.run();
            onClose();
          }}
        >
          {toast.action.label}
        </button>
      )}
      <button type="button" className="icon" aria-label={t('common.close')} onClick={onClose}>
        <IconClose size={CLOSE_ICON_SIZE} />
      </button>
    </div>
  );
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<Toast | null>(null);
  const close = useCallback(() => setToast(null), []);
  const show = useMemo(() => (next: Toast) => setToast(next), []);
  return (
    <ToastContext.Provider value={show}>
      {children}
      {/* The region exists before a message does, so screen readers announce it. */}
      <div role="status" className="toast-region">
        {toast && <ToastMessage key={toast.message} toast={toast} onClose={close} />}
      </div>
    </ToastContext.Provider>
  );
}
