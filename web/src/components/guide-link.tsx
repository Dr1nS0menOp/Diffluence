// SPDX-License-Identifier: Apache-2.0
// A link to a part of the documentation: at the address the administrator set, or ours. It
// opens in a new tab and says so, with a mark after it and in its name (NAV-113, NAV-158).
// Nothing is fetched to show it.
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { appearanceQuery } from '../appearance';
import { IconExternal } from './icons';

/** Our own documentation, for an instance whose administrator set no address. */
export const DOCUMENTATION_ADDRESS = 'https://github.com/Dr1nS0menOp/Diffluence/tree/main/docs';

const MARK_SIZE = 14;

export function GuideLink({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const address = useQuery(appearanceQuery).data?.help_url || DOCUMENTATION_ADDRESS;
  return (
    <a className="guide-link" href={address} target="_blank" rel="noopener noreferrer">
      {children}
      <IconExternal size={MARK_SIZE} />
      <span className="visually-hidden">{t('space.opensNewTab')}</span>
    </a>
  );
}
