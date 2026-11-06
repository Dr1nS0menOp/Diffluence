// SPDX-License-Identifier: Apache-2.0
// What greets someone new (spec 013, F7), and the page an administrator put on the
// start screen. The tour starts from the card and not by itself: a dialog that opens
// on its own takes the focus from someone who came here to read.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type PageDetail } from '../api';
import { appearanceQuery } from '../appearance';
import { usePreferences } from '../preferences';
import { Link } from '../router';
import { DocumentView } from './editor/editor';
import { Dialog } from './ui';

const TOUR_STEPS = ['spaces', 'pages', 'people'] as const;
const PAGE_ADDRESS = /\/pages\/([0-9a-f-]{36})/;

function Tour({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [at, setAt] = useState(0);
  const step = TOUR_STEPS[at] ?? 'spaces';
  const isLast = at === TOUR_STEPS.length - 1;
  return (
    <Dialog title={t(`firstUse.tour_${step}`)} onClose={onClose}>
      <p>{t(`firstUse.tourHelp_${step}`)}</p>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('firstUse.skip')}
        </button>
        <button type="button" className="primary" onClick={() => (isLast ? onClose() : setAt(at + 1))}>
          {isLast ? t('firstUse.done') : t('firstUse.next')}
        </button>
      </div>
    </Dialog>
  );
}

/** Explains spaces and pages until a person says they got it. */
export function FirstUseCard() {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const [touring, setTouring] = useState(false);
  if (preferences.welcome_dismissed) return null;
  return (
    <section className="first-use">
      <h2>{t('firstUse.title')}</h2>
      <p>{t('firstUse.help')}</p>
      <div className="row">
        {!preferences.tour_seen && (
          <button type="button" onClick={() => setTouring(true)}>
            {t('firstUse.tour')}
          </button>
        )}
        <button type="button" className="ghost" onClick={() => update({ welcome_dismissed: true })}>
          {t('firstUse.dismiss')}
        </button>
      </div>
      {touring && (
        <Tour
          onClose={() => {
            setTouring(false);
            update({ tour_seen: true });
          }}
        />
      )}
    </section>
  );
}

/** Shows for those who may read the page: the check is the normal one (NAV-038). */
export function WelcomePage() {
  const address = useQuery(appearanceQuery).data?.welcome_page ?? '';
  const id = PAGE_ADDRESS.exec(address)?.[1];
  const page = useQuery({
    queryKey: ['page', id],
    queryFn: () => api.get<PageDetail>(`/pages/${id}`),
    enabled: !!id,
    retry: false,
  });
  if (!page.data) return null;
  return (
    <section className="first-use">
      <h2>
        <Link href={`/pages/${page.data.id}`}>{page.data.title}</Link>
      </h2>
      <DocumentView document={page.data.document} plain />
    </section>
  );
}
