// SPDX-License-Identifier: Apache-2.0
// The list of headings beside a long page. It marks the section in view with a
// reading band near the top of the window, the usual use of IntersectionObserver.
import { useEffect, useState, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { jumpToHeading, type Heading } from './editor/outline';

// Starts below the top bar's edge, so a heading that a link scrolled to is inside it.
const READING_BAND = '-40px 0px -70% 0px';
export const MIN_HEADINGS_FOR_CONTENTS = 3;

function useCurrentHeading(headings: Heading[]) {
  const [current, setCurrent] = useState<string | undefined>(headings[0]?.slug);
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const inBand = entries.find((entry) => entry.isIntersecting);
        if (inBand) setCurrent(inBand.target.id);
      },
      { rootMargin: READING_BAND },
    );
    headings.forEach((h) => {
      const element = document.getElementById(h.slug);
      if (element) observer.observe(element);
    });
    return () => observer.disconnect();
  }, [headings]);
  return [current, setCurrent] as const;
}

export function PageContents({ headings }: { headings: Heading[] }) {
  const { t } = useTranslation();
  const [current, setCurrent] = useCurrentHeading(headings);
  const go = (e: MouseEvent, slug: string) => {
    e.preventDefault();
    window.history.replaceState(null, '', `#${slug}`);
    jumpToHeading(slug, true);
    setCurrent(slug);
  };
  return (
    <nav className="page-contents" aria-label={t('page.contents')}>
      <p className="page-contents-title">{t('page.contents')}</p>
      <ol>
        {headings.map((h) => (
          <li key={h.slug} data-level={h.level}>
            <a
              href={`#${h.slug}`}
              aria-current={h.slug === current ? 'location' : undefined}
              onClick={(e) => go(e, h.slug)}
            >
              {h.text}
            </a>
          </li>
        ))}
      </ol>
    </nav>
  );
}
