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
export const IconUnlink = line(
  'M9 15l-1.5 1.5a3.5 3.5 0 0 1-5-5L5 9M15 9l1.5-1.5a3.5 3.5 0 0 1 5 5L19 15M9 4v3M15 20v-3M4 9h0M20 15h0',
);
export const IconLink = line(
  'M10 14a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1M14 10a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1',
);
export const IconAttach = line(
  'M20 11.5l-8.2 8.2a5 5 0 0 1-7.1-7.1l8.7-8.7a3.3 3.3 0 0 1 4.700 4.700l-8.700 8.700a1.700 1.700 0 0 1-2.400-2.400l8-8',
);
export const IconPicture = line('M4 5h16v14H4zM4 16l5-5 4 4 3-3 4 4M15.5 9h.01');
export const IconChevronUp = line('M6 15l6-6 6 6');
export const IconTextColor = line('M6 17l6-13 6 13M8.3 12h7.4M4 21h16');
export const IconHighlight = line('M14 5l5 5-8 8H7v-4zM4 21h16');
export const IconClear = line('M6 5h13M12 5l-3 14M4 4l16 16');
export const IconSubscript = line('M4 6l7 10M11 6L4 16M15 15h5l-5 5h5');
export const IconSuperscript = line('M4 8l7 10M11 8L4 18M15 4h5l-5 5h5');
export const IconAlignStart = line('M4 6h16M4 11h10M4 16h16M4 21h10');
export const IconAlignCenter = line('M4 6h16M7 11h10M4 16h16M7 21h10');
export const IconAlignEnd = line('M4 6h16M10 11h10M4 16h16M10 21h10');
export const IconIndent = line('M4 5h16M11 10h9M11 15h9M4 20h16M4 9l3 3-3 3');
export const IconOutdent = line('M4 5h16M11 10h9M11 15h9M4 20h16M7 9l-3 3 3 3');
export const IconHeading = line('M6 5v14M18 5v14M6 12h12');
export const IconText = line('M5 7V5h14v2M12 5v14M9 19h6');
export const IconPanelInfo = line('M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 11v5M12 8h.01');
export const IconPanelNote = line('M5 4h14v12l-4 4H5zM15 20v-4h4');
export const IconPanelSuccess = line('M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM8 12l3 3 5-6');
export const IconPanelWarning = line('M12 4l9 16H3zM12 10v4M12 17h.01');
export const IconPanelError = line('M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM9 9l6 6M15 9l-6 6');
export const IconExpand = line('M4 6l4 4 4-4M15 8h5M4 14h16M4 19h16');
export const IconColumns = line('M4 5h7v14H4zM13 5h7v14h-7z');
export const IconDecision = line('M12 3v6M12 9l-6 5v7M12 9l6 5v7');
export const IconStatus = line('M7 8h10a4 4 0 0 1 0 8H7a4 4 0 0 1 0-8z');
export const IconDate = line('M5 6h14v14H5zM5 10h14M9 4v4M15 4v4');
export const IconEmoji = line('M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM9 10h.01M15 10h.01M8 14c2 3 6 3 8 0');
export const IconGrip = line('M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01');
export const IconMoveUp = line('M12 19V5M6 11l6-6 6 6');
export const IconMoveDown = line('M12 5v14M6 13l6 6 6-6');
export const IconUnwrap = line('M4 8V4h4M20 8V4h-4M4 16v4h4M20 16v4h-4M9 12h6');
export const IconHeaderRow = line('M4 5h16v14H4zM4 10h16M6 7.500h12');
export const IconHeaderColumn = line('M4 5h16v14H4zM10 5v14M7 7v10');
export const IconMerge = line('M4 5h16v14H4zM9 12H4M20 12h-5M9 9l3 3-3 3M15 9l-3 3 3 3');
export const IconEqual = line('M4 5h16v14H4zM9.300 5v14M14.700 5v14');
export const IconFill = line(
  'M5 12l7-7 7 7-7 7zM5 12h14M20 16c1 2 1.500 3 1.500 4a1.500 1.500 0 0 1-3 0c0-1 .5-2 1.500-4',
);
export const IconSortUp = line('M6 18V6M3 9l3-3 3 3M13 6h8M13 11h6M13 16h4');
export const IconSortDown = line('M6 6v12M3 15l3 3 3-3M13 6h4M13 11h6M13 16h8');
export const IconLineNumbers = line('M9 6h11M9 12h11M9 18h11M4 5v3M4 11h2l-2 3h2M4 17h2v3H4');
export const IconChart = line('M4 4v16h16M8 16v-4M12 16V8M16 16v-6');
export const IconFrame = line('M4 4h16v16H4zM8 8h8v8H8z');
export const IconSpotlight = line('M3 7h18v10H3zM7 11h6M7 14h3M15 13h3');
export const IconWrap = line('M4 6h16M4 12h13a3 3 0 0 1 0 6h-4M15 16l-2 2 2 2M4 18h5');
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

export const IconFolder = line('M3 6h6l2 3h10v10H3z');
export const IconTick = line('M5 13l4 4L19 7');
export const IconSheet = line('M4 4h16v16H4zM4 10h16M4 15h16M10 4v16');
export const IconArchive = line('M4 5h16v4H4zM6 9v10h12V9M10 13h4');
export const IconVerified = line('M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM8 12l3 3 5-6');
export const IconBlog = line('M5 4h14v16H5zM8 8h8M8 12h8M8 16h5');
export const IconTable2 = line('M4 5h16v14H4zM4 10h16M10 10v9');
export const IconBell = line('M6 16v-5a6 6 0 0 1 12 0v5l2 2H4zM10 21h4');
export const IconEye = line(
  'M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z',
);
export const IconComment = line('M4 5h16v11H9l-5 4z');
export const IconApps = line('M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z');
export const IconHome = line('M4 11l8-7 8 7M6 10v10h12V10M10 20v-6h4v6');
export const IconPeople = line(
  'M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM2 21v-1a6 6 0 0 1 6-6h2a6 6 0 0 1 6 6v1M16 3.5a4 4 0 0 1 0 7M18 14a5 5 0 0 1 4 5v2',
);
export const IconHelp = line(
  'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM9.5 9.5a2.5 2.5 0 1 1 3.7 2.2c-.8.4-1.2 1-1.2 1.8M12 17h.01',
);
export const IconInfo = line('M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 11v5M12 8h.01');
export const IconCollapse = line('M7 10l5-5 5 5M7 14l5 5 5-5');
export const IconLocate = line('M12 18a6 6 0 1 0 0-12 6 6 0 0 0 0 12zM12 2v4M12 18v4M2 12h4M18 12h4');
export const IconExternal = line('M14 5h5v5M19 5l-8 8M18 14v5H5V6h5');
export const IconPrint = line('M7 9V4h10v5M7 17H4v-7h16v7h-3M7 14h10v6H7z');
export const IconPresent = line('M3 5h18v11H3zM12 16v4M8 20h8');
export const IconReader = line(
  'M4 5h6a2 2 0 0 1 2 2v13a2 2 0 0 0-2-2H4zM20 5h-6a2 2 0 0 0-2 2v13a2 2 0 0 1 2-2h6z',
);
export const IconFullWidth = line('M3 12h18M3 12l4-4M3 12l4 4M21 12l-4-4M21 12l-4 4M3 4v16M21 4v16');
export const IconTableMenu = line('M4 5h16v14H4zM4 10h16M10 10v9M16 10v9');
export const IconArrowUp = line('M12 19V5M6 11l6-6 6 6');
export const IconArrowDown = line('M12 5v14M6 13l6 6 6-6');

/** The same link icon, for the heading marks that ProseMirror builds outside React. */
export function linkIconElement(): SVGSVGElement {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('width', '16');
  svg.setAttribute('height', '16');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.8');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');
  svg.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS(ns, 'path');
  path.setAttribute(
    'd',
    'M10 14a4 4 0 0 0 6 0l3-3a4 4 0 0 0-6-6l-1 1M14 10a4 4 0 0 0-6 0l-3 3a4 4 0 0 0 6 6l1-1',
  );
  svg.append(path);
  return svg;
}
