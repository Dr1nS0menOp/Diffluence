// SPDX-License-Identifier: Apache-2.0
// A notice in the flow of a screen (NAV-149, NAV-150): a tinted block with an icon in a column of
// its own, an optional bold title, the text, and an optional row of actions. Its kind shows in the
// icon's shape as well as its colour, so it never rests on colour alone.
import type { ReactNode } from 'react';
import { IconInfo, IconPanelError, IconPanelWarning } from './icons';

export type NoticeKind = 'info' | 'warning' | 'error';

const ICONS = { info: IconInfo, warning: IconPanelWarning, error: IconPanelError } as const;
const ICON_SIZE = 18;

/** A warning always has a bold title, so its kind never rests on colour or icon alone (NAV-150). */
type NoticeHead = { kind: 'warning'; title: string } | { kind: 'info' | 'error'; title?: string };

/**
 * One notice. An error is an alert, so a screen reader says it when it appears; the others are a
 * status. A notice about a setting or a lasting cause has no close button, so none is offered.
 */
export function Notice({
  kind,
  title,
  actions,
  children,
}: NoticeHead & {
  /** A primary button, then a link, under the text. */
  actions?: ReactNode;
  children: ReactNode;
}) {
  const Icon = ICONS[kind];
  return (
    <div role={kind === 'error' ? 'alert' : 'status'} className={`notice notice-${kind}`}>
      <span className="notice-icon">
        <Icon size={ICON_SIZE} />
      </span>
      <div className="notice-body">
        {title && <p className="notice-title">{title}</p>}
        <div className="notice-text">{children}</div>
        {actions && <div className="notice-actions">{actions}</div>}
      </div>
    </div>
  );
}
