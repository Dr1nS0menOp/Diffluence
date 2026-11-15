// SPDX-License-Identifier: Apache-2.0
// The signatures of a version as text: name, meaning, date and time, and the mark of each (spec
// 064, DOC-740), on the band of a page and of its published copy, and in the history.
import { useTranslation } from 'react-i18next';
import type { SignatureView } from '../api';
import { IconVerified } from './icons';
import { RelativeTime } from './ui';

const MARK_ICON_SIZE = 16;

/** The signatures of the version on display, each with its mark (DOC-740). */
export function Signatures({ list }: { list: SignatureView[] }) {
  const { t } = useTranslation();
  if (list.length === 0) return null;
  return (
    <ul className="plain-list control-signatures" aria-label={t('signing.listLabel')}>
      {list.map((s) => (
        <li key={s.id}>
          {s.mark === 'valid' ? <IconVerified size={MARK_ICON_SIZE} /> : null}{' '}
          {t('signing.line', {
            name: s.signer_name,
            meaning: t(`signing.meaning_${s.meaning}`, { defaultValue: s.meaning_name }),
          })}{' '}
          <RelativeTime value={s.signed_at} />{' '}
          <span className={s.mark === 'valid' ? 'muted' : 'lozenge'}>
            {s.mark === 'valid' ? t('signing.markValid') : t('signing.markMismatch')}
          </span>
        </li>
      ))}
    </ul>
  );
}
