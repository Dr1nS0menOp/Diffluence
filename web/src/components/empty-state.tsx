// SPDX-License-Identifier: Apache-2.0
// A section the person may fill that holds nothing yet (NAV-173, ADM-114): one centred block with
// a short bold heading, a few lines, the button that makes the first item, and a guide link.
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { GuideLink } from './guide-link';

export function EmptyState({
  title,
  children,
  action,
  doc,
}: {
  title: string;
  children: ReactNode;
  /** The primary button that makes the first item. */
  action?: ReactNode;
  /** The path of the document that explains the section; without one, the documentation's start. */
  doc?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="empty-state">
      <p className="empty-state-title">{title}</p>
      <p>{children}</p>
      {action}
      <GuideLink path={doc}>{t('settingsSection.guide')}</GuideLink>
    </div>
  );
}
