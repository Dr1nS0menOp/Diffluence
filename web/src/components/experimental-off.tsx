// SPDX-License-Identifier: Apache-2.0
// What a screen shows in place of an experimental part that is off (spec 069).
import { useTranslation } from 'react-i18next';

export function ExperimentalOff() {
  const { t } = useTranslation();
  return <p className="muted">{t('experimental.off')}</p>;
}
