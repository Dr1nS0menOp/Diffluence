// SPDX-License-Identifier: Apache-2.0
// Save and Reset changes as one unit under the fields of a form (NAV-162, NAV-171, ADM-094): both
// inactive until something in the unit changed, and Save says why while it is inactive. The
// administration uses it now; the settings of a space and Your account follow in their waves.
import { useId } from 'react';
import { useTranslation } from 'react-i18next';

export function SaveReset({
  changed,
  busy = false,
  saveLabel,
  onReset,
}: {
  /** Whether a field of the unit differs from what is stored. */
  changed: boolean;
  busy?: boolean;
  /** Words for Save when the unit saves something else, such as a confirmed change. */
  saveLabel?: string;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const why = useId();
  return (
    <div className="save-reset">
      <button
        type="submit"
        className="primary"
        disabled={!changed || busy}
        aria-describedby={changed ? undefined : why}
      >
        {saveLabel ?? t('common.save')}
      </button>
      <button type="button" className="quiet" disabled={!changed || busy} onClick={onReset}>
        {t('saveReset.reset')}
      </button>
      {!changed && (
        <span id={why} className="visually-hidden">
          {t('saveReset.unchanged')}
        </span>
      )}
    </div>
  );
}

/** Whether a form differs from what is stored, field by field. */
export function differs<T extends object>(stored: T, form: T): boolean {
  return JSON.stringify(stored) !== JSON.stringify(form);
}
