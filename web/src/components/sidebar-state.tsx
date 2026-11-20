// SPDX-License-Identifier: Apache-2.0
// Whether the sidebar shows, one state for every frame, and the button at the start of the top
// bar that changes it (spec 078, NAV-092). A frame that has a sidebar puts the button into the
// bar, so a screen without one has no button that would do nothing (NAV-004).
import { createContext, useContext, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { IconSidebar } from './icons';
import { useShortcut } from './shortcuts';

const SIDEBAR_KEY = 'kenwerk.sidebar';
const SIDEBAR_HIDDEN = 'hidden';
const NARROW_SCREEN = '(max-width: 50rem)';
const SIDEBAR_ICON_SIZE = 20;

interface SidebarState {
  visible: boolean;
  setVisible: (visible: boolean) => void;
  /** Where the button goes in the top bar, once the bar is on the screen. */
  slot: HTMLElement | null;
  setSlot: (slot: HTMLElement | null) => void;
}

const SidebarContext = createContext<SidebarState | null>(null);

/** The browser keeps the choice; a narrow screen starts without the sidebar. */
function storedVisibility(): boolean {
  const stored = localStorage.getItem(SIDEBAR_KEY);
  return stored ? stored !== SIDEBAR_HIDDEN : !window.matchMedia(NARROW_SCREEN).matches;
}

export function SidebarProvider({ children }: { children: ReactNode }) {
  const [visible, setShown] = useState(storedVisibility);
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  const setVisible = (next: boolean) => {
    setShown(next);
    localStorage.setItem(SIDEBAR_KEY, next ? 'visible' : SIDEBAR_HIDDEN);
  };
  return (
    <SidebarContext.Provider value={{ visible, setVisible, slot, setSlot }}>
      {children}
    </SidebarContext.Provider>
  );
}

function useSidebarState(): SidebarState {
  const state = useContext(SidebarContext);
  if (!state) throw new Error('the sidebar is used outside SidebarProvider');
  return state;
}

/** Whether the sidebar shows. */
export function useSidebarVisible(): boolean {
  return useSidebarState().visible;
}

/** The place at the start of the top bar that a frame fills with its button. */
export function SidebarSlot() {
  const { setSlot } = useSidebarState();
  return <span ref={setSlot} className="top-slot" />;
}

/** The button of a frame that has a sidebar, placed at the start of the top bar, and `[`. */
export function SidebarToggle() {
  const { t } = useTranslation();
  const { visible, setVisible, slot } = useSidebarState();
  useShortcut('sidebar', () => setVisible(!visible));
  if (!slot) return null;
  const label = visible ? t('space.hideSidebar') : t('space.showSidebar');
  return createPortal(
    <button
      type="button"
      className="icon top-icon"
      aria-label={label}
      title={label}
      onClick={() => setVisible(!visible)}
    >
      <IconSidebar size={SIDEBAR_ICON_SIZE} />
    </button>,
    slot,
  );
}
