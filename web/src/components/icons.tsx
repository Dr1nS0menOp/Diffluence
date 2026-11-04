// SPDX-License-Identifier: Apache-2.0
// Our own icon set: simple line drawings, so no icon library is needed. This is
// the only file that may draw; docs/design/README.md says how to add one.
import type { ReactNode } from 'react';

const DEFAULT_ICON_SIZE = 20;

function Svg({ children, size = DEFAULT_ICON_SIZE }: { children: ReactNode; size?: number }) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

const line = (path: string) => {
  const Icon = ({ size }: { size?: number }) => (
    <Svg size={size}>
      <path d={path} />
    </Svg>
  );
  return Icon;
};

export const IconPencil = line('M4 20l1-4L16 5l3 3L8 19l-4 1zM14 7l3 3');
export const IconStar = line('M12 3.6l2.6 5.3 5.9.9-4.2 4.1 1 5.8-5.3-2.8-5.3 2.8 1-5.8-4.2-4.1 5.9-.9z');
export const IconLock = line('M7 11V8a5 5 0 0 1 10 0v3M6 11h12v9H6z');
export const IconPlus = line('M12 5v14M5 12h14');
export const IconSearch = line('M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM20 20l-4-4');
export const IconChevronDown = line('M6 9l6 6 6-6');
export const IconChevronRight = line('M9 6l6 6-6 6');
export const IconChevronLeft = line('M15 6l-6 6 6 6');
export const IconArrowBack = line('M19 12H5M11 6l-6 6 6 6');
export const IconPage = line('M7 3h7l4 4v14H7zM14 3v4h4');
export const IconSliders = line('M4 7h9M17 7h3M4 17h3M11 17h9M15 5v4M9 15v4');
export const IconTrash = line('M5 7h14M10 7V4h4v3M7 7l1 13h8l1-13');
export const IconClock = line('M12 7v5l3 2M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18z');
export const IconMove = line(
  'M12 3v18M12 3l-3 3M12 3l3 3M12 21l-3-3M12 21l3-3M3 12h18M3 12l3-3M3 12l3 3M21 12l-3-3M21 12l-3 3',
);
export const IconCopy = line('M9 9h11v11H9zM5 15H4V4h11v1');
export const IconDownload = line('M12 4v11M8 11l4 4 4-4M5 20h14');
export const IconUpload = line('M12 15V4M8 8l4-4 4 4M5 20h14');
export const IconRename = line('M4 7V5h12v2M10 5v14M8 19h4M16 11h4M18 11v8M16 19h4');
export const IconClose = line('M6 6l12 12M18 6L6 18');
export const IconBold = line('M7 5h6a3.5 3.5 0 0 1 0 7H7zM7 12h7a3.5 3.5 0 0 1 0 7H7z');
export const IconItalic = line('M10 5h8M6 19h8M14 5l-4 14');
export const IconUnderline = line('M7 4v7a5 5 0 0 0 10 0V4M5 20h14');
export const IconStrike = line(
  'M4 12h16M16.5 7.5C15.7 5.9 14.1 5 12 5 9.4 5 7.5 6.4 7.5 8.500c0 1 .4 1.8 1.2 2.400M7.5 16.500C8.3 18 10 19 12 19c2.8 0 4.7-1.5 4.7-3.6 0-.6-.1-1.1-.4-1.5',
);
export const IconBullet = line('M9 6h11M9 12h11M9 18h11M4.5 6h.01M4.5 12h.01M4.5 18h.01');
export const IconNumbered = line(
  'M10 6h10M10 12h10M10 18h10M4 5l1.5-1v5M4 14c.5-1 2.5-1 2.5.5 0 1-2.5 2-2.5 3.500h2.5',
);
export const IconTask = line('M4 5h6v6H4zM5.5 8l1 1 2-2M14 8h6M4 15h6v6H4zM14 18h6');
export const IconQuote = line('M9 7c-3 1-4 3-4 6v3h5v-5H6M19 7c-3 1-4 3-4 6v3h5v-5h-4');
export const IconCode = line('M8 8l-4 4 4 4M16 8l4 4-4 4M14 5l-4 14');
export const IconCodeBlock = line('M4 5h16v14H4zM9 10l-2 2 2 2M15 10l2 2-2 2');
export const IconRule = line('M4 12h16');
export const IconLink = line(
  'M10 14a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1M14 10a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1',
);
export const IconImage = line('M4 5h16v14H4zM4 16l5-5 4 4 3-3 4 4M15.5 9h.01');
export const IconTable = line('M4 5h16v14H4zM4 10h16M4 15h16M10 5v14');
export const IconRowAdd = line('M4 5h16v6H4zM12 15v6M9 18h6');
export const IconColumnAdd = line('M4 4h6v16H4zM15 12h6M18 9v6');
export const IconRowDelete = line('M4 5h16v6H4zM9 18h6');
export const IconColumnDelete = line('M4 4h6v16H4zM15 12h6');
export const IconUndo = line('M9 7l-5 5 5 5M4 12h10a5 5 0 0 1 5 5');
export const IconRedo = line('M15 7l5 5-5 5M20 12H10a5 5 0 0 0-5 5');

export function IconMore({ size }: { size?: number }) {
  return (
    <Svg size={size}>
      <circle cx="5" cy="12" r="1.2" fill="currentColor" />
      <circle cx="12" cy="12" r="1.2" fill="currentColor" />
      <circle cx="19" cy="12" r="1.2" fill="currentColor" />
    </Svg>
  );
}

/** A river that splits in two: what the word diffluence means. */
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg viewBox="0 0 32 32" width={size} height={size} aria-hidden="true" focusable="false">
      <rect width="32" height="32" rx="7" className="logo-bg" />
      <path
        d="M16 27v-9c0-5-5-6-7-13M16 18c0-5 5-6 7-13"
        fill="none"
        className="logo-mark"
        strokeWidth="3"
        strokeLinecap="round"
      />
    </svg>
  );
}
