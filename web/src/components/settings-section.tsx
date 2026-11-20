// SPDX-License-Identifier: Apache-2.0
// One section of the settings from top to bottom (ADM-080 to ADM-082, NAV-113): its name as the
// one h1, its primary action and More at the end of the title row, one or two sentences on what
// it is for with a link to the documentation, the notices about the whole section, then the content.
import { createContext, useContext, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { GuideLink } from './guide-link';

/** Where the content of a section puts its primary action: at the end of the title row. */
const ActionsSlot = createContext<HTMLElement | null>(null);

export function SettingsSection({
  title,
  intro,
  doc,
  notices,
  children,
}: {
  title: string;
  intro: string;
  /** The path of the section's document, relative to the root of the documentation (D10). */
  doc?: string;
  notices?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  return (
    <ActionsSlot.Provider value={slot}>
      <header className="settings-section-head">
        <div className="section-title-row">
          <h1>{title}</h1>
          <div className="section-actions" ref={setSlot} />
        </div>
        <p className="section-intro">
          {intro}
          {doc && (
            <>
              {' '}
              <GuideLink path={doc}>{t('settingsSection.guide')}</GuideLink>
            </>
          )}
        </p>
        {notices}
      </header>
      {children}
    </ActionsSlot.Provider>
  );
}

/**
 * The primary action of a section, and More, drawn at the end of its title row wherever the
 * content that owns them sits. Outside a section they are not shown.
 */
export function SectionActions({ children }: { children: ReactNode }) {
  const slot = useContext(ActionsSlot);
  return slot ? createPortal(children, slot) : null;
}
